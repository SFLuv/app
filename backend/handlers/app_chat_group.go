package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/realtime"
	"github.com/SFLuv/app/backend/structs"
	"github.com/go-chi/chi/v5"
)

const groupTitleMaxRunes = 60

// writeGroupDetail is the single-thread view of a group.
//
// Diverges from the direct-thread view in three ways, each for a reason:
//
//   - A member list, with roles, because the info screen is built from it.
//   - No PayAddress. In-chat payment is a two-party idea: "pay the person I am
//     talking to". In a group there is no single recipient, and quietly picking
//     one would be a way to send money to the wrong person. The client hides
//     the control; this is the server making the same call so a crafted request
//     cannot reach it either.
//   - No block state, for the reason given on requireCanMessage.
func (a *AppService) writeGroupDetail(w http.ResponseWriter, r *http.Request, userId string, conv *structs.ConversationRow, status int) {
	ctx := r.Context()

	rows, err := a.db.ListGroupMembers(ctx, conv.Id)
	if err != nil {
		a.logger.Logf("error listing members of group %d: %s", conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UserId)
	}
	people, err := a.db.GetPublicUsers(ctx, ids)
	if err != nil {
		a.logger.Logf("error resolving group members: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	chatActive, err := a.db.ActiveBetaMembersAmong(ctx, structs.BetaFeatureChat, ids)
	if err != nil {
		a.logger.Logf("error narrowing chat beta members: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	members := make([]structs.ConversationMember, 0, len(rows))
	active := 0
	for _, row := range rows {
		_, stillIn := chatActive[row.UserId]
		members = append(members, structs.ConversationMember{
			User:     people[row.UserId],
			Role:     row.Role,
			Left:     row.Left,
			Inactive: !stillIn,
		})
		if !row.Left {
			active++
		}
	}

	writeJSON(w, status, &structs.Conversation{
		Id:               conv.Id,
		Kind:             conv.Kind,
		Title:            conv.Title,
		IconAttachmentId: conv.IconAttachmentId,
		OwnerId:          conv.OwnerId,
		MyRole:           conv.MyRole,
		Members:          members,
		MemberCount:      active,
		LastMessageAt:    conv.LastMessageAt,
	})
}

// CreateGroupConversation opens a group chat.
//
// Members must each be somebody the caller could already message one-to-one:
// not blocking them, not blocked by them, and holding the chat beta. Checked
// here rather than left to the client, because being added to a group is being
// put in a room with somebody — the one operation in chat where a third party
// decides who can reach you.
func (a *AppService) CreateGroupConversation(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	if !a.requireBulletinParticipant(w, r, userId) {
		return
	}

	var req structs.NewGroupRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<15)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read that request.")
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		bulletinError(w, http.StatusBadRequest, "empty", "Give the group a name.")
		return
	}
	if utf8.RuneCountInString(title) > groupTitleMaxRunes {
		bulletinError(w, http.StatusBadRequest, "too_long", "Group names can be up to 60 characters.")
		return
	}

	ctx := r.Context()
	allowed, rejected := a.filterMessageableUsers(ctx, userId, req.UserIds)
	if len(allowed) == 0 {
		bulletinError(w, http.StatusBadRequest, "no_members", "Add at least one other person to the group.")
		return
	}
	if rejected > 0 {
		// Proceeding with the rest rather than refusing the whole thing: the
		// person is making a group, and one unreachable invitee should not cost
		// them the other five. The client is told how many were left out.
		a.logger.Logf("group create by %s: %d invitee(s) could not be added", userId, rejected)
	}

	conv, err := a.db.CreateGroupConversation(ctx, userId, title, allowed)
	if errors.Is(err, db.ErrGroupTooLarge) {
		bulletinError(w, http.StatusBadRequest, "too_many", "A group can have up to 50 people.")
		return
	}
	if err != nil {
		a.logger.Logf("error creating group for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// The thread needs one message to exist: the inbox lists conversations that
	// have at least one, so a group with nothing in it would be invisible to
	// everybody including its owner.
	if _, err := a.db.InsertSystemMessage(ctx, conv.Id, userId, a.groupCreatedLine(ctx, userId, title)); err != nil {
		a.logger.Logf("error writing the opening line of group %d: %s", conv.Id, err)
	}

	a.publishConversationUpdated(ctx, conv.Id, userId)
	a.notifyGroupMembers(ctx, conv, userId, "You were added to "+title)

	a.writeGroupDetail(w, r, userId, conv, http.StatusCreated)
}

// filterMessageableUsers narrows a list of invitees to those the caller may put
// in a group, and reports how many were dropped.
func (a *AppService) filterMessageableUsers(ctx context.Context, callerId string, candidates []string) ([]string, int) {
	allowed := []string{}
	rejected := 0
	seen := map[string]struct{}{}

	for _, raw := range candidates {
		id := strings.TrimSpace(raw)
		if id == "" || id == callerId {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}

		blocked, err := a.db.UsersAreBlocked(ctx, callerId, id)
		if err != nil || blocked {
			rejected++
			continue
		}
		if !a.BetaFeatureAllowed(ctx, id, structs.BetaFeatureChat) {
			rejected++
			continue
		}
		allowed = append(allowed, id)
	}
	return allowed, rejected
}

// groupCreatedLine is the system message a new group opens with.
func (a *AppService) groupCreatedLine(ctx context.Context, userId string, title string) string {
	name := a.publicName(ctx, userId)
	return name + " created \"" + title + "\""
}

// publicName is one person's display name, or a neutral fallback.
func (a *AppService) publicName(ctx context.Context, userId string) string {
	if names, err := a.db.GetSenderNames(ctx, []string{userId}); err == nil {
		if name := strings.TrimSpace(names[userId]); name != "" {
			return name
		}
	}
	return "Someone"
}

// UpdateGroupConversation renames a group or clears its picture.
func (a *AppService) UpdateGroupConversation(w http.ResponseWriter, r *http.Request) {
	userId, conv, ok := a.loadGroupForMember(w, r)
	if !ok {
		return
	}

	var patch structs.GroupPatch
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&patch); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read that request.")
		return
	}
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			bulletinError(w, http.StatusBadRequest, "empty", "Give the group a name.")
			return
		}
		if utf8.RuneCountInString(title) > groupTitleMaxRunes {
			bulletinError(w, http.StatusBadRequest, "too_long", "Group names can be up to 60 characters.")
			return
		}
		patch.Title = &title
	}

	ctx := r.Context()
	if err := a.db.UpdateGroupDetails(ctx, conv.Id, userId, &patch); err != nil {
		a.writeGroupError(w, err, conv.Id)
		return
	}

	if patch.Title != nil && *patch.Title != conv.Title {
		if _, err := a.db.InsertSystemMessage(ctx, conv.Id, userId,
			a.publicName(ctx, userId)+" named the group \""+*patch.Title+"\""); err != nil {
			a.logger.Logf("error recording the rename of group %d: %s", conv.Id, err)
		}
	}

	a.publishConversationUpdated(ctx, conv.Id, userId)
	updated, err := a.db.GetConversationForMember(ctx, conv.Id, userId)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	a.writeGroupDetail(w, r, userId, updated, http.StatusOK)
}

