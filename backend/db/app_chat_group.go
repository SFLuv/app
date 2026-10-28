package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// Group chat membership.
//
// The rule set, in one place because it is enforced in several:
//
//   - Exactly one owner, always. The owner cannot be removed, cannot leave
//     without handing the group over, and cannot be demoted by anybody
//     including themselves.
//   - The owner may promote a member to admin and demote an admin back.
//   - Owner and admins may invite and remove members. An admin may not remove
//     another admin — only the owner can — because otherwise two admins can
//     remove each other and whoever clicks first wins.
//   - A member may leave. Leaving sets left_at rather than deleting the row, so
//     their messages keep a sender and rejoining restores their history.
var (
	// ErrNotGroupMember means the caller is not in this group.
	ErrNotGroupMember = errors.New("not a member of this group")
	// ErrGroupPermission means the caller's role does not allow the action.
	ErrGroupPermission = errors.New("not allowed")
	// ErrOwnerImmutable covers removing, demoting or kicking the owner.
	ErrOwnerImmutable = errors.New("the owner cannot be changed that way")
	// ErrGroupTooLarge means the member cap would be exceeded.
	ErrGroupTooLarge = errors.New("this group is full")
)

// MaxGroupMembers caps a group.
//
// Fifty, not because anything breaks at fifty-one, but because every member is
// a fan-out target for every message: a push, a socket write, and an unread
// count recomputed. A cap that is clearly generous for a neighborhood group and
// clearly a cap is better than discovering the real limit in production.
const MaxGroupMembers = 50

// CreateGroupConversation opens a group with the caller as owner.
//
// The caller is inserted as owner whether or not they appear in members, and a
// duplicate in members cannot demote them: the ON CONFLICT keeps the higher
// role rather than the later write.
func (a *AppDB) CreateGroupConversation(ctx context.Context, ownerId string, title string, members []string) (*structs.ConversationRow, error) {
	title = strings.TrimSpace(title)

	unique := map[string]struct{}{}
	for _, id := range members {
		id = strings.TrimSpace(id)
		if id == "" || id == ownerId {
			continue
		}
		unique[id] = struct{}{}
	}
	if len(unique)+1 > MaxGroupMembers {
		return nil, ErrGroupTooLarge
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO conversations(kind, title, started_by, owner_id)
		VALUES ('group', $1, $2, $2)
		RETURNING id;
	`, title, ownerId).Scan(&id); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO conversation_members(conversation_id, user_id, role, added_by)
		VALUES ($1, $2, 'owner', $2);
	`, id, ownerId); err != nil {
		return nil, err
	}

	for userId := range unique {
		if _, err := tx.Exec(ctx, `
			INSERT INTO conversation_members(conversation_id, user_id, role, added_by)
			VALUES ($1, $2, 'member', $3)
			ON CONFLICT (conversation_id, user_id) DO NOTHING;
		`, id, userId, ownerId); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a.GetConversationForMember(ctx, id, ownerId)
}

// GroupMemberRow is one membership, as the management screen needs it.
type GroupMemberRow struct {
	UserId string
	Role   string
	Left   bool
}

// ListGroupMembers returns current members first, then people who have left but
// still have messages in the thread — the client needs their names to label
// those bubbles.
func (a *AppDB) ListGroupMembers(ctx context.Context, conversationId int64) ([]GroupMemberRow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT m.user_id, m.role, (m.left_at IS NOT NULL)
		FROM conversation_members m
		WHERE m.conversation_id = $1
			AND (
				m.left_at IS NULL
				OR EXISTS (
					SELECT 1 FROM messages x
					WHERE x.conversation_id = $1 AND x.sender_id = m.user_id AND x.deleted_at IS NULL
				)
			)
		ORDER BY
			(m.left_at IS NOT NULL),
			CASE m.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END,
			m.joined_at;
	`, conversationId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []GroupMemberRow{}
	for rows.Next() {
		var m GroupMemberRow
		if err := rows.Scan(&m.UserId, &m.Role, &m.Left); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// roleInGroup reads one member's current role, or ErrNotGroupMember.
func (a *AppDB) roleInGroup(ctx context.Context, q pgx.Tx, conversationId int64, userId string) (string, error) {
	var role string
	err := q.QueryRow(ctx, `
		SELECT role FROM conversation_members
		WHERE conversation_id = $1 AND user_id = $2 AND left_at IS NULL;
	`, conversationId, userId).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotGroupMember
	}
	return role, err
}

// UpdateGroupDetails renames a group or clears its picture.
//
// Admins and the owner may do this. Setting a picture is a separate upload
// route, because it is bytes rather than JSON.
func (a *AppDB) UpdateGroupDetails(ctx context.Context, conversationId int64, actorId string, patch *structs.GroupPatch) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	role, err := a.roleInGroup(ctx, tx, conversationId, actorId)
	if err != nil {
		return err
	}
	if role != structs.ConversationRoleOwner && role != structs.ConversationRoleAdmin {
		return ErrGroupPermission
	}

	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if _, err := tx.Exec(ctx, `
			UPDATE conversations SET title = $2 WHERE id = $1 AND kind = 'group';
		`, conversationId, title); err != nil {
			return err
		}
	}
	if patch.ClearIcon {
		// The attachment row goes with it: an icon nothing points at is bytes
		// in the database with no way to reach them.
		if _, err := tx.Exec(ctx, `
			DELETE FROM message_attachments
			WHERE id = (SELECT icon_attachment_id FROM conversations WHERE id = $1)
				AND message_id IS NULL;
		`, conversationId); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE conversations SET icon_attachment_id = NULL WHERE id = $1 AND kind = 'group';
		`, conversationId); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// SetGroupIcon points a group at an already-stored attachment, replacing and
// deleting whatever it pointed at before.
func (a *AppDB) SetGroupIcon(ctx context.Context, conversationId int64, attachmentId int64) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var previous *int64
	if err := tx.QueryRow(ctx, `
		SELECT icon_attachment_id FROM conversations WHERE id = $1;
	`, conversationId).Scan(&previous); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE conversations SET icon_attachment_id = $2 WHERE id = $1 AND kind = 'group';
	`, conversationId, attachmentId); err != nil {
		return err
	}
	if previous != nil {
		if _, err := tx.Exec(ctx, `
			DELETE FROM message_attachments WHERE id = $1 AND message_id IS NULL;
		`, *previous); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// AddGroupMember invites somebody. Idempotent: re-adding a current member is a
// no-op, and re-adding somebody who left clears their left_at so their history
// comes back with them.
func (a *AppDB) AddGroupMember(ctx context.Context, conversationId int64, actorId string, userId string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	role, err := a.roleInGroup(ctx, tx, conversationId, actorId)
	if err != nil {
		return err
	}
	if role != structs.ConversationRoleOwner && role != structs.ConversationRoleAdmin {
		return ErrGroupPermission
	}

	var count int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM conversation_members WHERE conversation_id = $1 AND left_at IS NULL;
	`, conversationId).Scan(&count); err != nil {
		return err
	}
	if count >= MaxGroupMembers {
		return ErrGroupTooLarge
	}

	// Re-joining keeps whatever role they held, which is deliberate: an admin
	// who left and was invited back by the owner is an admin again. It cannot
	// be used to escalate, because only the owner can have made them one.
	if _, err := tx.Exec(ctx, `
		INSERT INTO conversation_members(conversation_id, user_id, role, added_by, joined_at)
		VALUES ($1, $2, 'member', $3, NOW())
		ON CONFLICT (conversation_id, user_id) DO UPDATE SET
			left_at = NULL,
			archived_at = NULL,
			added_by = $3,
			joined_at = CASE
				WHEN conversation_members.left_at IS NOT NULL THEN NOW()
				ELSE conversation_members.joined_at
			END;
	`, conversationId, userId, actorId); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RemoveGroupMember kicks somebody, or records that they left.
