package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	messagesMaxNewThreadsPerDay = 20
	messagesPageSize            = 50
	messageMaxRunes             = 2000
)

var txHashPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)

func conversationIdFromPath(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil && id > 0
}

// otherMember is the person on the other end of a DIRECT thread.
//
// Empty for a group, which has no "other member" — every caller that uses the
// result to look somebody up must check IsGroup first. Returning "" rather than
// an arbitrary member is deliberate: a group silently resolving to whoever
// happened to be user_low would be a wrong answer that looks like a right one.
func otherMember(c *structs.ConversationRow, me string) string {
	if c.IsGroup() {
		return ""
	}
	if c.UserLow == me {
		return c.UserHigh
	}
	return c.UserLow
}

// loadMemberConversation loads a thread the caller belongs to, writing a 404
// otherwise.
func (a *AppService) loadMemberConversation(w http.ResponseWriter, r *http.Request, userId string) (*structs.ConversationRow, bool) {
	id, ok := conversationIdFromPath(r)
	if !ok {
		bulletinError(w, http.StatusNotFound, "not_found", "That conversation could not be found.")
		return nil, false
	}
	c, err := a.db.GetConversationForMember(r.Context(), id, userId)
	if errors.Is(err, db.ErrConversationNotFound) {
		bulletinError(w, http.StatusNotFound, "not_found", "That conversation could not be found.")
		return nil, false
	}
	if err != nil {
		a.logger.Logf("error loading conversation %d: %s", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}
	return c, true
}

func markMine(msgs []*structs.Message, me string) {
	for _, m := range msgs {
		m.IsMine = m.SenderId == me
	}
}

// --- Inbox ---------------------------------------------------------------------

func (a *AppService) ListMyConversations(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	rows, err := a.db.ListConversations(ctx, userId)
	if err != nil {
		a.logger.Logf("error loading conversations for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	others := []string{}
	postIds := []int64{}
	for i := range rows {
		if other := otherMember(&rows[i].Conversation, userId); other != "" {
			others = append(others, other)
		}
		if rows[i].Conversation.PostId != nil {
			postIds = append(postIds, *rows[i].Conversation.PostId)
		}
	}
	people, err := a.db.GetPublicUsers(ctx, others)
	if err != nil {
		a.logger.Logf("error resolving conversation members: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	chips, err := a.db.GetConversationPostChips(ctx, postIds)
	if err != nil {
		a.logger.Logf("error resolving conversation posts: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	blockedIds, err := a.db.GetBlockedUserIds(ctx, userId)
	if err != nil {
		a.logger.Logf("error loading blocks for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	blocked := map[string]bool{}
	for _, id := range blockedIds {
		blocked[id] = true
	}

	// Which of the people on this page still have messages. Asked about the
	// page's own ids rather than the whole beta, and asked once rather than per
	// row. Nobody is removed from a conversation when they lose access, so this
	// is the only thing that distinguishes them — and it is why putting them
	// back needs no repair step.
	chatActive, chatErr := a.db.ActiveBetaMembersAmong(ctx, structs.BetaFeatureChat, others)
	if chatErr != nil {
		a.logger.Logf("error narrowing chat beta members: %s", chatErr)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	out := make([]*structs.Conversation, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		conv := &structs.Conversation{
			Id:              row.Conversation.Id,
			Kind:            row.Conversation.Kind,
			LastMessage:     row.Last,
			UnreadCount:     row.Unread,
			LastMessageAt:   row.Conversation.LastMessageAt,
			Muted:           row.Muted,
			OtherLastReadId: row.OtherLastReadId,
		}

		if row.Conversation.IsGroup() {
			conv.Title = row.Conversation.Title
			conv.IconAttachmentId = row.Conversation.IconAttachmentId
			conv.OwnerId = row.Conversation.OwnerId
			conv.MyRole = row.Conversation.MyRole
			conv.MemberCount = row.MemberCount
			// No OtherUser, no block state, no inactive flag: all three are
			// statements about one other person, and a group does not have one.
			// The row shows the title and the member count instead.
		} else {
			other := otherMember(&row.Conversation, userId)
			_, otherStillIn := chatActive[other]
			conv.OtherUser = people[other]
			conv.BlockedByMe = blocked[other]
			conv.OtherUserInactive = !otherStillIn
		}

		// A reaction is shown only when it is the NEWEST thing in the thread.
		// Older than the last message it is not the latest activity, and
		// showing it would describe the thread wrongly.
		if row.LatestReaction != nil && row.Last != nil &&
			row.LatestReaction.CreatedAt.After(row.Last.CreatedAt) {
			reaction := *row.LatestReaction
			reaction.IsMine = row.LatestReactionUserId == userId
			if !reaction.IsMine {
				if names, err := a.db.GetSenderNames(ctx, []string{row.LatestReactionUserId}); err == nil {
					reaction.UserName = names[row.LatestReactionUserId]
				}
			}
			conv.LatestReaction = &reaction
		}

		if conv.LastMessage != nil {
			conv.LastMessage.IsMine = conv.LastMessage.SenderId == userId
			// The inbox row for a group says who spoke last, so a preview is
			// attributable without opening the thread.
			if row.Conversation.IsGroup() {
				if names, err := a.db.GetSenderNames(ctx, []string{conv.LastMessage.SenderId}); err == nil {
					conv.LastMessage.SenderName = names[conv.LastMessage.SenderId]
				}
			}
		}
		if row.Conversation.PostId != nil {
			conv.Post = chips[*row.Conversation.PostId]
			if conv.Post != nil {
				conv.Post.PriceSfluv = displayBulletinPrice(conv.Post.PriceSfluv)
			}
		}
		out = append(out, conv)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *AppService) GetMessagingSummary(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	n, err := a.db.CountUnreadConversations(r.Context(), userId)
	if err != nil {
		a.logger.Logf("error counting unread conversations for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, structs.MessagingSummary{UnreadConversations: n})
}

// --- Opening a thread ---------------------------------------------------------------

// StartConversation opens (or reopens) a thread with another user, optionally
// about a post. A thread about a post must include the post's author.
func (a *AppService) StartConversation(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	if !a.requireBulletinParticipant(w, r, userId) {
		return
	}
	var req structs.NewConversationRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&req); err != nil || strings.TrimSpace(req.UserId) == "" {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read the request.")
		return
	}
	ctx := r.Context()
	if req.UserId == userId {
		bulletinError(w, http.StatusBadRequest, "self", "You can't message yourself.")
		return
	}

	active, err := a.db.UserIsActive(ctx, req.UserId)
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	blocked, err := a.db.UsersAreBlocked(ctx, userId, req.UserId)
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// Both people need the chat beta. The caller's own access was checked by the
	// route guard; this is the other half, and without it somebody in the beta
	// could pull anybody into a conversation they have no way to read.
	//
	// Reported as "could not be found", exactly as a blocked or deactivated
	// person is, rather than as "they are not in the beta". The distinction is
	// deliberate: a different answer here would let anyone enumerate the beta's
	// membership by trying to message people.
	inChatBeta := a.BetaFeatureAllowed(ctx, req.UserId, structs.BetaFeatureChat)
	if !active || blocked || !inChatBeta {
		// The caller sees one answer for all three; the log says which, so a
		// "could not be found" report can be explained from its request id.
		a.logger.Logf("[%s] start conversation refused: active=%t blocked=%t in_chat_beta=%t",
			middleware.GetReqID(ctx), active, blocked, inChatBeta)
		bulletinError(w, http.StatusNotFound, "not_found", "That person could not be found.")
		return
	}

	if req.PostId != nil {
		post, err := a.db.GetBulletinPost(ctx, *req.PostId, userId)
		if errors.Is(err, db.ErrBulletinNotFound) {
			bulletinError(w, http.StatusNotFound, "not_found", "That post could not be found.")
			return
		}
		if err != nil {
			a.logger.Logf("bulletin: unexpected error: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if post.AuthorId != userId && post.AuthorId != req.UserId {
			bulletinError(w, http.StatusBadRequest, "bad_request", "Message the person who posted this.")
			return
		}
	}

	existing, err := a.db.FindConversationId(ctx, userId, req.UserId, req.PostId)
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if existing == 0 {
		started, err := a.db.CountConversationsStartedSince(ctx, userId, time.Now().Add(-24*time.Hour))
		if err != nil {
			a.logger.Logf("bulletin: unexpected error: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if started >= messagesMaxNewThreadsPerDay {
			bulletinError(w, http.StatusTooManyRequests, "rate_limited", "You've started a lot of new conversations today. Try again tomorrow.")
			return
		}
	}

	conv, _, err := a.db.GetOrCreateConversation(ctx, userId, req.UserId, req.PostId)
	if err != nil {
		a.logger.Logf("error opening conversation for %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	a.writeConversationDetail(w, r, userId, conv, http.StatusOK)
}

func (a *AppService) GetConversation(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return
	}
	a.writeConversationDetail(w, r, userId, conv, http.StatusOK)
}

// writeConversationDetail is the single-thread view. It is the only place the
// other person's wallet address is returned, and only when the thread can
// carry an SFLUV payment: a plain thread, or a post that accepts SFLUV.
func (a *AppService) writeConversationDetail(w http.ResponseWriter, r *http.Request, userId string, conv *structs.ConversationRow, status int) {
	ctx := r.Context()

	if conv.IsGroup() {
		a.writeGroupDetail(w, r, userId, conv, status)
		return
	}

	other := otherMember(conv, userId)
	people, err := a.db.GetPublicUsers(ctx, []string{other})
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out := &structs.Conversation{
		Id:                conv.Id,
		Kind:              conv.Kind,
		OtherUser:         people[other],
		LastMessageAt:     conv.LastMessageAt,
		MemberCount:       2,
		OtherUserInactive: !a.BetaFeatureAllowed(ctx, other, structs.BetaFeatureChat),
	}
	canPay := true
	if conv.PostId != nil {
		chips, err := a.db.GetConversationPostChips(ctx, []int64{*conv.PostId})
		if err != nil {
			a.logger.Logf("bulletin: unexpected error: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		out.Post = chips[*conv.PostId]
		if out.Post != nil {
			out.Post.PriceSfluv = displayBulletinPrice(out.Post.PriceSfluv)
		}
		if out.Post != nil && out.Post.Category == structs.BulletinCategoryForSale && !out.Post.AcceptsSfluv {
			canPay = false
		}
	}
	blockedByMe, err := a.db.HasBlocked(ctx, userId, other)
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out.BlockedByMe = blockedByMe
	blocked, err := a.db.UsersAreBlocked(ctx, userId, other)
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if canPay && !blocked {
		addr, err := a.db.GetPrimaryWalletAddress(ctx, other)
		if err != nil {
			a.logger.Logf("error loading pay address for %s: %s", other, err)
		}
		out.PayAddress = addr
	}
	writeJSON(w, status, out)
}

// --- Reading and writing ------------------------------------------------------------

// GetConversationMessages returns a page of a thread, oldest first within the
// page. before is a message id; omit it for the newest page. Reading a thread
// also retries verification of any payments still waiting on the indexer.
func (a *AppService) GetConversationMessages(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return
	}
	before := int64(0)
	if raw := r.URL.Query().Get("before"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			bulletinError(w, http.StatusBadRequest, "bad_cursor", "That page link is no longer valid.")
			return
		}
		before = parsed
	}

	if before == 0 {
		a.retryPendingPayments(r.Context(), conv)
	}

	msgs, hasMore, err := a.db.GetMessages(r.Context(), conv.Id, before, messagesPageSize)
	if err != nil {
		a.logger.Logf("error loading messages for conversation %d: %s", conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	markMine(msgs, userId)
	a.hydrateMessages(r.Context(), conv, msgs, userId)
	writeJSON(w, http.StatusOK, structs.MessagesPage{Messages: msgs, HasMore: hasMore})
}

// requireCanMessage blocks sending into a thread when either person has
// blocked the other, or the sender is suspended or has no profile.
func (a *AppService) requireCanMessage(w http.ResponseWriter, r *http.Request, userId string, conv *structs.ConversationRow) bool {
	if !a.requireBulletinParticipant(w, r, userId) {
		return false
	}

	// A group is not gated on blocks.
	//
	// A block is a statement about a conversation between two people: "do not
	// let this person reach me". In a group the message is addressed to the
	// room, and letting one block silence somebody for everybody else would
	// hand any member a veto over the whole thread. The client-side consequence
	// of a block in a group is the reader's to apply — they can leave, and they
	// can still report — not the sender's to be refused for.
	if conv.IsGroup() {
		return true
	}

	blocked, err := a.db.UsersAreBlocked(r.Context(), userId, otherMember(conv, userId))
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}
	if blocked {
		bulletinError(w, http.StatusConflict, "blocked", "You can't send messages in this conversation.")
		return false
	}
	return true
}

func (a *AppService) SendConversationMessage(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return
	}
	if !a.requireCanMessage(w, r, userId, conv) {
		return
	}
	var req structs.NewMessageRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<15)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read the message.")
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		bulletinError(w, http.StatusBadRequest, "empty", "Write a message first.")
		return
	}
	if utf8.RuneCountInString(body) > messageMaxRunes {
		bulletinError(w, http.StatusBadRequest, "too_long", "Messages can be up to 2,000 characters.")
		return
	}

	msg, err := a.db.InsertMessage(r.Context(), &db.NewMessage{
		ConversationId: conv.Id,
		SenderId:       userId,
		Kind:           structs.MessageKindText,
		Body:           body,
		ReplyToId:      req.ReplyToId,
	})
	if err != nil {
		a.logger.Logf("error sending message in conversation %d: %s", conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	msg.IsMine = true
	a.hydrateMessages(r.Context(), conv, []*structs.Message{msg}, userId)
	a.fanOutMessage(r.Context(), conv, msg, userId, previewText(body))
	writeJSON(w, http.StatusCreated, msg)
}

// fanOutMessage is the one place a new message is announced, by either route.
//
// The order matters: the socket first, then pushes to whoever the socket did not
// reach. Doing it the other way round would mean a person with the app open gets
// a system banner for the message that is already on their screen, which is the
// single most irritating thing a chat app can do and the reason the realtime
// layer exists at all.
//
// Everything after the insert is best-effort. A failure here means somebody
// learns about the message on their next poll instead of instantly; it must
// never fail the send, which has already committed.
func (a *AppService) fanOutMessage(ctx context.Context, conv *structs.ConversationRow, msg *structs.Message, senderId string, preview string) {
	reached := a.publishMessage(ctx, conv, msg, senderId)

	recipients := []string{}
	if conv.IsGroup() {
		members, err := a.db.GetConversationMemberIds(ctx, conv.Id)
		if err != nil {
			a.logger.Logf("error listing members of conversation %d for push: %s", conv.Id, err)
			return
		}
		recipients = members
	} else if other := otherMember(conv, senderId); other != "" {
		recipients = []string{other}
	}

	title := ""
	if conv.IsGroup() {
		// A group push needs the group's name, not the sender's: four people
		// messaging one thread would otherwise read as four unrelated
		// conversations in the tray.
		title = strings.TrimSpace(conv.Title)
		if title == "" {
			title = "Group message"
		}
		if msg.SenderName != "" {
			preview = msg.SenderName + ": " + preview
		}
	}

	for _, id := range recipients {
		if id == senderId {
			continue
		}
		if _, alreadyTold := reached[id]; alreadyTold {
			continue
		}
		a.notifyNewMessageWithTitle(senderId, id, conv.Id, title, preview)
	}
}

// AttachConversationPayment records an SFLUV payment the sender already made.
// The transfer is checked against the indexer: it must go from a wallet the
// sender owns to a wallet the other person owns. If the indexer has not seen it
// yet, it is stored as pending and checked again when the thread is next read.
//
// Serves both the amount typed in the chat composer and the older route through
// the wallet's send screen. They differ only in where the number was entered —
// the client signs and broadcasts either way, and what arrives here is the hash
// of something that has already happened — so sharing one verification path is
// what keeps "paid in chat" from being a weaker claim than "paid and attached".
//
// ── Direct threads only ─────────────────────────────────────────────────────
//
// "I paid you" needs a "you". A group has no single counterparty, and the
// verification below is built on there being one: it checks the transfer went
// from the sender to the other member. In a group there is nothing to check
// against, so the honest options are to refuse or to invent a recipient, and
// inventing one means money moving to somebody nobody chose.
//
// The client hides the control in groups. This is the server refusing it too,
// so a crafted request cannot reach what the UI does not offer.
func (a *AppService) AttachConversationPayment(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return
	}
	if conv.IsGroup() {
		bulletinError(w, http.StatusBadRequest, "not_in_groups",
			"Sending SFLUV isn't available in group chats.")
		return
	}
	if !a.requireCanMessage(w, r, userId, conv) {
		return
	}
	var req structs.InChatPaymentRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read the payment.")
		return
	}
	hash := strings.ToLower(strings.TrimSpace(req.TxHash))
	if !txHashPattern.MatchString(hash) {
		bulletinError(w, http.StatusBadRequest, "bad_tx", "That isn't a valid transaction.")
		return
	}
	note := strings.TrimSpace(req.Note)
	if utf8.RuneCountInString(note) > 200 {
		bulletinError(w, http.StatusBadRequest, "too_long", "Payment notes can be up to 200 characters.")
		return
	}

	other := otherMember(conv, userId)
	verified, amount, mismatch, err := a.verifyConversationPayment(r.Context(), hash, userId, other)
	if err != nil {
		a.logger.Logf("error verifying payment %s: %s", hash, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if mismatch {
		bulletinError(w, http.StatusBadRequest, "wrong_payment", "That payment wasn't sent from you to this person.")
		return
	}

	var amountPtr *string
	if verified {
		amountPtr = &amount
	} else if claimed := strings.TrimSpace(req.AmountSfluvBase); claimed != "" {
		// What the sender says they sent, shown while the indexer catches up so
		// the bubble reads "12 SFLUV (confirming)" rather than a blank amount.
		//
		// Safe to show unverified because PaymentVerified is false alongside it
		// and the client renders that state differently. The moment the indexer
		// answers, retryPendingPayments overwrites this with the real figure —
		// so a sender who lies about the amount is corrected within a minute,
		// in front of the person they lied to.
		if amountPattern.MatchString(claimed) {
			amountPtr = &claimed
		}
	}
	msg, err := a.db.InsertMessage(r.Context(), &db.NewMessage{
		ConversationId:  conv.Id,
		SenderId:        userId,
		Kind:            structs.MessageKindPayment,
		Body:            note,
		TxHash:          &hash,
		AmountSfluvBase: amountPtr,
		PaymentVerified: verified,
		ReplyToId:       nil,
	})
	if errors.Is(err, db.ErrPaymentAlreadyAttached) {
		bulletinError(w, http.StatusConflict, "already_attached", "That payment is already in a conversation.")
		return
	}
	if err != nil {
		a.logger.Logf("error recording payment in conversation %d: %s", conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	msg.IsMine = true

	preview := "Sent you SFLUV"
	if verified {
		preview = fmt.Sprintf("Sent you %s SFLUV", formatBaseAmount(amount))
	}
	if note != "" {
		preview += " — " + previewText(note)
	}
	a.hydrateMessages(r.Context(), conv, []*structs.Message{msg}, userId)
	a.fanOutMessage(r.Context(), conv, msg, userId, preview)
	writeJSON(w, http.StatusCreated, msg)
}

// amountPattern is a whole number of base units, which is all a claimed amount
// is ever allowed to be.
var amountPattern = regexp.MustCompile(`^[0-9]{1,30}$`)

// ToggleMessageReaction adds or removes one emoji on one message.
//
// Idempotent in the useful sense: tapping the same emoji twice ends with it
// absent, and the response says which way it went so the client can settle on
// the server's answer rather than its own optimistic guess.
func (a *AppService) ToggleMessageReaction(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return
	}
	if !a.requireCanMessage(w, r, userId, conv) {
		return
	}

	messageId, err := strconv.ParseInt(chi.URLParam(r, "messageId"), 10, 64)
	if err != nil || messageId <= 0 {
		bulletinError(w, http.StatusNotFound, "not_found", "That message could not be found.")
		return
	}

	var req structs.ReactionRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read that reaction.")
		return
	}
	emoji, problem := normalizeReactionEmoji(req.Emoji)
	if problem != "" {
		bulletinError(w, http.StatusBadRequest, "bad_emoji", problem)
		return
	}

	ctx := r.Context()
	// Two checks, because they answer different questions and either alone
	// leaves a gap.
	//
	// The conversation in the path has already been confirmed as one the caller
	// is in, by loadMemberConversation above. What is NOT yet established is
	// that the message id in the path belongs to that conversation — the two
	// arrive independently, and a caller who is in any other thread would
	// otherwise pass a membership-only check while acting on a message from it.
	inThread, err := a.db.MessageIsInConversation(ctx, messageId, conv.Id)
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !inThread {
		bulletinError(w, http.StatusNotFound, "not_found", "That message could not be found.")
		return
	}
	visible, err := a.db.UserCanSeeMessage(ctx, messageId, userId)
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !visible {
		bulletinError(w, http.StatusNotFound, "not_found", "That message could not be found.")
		return
	}

	added, err := a.db.ToggleReaction(ctx, messageId, userId, emoji)
	if err != nil {
		a.logger.Logf("error toggling a reaction on message %d: %s", messageId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	reactions, err := a.db.GetMessageReactions(ctx, []int64{messageId}, userId)
	if err != nil {
		a.logger.Logf("error reloading reactions for message %d: %s", messageId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	a.publishReaction(ctx, conv.Id, messageId, userId, reactions[messageId])

	writeJSON(w, http.StatusOK, map[string]any{
		"added":     added,
		"reactions": reactions[messageId],
	})
}

// normalizeReactionEmoji checks that what arrived is plausibly one emoji.
//
// Not a fixed list. The client offers six by default and a full picker behind
// them, and a server-side allowlist would mean a release to add one. What is
// enforced instead is shape: short, no whitespace, no control characters, and
// nothing from the ASCII range — which is what keeps the field from becoming a
// second message body with no length limit and no moderation.
func normalizeReactionEmoji(raw string) (string, string) {
	emoji := strings.TrimSpace(raw)
	if emoji == "" {
		return "", "Pick a reaction."
	}
	// Bytes, not runes, and 64 rather than 16.
	//
	// 16 looked generous and was not: a family emoji like 👨‍👩‍👧 is three
	// pictographs joined by zero-width joiners, which is 18 bytes, so the cap
	// that was meant to bound the field was silently rejecting real reactions.
	// 64 admits the longest sequences anybody sends and still bounds it hard.
	//
	// The column's own CHECK is char_length BETWEEN 1 AND 16, which counts
	// CHARACTERS — that same family emoji is five — so the two limits agree
	// despite the different numbers.
	if len(emoji) > 64 {
		return "", "That isn't a reaction."
	}
	for _, r := range emoji {
		if r < 0x80 || r == 0xFFFD {
			return "", "That isn't a reaction."
		}
	}
	// A grapheme cluster can legitimately be several runes — a skin tone
	// modifier, a ZWJ sequence — so runes are deliberately not counted. The
	// byte cap above is what bounds it.
	return emoji, ""
}

// verifyConversationPayment checks a transfer against the indexer. verified is
// true when it is indexed and goes from sender to recipient; mismatch is true
// when it is indexed but goes somewhere else. Neither means not indexed yet.
func (a *AppService) verifyConversationPayment(ctx context.Context, hash, sender, recipient string) (verified bool, amount string, mismatch bool, err error) {
	if a.ponderDb == nil {
		return false, "", false, nil
	}
	transfer, err := a.ponderDb.GetTransferByHash(ctx, hash)
	if err != nil || transfer == nil {
		return false, "", false, err
	}
	fromOwner, err := a.db.GetUserIdByWalletAddress(ctx, transfer.From)
	if err != nil {
		return false, "", false, err
	}
	toOwner, err := a.db.GetUserIdByWalletAddress(ctx, transfer.To)
	if err != nil {
		return false, "", false, err
	}
	if fromOwner == nil || toOwner == nil || *fromOwner != sender || *toOwner != recipient {
		return false, "", true, nil
	}
	return true, transfer.Amount, false, nil
}

func (a *AppService) retryPendingPayments(ctx context.Context, conv *structs.ConversationRow) {
	pending, err := a.db.GetUnverifiedPayments(ctx, conv.Id)
	if err != nil || len(pending) == 0 {
		return
	}
	for _, m := range pending {
		if m.TxHash == nil {
			continue
		}
		recipient := conv.UserLow
		if recipient == m.SenderId {
			recipient = conv.UserHigh
		}
		verified, amount, _, err := a.verifyConversationPayment(ctx, *m.TxHash, m.SenderId, recipient)
		if err != nil || !verified {
			continue
		}
		if err := a.db.MarkPaymentVerified(ctx, m.Id, amount); err != nil {
			a.logger.Logf("error marking payment %d verified: %s", m.Id, err)
		}
	}
}

func (a *AppService) MarkConversationRead(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return
	}
	if err := a.db.MarkConversationRead(r.Context(), conv.Id, userId); err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// Re-read rather than reusing the row loaded above: that one's
	// last_read_message_id is from before this call, and a receipt carrying it
	// would tell the other end the reader is one message behind where they are.
	if updated, err := a.db.GetConversationForMember(r.Context(), conv.Id, userId); err == nil {
		a.publishRead(r.Context(), conv.Id, userId, updated.LastReadId)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *AppService) ArchiveConversation(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return
	}
	if err := a.db.ArchiveConversation(r.Context(), conv.Id, userId); err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Push -----------------------------------------------------------------------------

func previewText(body string) string {
	body = strings.Join(strings.Fields(body), " ")
	if utf8.RuneCountInString(body) > 120 {
		return truncateRunes(body, 117) + "..."
	}
	return body
}

// formatBaseAmount renders an indexer amount string (base units) as whole
// SFLUV using the shared formatter.
func formatBaseAmount(base string) string {
	v, ok := new(big.Int).SetString(base, 10)
	if !ok {
		return base
	}
	return formatSfluvBase(v)
}

// notifyNewMessage pushes to the recipient's devices in the background, so a
// slow push service never delays the sender. The title is the sender's name.
func (a *AppService) notifyNewMessage(senderId, recipientId string, conversationId int64, preview string) {
	a.notifyNewMessageWithTitle(senderId, recipientId, conversationId, "", preview)
}

// notifyNewMessageWithTitle is notifyNewMessage with the banner title chosen by
// the caller, which a group needs: its pushes are titled with the group's name
// rather than whoever happened to speak.
//
// An empty title falls back to the sender's public name, which is what a direct
// thread wants.
func (a *AppService) notifyNewMessageWithTitle(senderId, recipientId string, conversationId int64, title, preview string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Somebody taken out of the chat beta stops being notified. Their
		// conversations and unread counts are all still there for when they are
		// let back in; what stops is the phone buzzing about a thread they
		// currently cannot open.
		if !a.BetaFeatureAllowed(ctx, recipientId, structs.BetaFeatureChat) {
			return
		}

		// A muted thread does not buzz. Only the push is suppressed: the
		// message still arrives, the thread still shows unread, and the badge
		// still counts it — muting silences the phone, it does not hide the
		// conversation.
		if muted, err := a.db.ConversationIsMuted(ctx, conversationId, recipientId); err == nil && muted {
			return
		}

		if title == "" {
			people, err := a.db.GetPublicUsers(ctx, []string{senderId})
			if err != nil {
				return
			}
			title = people[senderId].PublicName
		}
		if title == "" {
			title = "New message"
		}
		// The phone buzzes; nothing is written to the notification history.
		//
		// A message is the one push with somewhere better to be counted. The
		// thread carries its own unread state, the inbox shows a dot per
		// conversation and the Messages tab carries the count, so a history row
		// would be the third place saying the same thing — and the one most
		// likely to bury a payout receipt under an afternoon of chat.
		data := map[string]string{
			"type":            "bulletin_message",
			"conversation_id": strconv.FormatInt(conversationId, 10),
		}

		subscriptions, err := a.db.GetMobilePushSubscriptionsByUser(ctx, recipientId)
		if err != nil {
			a.logger.Logf("error loading push subscriptions for %s: %s", recipientId, err)
			return
		}
		seen := map[string]struct{}{}
		for _, s := range subscriptions {
			if s == nil || !s.Active || !s.DeviceRegistered {
				continue
			}
			token := strings.TrimSpace(s.Token)
			if token == "" {
				continue
			}
			if _, dup := seen[token]; dup {
				continue
			}
			seen[token] = struct{}{}
			if _, err := sendExpoPushNotification(ctx, token, title, preview, data); err != nil {
				a.logger.Logf("error sending message push to %s: %s", recipientId, err)
			}
		}
	}()
}

// EditConversationMessage changes what a message says, within the edit window.
//
// Sender only, and the window is enforced in the UPDATE rather than checked
// first — see db.EditMessage for why a check-then-write leaves a gap a deadline
// can slip through.
func (a *AppService) EditConversationMessage(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return
	}
	messageId, ok := messageIdFromPath(w, r)
	if !ok {
		return
	}
	if !a.requireMessageInConversation(w, r, messageId, conv.Id) {
		return
	}

	var req structs.EditMessageRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<15)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read that edit.")
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		// Deliberately not treated as a delete. "Clear the text" and "remove
		// the message" are different intentions and the second has its own
		// button; inferring one from an empty field would delete somebody's
		// message because they selected all and hit send.
		bulletinError(w, http.StatusBadRequest, "empty", "A message can't be empty. Delete it instead.")
		return
	}
	if utf8.RuneCountInString(body) > messageMaxRunes {
		bulletinError(w, http.StatusBadRequest, "too_long", "Messages can be up to 2,000 characters.")
		return
	}

	ctx := r.Context()
	msg, err := a.db.EditMessage(ctx, messageId, userId, body)
	switch {
	case errors.Is(err, db.ErrEditWindowClosed):
		bulletinError(w, http.StatusConflict, "edit_window_closed",
			"That message is too old to edit.")
		return
	case errors.Is(err, db.ErrNotMyMessage):
		bulletinError(w, http.StatusForbidden, "not_yours", "You can only edit your own messages.")
		return
	case errors.Is(err, db.ErrConversationNotFound):
		bulletinError(w, http.StatusNotFound, "not_found", "That message could not be found.")
		return
	case err != nil:
		a.logger.Logf("error editing message %d: %s", messageId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	msg.IsMine = true
	a.hydrateMessages(ctx, conv, []*structs.Message{msg}, userId)
	// Published as a plain update rather than a new message: a client that
	// already has this id replaces it in place, and one that does not will pick
	// it up on its next read.
	a.publishMessageUpdated(ctx, conv, msg, userId)
	writeJSON(w, http.StatusOK, msg)
}

// DeleteConversationMessage removes a message for everybody, leaving a
// tombstone.
//
// Sender only. An admin removing reported content goes through the moderation
// path instead, which hides the message entirely rather than leaving a marker —
// see GetMessages for why those two outcomes differ.
func (a *AppService) DeleteConversationMessage(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return
	}
	messageId, ok := messageIdFromPath(w, r)
	if !ok {
		return
	}
	if !a.requireMessageInConversation(w, r, messageId, conv.Id) {
		return
	}

	ctx := r.Context()
	if err := a.db.DeleteOwnMessage(ctx, messageId, userId); err != nil {
		if errors.Is(err, db.ErrNotMyMessage) {
			bulletinError(w, http.StatusForbidden, "not_yours", "You can only delete your own messages.")
			return
		}
		a.logger.Logf("error deleting message %d: %s", messageId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.publishMessageDeleted(ctx, conv, messageId, userId)
	w.WriteHeader(http.StatusNoContent)
}

// messageIdFromPath reads {messageId}, writing a 404 when it is not a number.
func messageIdFromPath(w http.ResponseWriter, r *http.Request) (int64, bool) {
	messageId, err := strconv.ParseInt(chi.URLParam(r, "messageId"), 10, 64)
	if err != nil || messageId <= 0 {
		bulletinError(w, http.StatusNotFound, "not_found", "That message could not be found.")
		return 0, false
	}
	return messageId, true
}

// requireMessageInConversation checks that a message id from the path belongs to
// the conversation from the path.
//
// Both arrive independently, and membership of the conversation says nothing
// about the message — see the note on the reaction endpoint, which needs the
// same pair of checks for the same reason.
func (a *AppService) requireMessageInConversation(w http.ResponseWriter, r *http.Request, messageId, conversationId int64) bool {
	inThread, err := a.db.MessageIsInConversation(r.Context(), messageId, conversationId)
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}
	if !inThread {
		bulletinError(w, http.StatusNotFound, "not_found", "That message could not be found.")
		return false
	}
	return true
}