// AddGroupMember invites somebody into a group.
func (a *AppService) AddGroupMember(w http.ResponseWriter, r *http.Request) {
	userId, conv, ok := a.loadGroupForMember(w, r)
	if !ok {
		return
	}

	var req structs.GroupMemberRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read that request.")
		return
	}
	target := strings.TrimSpace(req.UserId)
	if target == "" {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Choose somebody to add.")
		return
	}

	ctx := r.Context()
	allowed, _ := a.filterMessageableUsers(ctx, userId, []string{target})
	if len(allowed) == 0 {
		bulletinError(w, http.StatusConflict, "cannot_add", "They can't be added to this group.")
		return
	}

	if err := a.db.AddGroupMember(ctx, conv.Id, userId, target); err != nil {
		a.writeGroupError(w, err, conv.Id)
		return
	}

	if _, err := a.db.InsertSystemMessage(ctx, conv.Id, userId,
		a.publicName(ctx, userId)+" added "+a.publicName(ctx, target)); err != nil {
		a.logger.Logf("error recording an addition to group %d: %s", conv.Id, err)
	}

	a.publishConversationUpdated(ctx, conv.Id, userId)
	title := strings.TrimSpace(conv.Title)
	if title == "" {
		title = "a group"
	}
	a.notifyNewMessageWithTitle(userId, target, conv.Id, title, "You were added to "+title)

	w.WriteHeader(http.StatusNoContent)
}

// RemoveGroupMember kicks somebody, or records that the caller left.
func (a *AppService) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	userId, conv, ok := a.loadGroupForMember(w, r)
	if !ok {
		return
	}
	target := strings.TrimSpace(chi.URLParam(r, "userId"))
	if target == "" {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Choose somebody to remove.")
		return
	}

	ctx := r.Context()
	if err := a.db.RemoveGroupMember(ctx, conv.Id, userId, target); err != nil {
		a.writeGroupError(w, err, conv.Id)
		return
	}

	line := a.publicName(ctx, userId) + " left"
	if target != userId {
		line = a.publicName(ctx, userId) + " removed " + a.publicName(ctx, target)
	}
	if _, err := a.db.InsertSystemMessage(ctx, conv.Id, userId, line); err != nil {
		a.logger.Logf("error recording a departure from group %d: %s", conv.Id, err)
	}

	a.publishConversationUpdated(ctx, conv.Id, userId)
	w.WriteHeader(http.StatusNoContent)
}