//
// actorId == userId is "leave", which any member may do. Otherwise it is a kick,
// which needs owner or admin, and an admin may not kick another admin.
func (a *AppDB) RemoveGroupMember(ctx context.Context, conversationId int64, actorId string, userId string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	actorRole, err := a.roleInGroup(ctx, tx, conversationId, actorId)
	if err != nil {
		return err
	}
	targetRole, err := a.roleInGroup(ctx, tx, conversationId, userId)
	if err != nil {
		return err
	}

	if targetRole == structs.ConversationRoleOwner {
		// Including the owner trying to leave. They have to hand the group over
		// first, so a group is never left with nobody who can administer it.
		return ErrOwnerImmutable
	}

	if actorId != userId {
		switch actorRole {
		case structs.ConversationRoleOwner:
			// May remove anybody but themselves, handled above.
		case structs.ConversationRoleAdmin:
			if targetRole == structs.ConversationRoleAdmin {
				return ErrGroupPermission
			}
		default:
			return ErrGroupPermission
		}
	}

	// Demoted to member on the way out, so an admin who is kicked does not come
	// back as one if somebody re-invites them later.
	if _, err := tx.Exec(ctx, `
		UPDATE conversation_members
		SET left_at = NOW(), role = 'member'
		WHERE conversation_id = $1 AND user_id = $2 AND left_at IS NULL;
	`, conversationId, userId); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetGroupMemberRole promotes or demotes. Owner only.
//
// Passing 'owner' transfers the group: the old owner becomes an admin in the
// same transaction, so there is never a moment with two owners or none.
func (a *AppDB) SetGroupMemberRole(ctx context.Context, conversationId int64, actorId string, userId string, role string) error {
	switch role {
	case structs.ConversationRoleOwner, structs.ConversationRoleAdmin, structs.ConversationRoleMember:
	default:
		return fmt.Errorf("unknown role %q", role)
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	actorRole, err := a.roleInGroup(ctx, tx, conversationId, actorId)
	if err != nil {
		return err
	}
	if actorRole != structs.ConversationRoleOwner {
		return ErrGroupPermission
	}
	if userId == actorId {
		// The owner cannot demote themselves. Handing over does it for them.
		return ErrOwnerImmutable
	}
	if _, err := a.roleInGroup(ctx, tx, conversationId, userId); err != nil {
		return err
	}

	if role == structs.ConversationRoleOwner {
		if _, err := tx.Exec(ctx, `
			UPDATE conversation_members SET role = 'admin'
			WHERE conversation_id = $1 AND user_id = $2;
		`, conversationId, actorId); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE conversations SET owner_id = $2 WHERE id = $1;
		`, conversationId, userId); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE conversation_members SET role = $3
		WHERE conversation_id = $1 AND user_id = $2 AND left_at IS NULL;
	`, conversationId, userId, role); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// InsertSystemMessage records a membership or title change in the thread, which
// is how everybody finds out one happened.
//
// Kind 'system', so clients render it centred and unattributed. The body is
// already-composed prose rather than a code and arguments: these lines are
// never translated, and a code would need a client release to add a new one.
func (a *AppDB) InsertSystemMessage(ctx context.Context, conversationId int64, actorId string, body string) (*structs.Message, error) {
	return a.InsertMessage(ctx, &NewMessage{
		ConversationId: conversationId,
		SenderId:       actorId,
		Kind:           structs.MessageKindSystem,
		Body:           body,
	})
}

// ── Conversation-level actions ───────────────────────────────────────────────

// MuteConversation silences a thread's pushes until a time, or clears the mute
// when until is nil.
//
// A timestamp rather than a mode: "an hour", "a day" and "forever" are all
// deadlines, and forever is simply a long one. One nullable column answers
// "should this buzz" with a single comparison, where a mode column beside a
// deadline would have two states that can disagree.
func (a *AppDB) MuteConversation(ctx context.Context, conversationId int64, userId string, until *time.Time) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE conversation_members SET muted_until = $3
		WHERE conversation_id = $1 AND user_id = $2 AND left_at IS NULL;
	`, conversationId, userId, until)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConversationNotFound
	}
	return nil
}

// ConversationIsMuted reports whether a member currently has it silenced.
func (a *AppDB) ConversationIsMuted(ctx context.Context, conversationId int64, userId string) (bool, error) {
	var muted bool
	err := a.db.QueryRow(ctx, `
		SELECT COALESCE(muted_until > NOW(), FALSE)
		FROM conversation_members
		WHERE conversation_id = $1 AND user_id = $2;
	`, conversationId, userId).Scan(&muted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return muted, err
}

// DeleteConversationForEveryone hides a thread from every member.
//
// A soft delete: the rows stay so a report filed against a message in it can
// still be answered. See the 1.76 migration for why that trade is made in this
// direction.
//
// Any member may do it, which is the capability the product asked for. It is
// recorded in deleted_by, so who did it is answerable afterwards rather than
// anonymous.
func (a *AppDB) DeleteConversationForEveryone(ctx context.Context, conversationId int64, userId string) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE conversations c
		SET deleted_at = NOW(), deleted_by = $2
		WHERE c.id = $1
			AND c.deleted_at IS NULL
			AND EXISTS (
				SELECT 1 FROM conversation_members m
				WHERE m.conversation_id = c.id AND m.user_id = $2 AND m.left_at IS NULL
			);
	`, conversationId, userId)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConversationNotFound
	}
	return nil
}

// DeleteOwnMessagesIn soft-deletes everything one person said in a thread, for
// "delete my messages and leave".
//
// deleted_by is set, so each one renders as the sender's own tombstone rather
// than vanishing — the thread keeps its shape for the people still in it, and a
// reply to something this person said does not become a non-sequitur.
func (a *AppDB) DeleteOwnMessagesIn(ctx context.Context, conversationId int64, userId string) error {
	_, err := a.db.Exec(ctx, `
		UPDATE messages
		SET deleted_at = NOW(), deleted_by = $2
		WHERE conversation_id = $1 AND sender_id = $2 AND deleted_at IS NULL;
	`, conversationId, userId)
	return err
}

// OtherMemberReadState is how far the other people in a thread have read, which
// is what "Delivered" and "Read" are derived from.
//
// The MINIMUM across the other members, not the maximum: in a group, "read"
// should mean everybody has seen it, and taking the furthest-along member would
// mark a message read because one person of five opened the thread.
func (a *AppDB) OtherMemberReadState(ctx context.Context, conversationId int64, userId string) (int64, error) {
	var lastRead *int64
	err := a.db.QueryRow(ctx, `
		SELECT MIN(last_read_message_id)
		FROM conversation_members
		WHERE conversation_id = $1 AND user_id <> $2 AND left_at IS NULL;
	`, conversationId, userId).Scan(&lastRead)
	if err != nil || lastRead == nil {
		return 0, err
	}
	return *lastRead, nil
}