// SetGroupMemberRole promotes or demotes a member, or hands the group over.
func (a *AppService) SetGroupMemberRole(w http.ResponseWriter, r *http.Request) {
	userId, conv, ok := a.loadGroupForMember(w, r)
	if !ok {
		return
	}
	target := strings.TrimSpace(chi.URLParam(r, "userId"))
	if target == "" {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Choose somebody.")
		return
	}

	var req structs.GroupMemberRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read that request.")
		return
	}

	ctx := r.Context()
	if err := a.db.SetGroupMemberRole(ctx, conv.Id, userId, target, req.Role); err != nil {
		a.writeGroupError(w, err, conv.Id)
		return
	}

	line := ""
	switch req.Role {
	case structs.ConversationRoleAdmin:
		line = a.publicName(ctx, target) + " is now an admin"
	case structs.ConversationRoleMember:
		line = a.publicName(ctx, target) + " is no longer an admin"
	case structs.ConversationRoleOwner:
		line = a.publicName(ctx, target) + " now owns the group"
	}
	if line != "" {
		if _, err := a.db.InsertSystemMessage(ctx, conv.Id, userId, line); err != nil {
			a.logger.Logf("error recording a role change in group %d: %s", conv.Id, err)
		}
	}

	a.publishConversationUpdated(ctx, conv.Id, userId)
	w.WriteHeader(http.StatusNoContent)
}

// loadGroupForMember loads the group in the path for the caller, refusing a
// direct thread.
func (a *AppService) loadGroupForMember(w http.ResponseWriter, r *http.Request) (string, *structs.ConversationRow, bool) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return "", nil, false
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return "", nil, false
	}
	if !conv.IsGroup() {
		bulletinError(w, http.StatusBadRequest, "not_a_group", "That conversation isn't a group.")
		return "", nil, false
	}
	return userId, conv, true
}

// writeGroupError maps the db layer's permission sentinels onto responses.
//
// ErrNotGroupMember becomes a 404 rather than a 403, matching how the rest of
// messaging treats membership: "not found" and "not yours" are the same answer,
// so neither can be used to probe the other.
func (a *AppService) writeGroupError(w http.ResponseWriter, err error, conversationId int64) {
	switch {
	case errors.Is(err, db.ErrNotGroupMember):
		bulletinError(w, http.StatusNotFound, "not_found", "That person isn't in this group.")
	case errors.Is(err, db.ErrGroupPermission):
		bulletinError(w, http.StatusForbidden, "not_allowed", "You don't have permission to do that.")
	case errors.Is(err, db.ErrOwnerImmutable):
		bulletinError(w, http.StatusConflict, "owner_immutable",
			"The group's owner can't be changed that way. Hand the group over to somebody else first.")
	case errors.Is(err, db.ErrGroupTooLarge):
		bulletinError(w, http.StatusConflict, "too_many", "A group can have up to 50 people.")
	default:
		a.logger.Logf("group %d: unexpected error: %s", conversationId, err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// notifyGroupMembers pushes a one-off line to everybody in a group except the
// actor, used when a group is created.
func (a *AppService) notifyGroupMembers(ctx context.Context, conv *structs.ConversationRow, actorId string, body string) {
	members, err := a.db.GetConversationMemberIds(ctx, conv.Id)
	if err != nil {
		return
	}
	title := strings.TrimSpace(conv.Title)
	if title == "" {
		title = "SFLuv"
	}
	reached := a.PublishNotification(members, title, body, map[string]string{
		"type":            "bulletin_message",
		"conversation_id": strconv.FormatInt(conv.Id, 10),
	})
	for _, id := range members {
		if id == actorId {
			continue
		}
		if _, told := reached[id]; told {
			continue
		}
		a.notifyNewMessageWithTitle(actorId, id, conv.Id, title, body)
	}
}

// UploadGroupIcon sets a group's picture.
//
// Two steps rather than one: the bytes are stored as a standalone attachment,
// then the group is pointed at it and whatever it pointed at before is deleted.
// Done in that order so a failure leaves the group with its old picture rather
// than with none — the attachment row is cheap to orphan and the migration's
// message_attachments_orphan_idx is there to find any that are.
func (a *AppService) UploadGroupIcon(w http.ResponseWriter, r *http.Request) {
	userId, conv, ok := a.loadGroupForMember(w, r)
	if !ok {
		return
	}
	if conv.MyRole != structs.ConversationRoleOwner && conv.MyRole != structs.ConversationRoleAdmin {
		bulletinError(w, http.StatusForbidden, "not_allowed", "Only admins can change the group picture.")
		return
	}

	contentType, data, ok := readUploadedImage(w, r, contactAvatarMaxBytes)
	if !ok {
		return
	}

	ctx := r.Context()
	size := len(data)
	attachmentId, err := a.db.InsertStandaloneAttachment(ctx, &db.NewAttachment{
		Kind:        structs.AttachmentKindImage,
		ContentType: contentType,
		FileName:    "group-icon",
		FileData:    data,
		SizeBytes:   &size,
	})
	if err != nil {
		a.logger.Logf("error storing the picture for group %d: %s", conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := a.db.SetGroupIcon(ctx, conv.Id, attachmentId); err != nil {
		a.logger.Logf("error setting the picture for group %d: %s", conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if _, err := a.db.InsertSystemMessage(ctx, conv.Id, userId,
		a.publicName(ctx, userId)+" changed the group picture"); err != nil {
		a.logger.Logf("error recording a picture change in group %d: %s", conv.Id, err)
	}

	a.publishConversationUpdated(ctx, conv.Id, userId)
	writeJSON(w, http.StatusOK, map[string]int64{"icon_attachment_id": attachmentId})
}

// ── Conversation actions ─────────────────────────────────────────────────────

// MuteForever is what the client sends for "mute until I unmute".
//
// A hundred years rather than NULL-means-forever, because NULL already means
// "not muted" and one column cannot carry both. Long enough that the
// distinction never matters, finite so the comparison stays a comparison.
const MuteForever = 100 * 365 * 24 * time.Hour

// MuteConversation silences a thread's pushes, or clears the mute.
func (a *AppService) MuteConversation(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return
	}

	var req structs.MuteConversationRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read that request.")
		return
	}

	var until *time.Time
	if req.Minutes > 0 {
		capped := time.Duration(req.Minutes) * time.Minute
		if capped > MuteForever {
			capped = MuteForever
		}
		when := time.Now().Add(capped)
		until = &when
	}

	if err := a.db.MuteConversation(r.Context(), conv.Id, userId, until); err != nil {
		if errors.Is(err, db.ErrConversationNotFound) {
			bulletinError(w, http.StatusNotFound, "not_found", "That conversation could not be found.")
			return
		}
		a.logger.Logf("error muting conversation %d: %s", conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"muted": until != nil})
}

// DeleteConversationForEveryone hides a thread from every member.
//
// A soft delete — the rows stay for moderation. See the 1.76 migration for why
// that direction was chosen, and note that this is a capability any member has
// over everybody else's copy, which is why who used it is recorded.
func (a *AppService) DeleteConversationForEveryone(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return
	}

	ctx := r.Context()
	// Captured before the delete: afterwards the membership read this needs is
	// still there, but the conversation is not loadable, and an event nobody
	// receives is worse than one sent a moment early.
	members, _ := a.db.GetConversationMemberIds(ctx, conv.Id)

	if err := a.db.DeleteConversationForEveryone(ctx, conv.Id, userId); err != nil {
		if errors.Is(err, db.ErrConversationNotFound) {
			bulletinError(w, http.StatusNotFound, "not_found", "That conversation could not be found.")
			return
		}
		a.logger.Logf("error deleting conversation %d: %s", conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if a.realtime != nil && len(members) > 0 {
		a.realtime.Publish(members, realtime.Event{
			Kind:           realtime.EventConversationUpdated,
			ConversationId: conv.Id,
			Data:           map[string]any{"deleted": true, "actor_id": userId},
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

// LeaveGroup leaves a group, optionally tombstoning everything the caller said
// in it first.
//
// "Delete my messages and leave" tombstones rather than erases, so the thread
// keeps its shape for the people still in it and a reply to something this
// person said does not become a non-sequitur.
func (a *AppService) LeaveGroup(w http.ResponseWriter, r *http.Request) {
	userId, conv, ok := a.loadGroupForMember(w, r)
	if !ok {
		return
	}

	var req structs.LeaveGroupRequest
	// An empty body is a plain leave; only the flag needs reading.
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&req)

	ctx := r.Context()
	if req.DeleteMyMessages {
		if err := a.db.DeleteOwnMessagesIn(ctx, conv.Id, userId); err != nil {
			a.logger.Logf("error clearing own messages in group %d: %s", conv.Id, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	if err := a.db.RemoveGroupMember(ctx, conv.Id, userId, userId); err != nil {
		a.writeGroupError(w, err, conv.Id)
		return
	}

	if _, err := a.db.InsertSystemMessage(ctx, conv.Id, userId, a.publicName(ctx, userId)+" left"); err != nil {
		a.logger.Logf("error recording a departure from group %d: %s", conv.Id, err)
	}
	a.publishConversationUpdated(ctx, conv.Id, userId)
	w.WriteHeader(http.StatusNoContent)
}
