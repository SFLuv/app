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

func otherMember(c *structs.ConversationRow, me string) string {
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
	for _, row := range rows {
		others = append(others, otherMember(&row.Conversation, userId))
		if row.Conversation.PostId != nil {
			postIds = append(postIds, *row.Conversation.PostId)
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

	out := make([]*structs.Conversation, 0, len(rows))
	for _, row := range rows {
		other := otherMember(&row.Conversation, userId)
		conv := &structs.Conversation{
			Id:            row.Conversation.Id,
			OtherUser:     people[other],
			LastMessage:   row.Last,
			UnreadCount:   row.Unread,
			LastMessageAt: row.Conversation.LastMessageAt,
			BlockedByMe:   blocked[other],
		}
		if conv.LastMessage != nil {
			conv.LastMessage.IsMine = conv.LastMessage.SenderId == userId
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
	if !active || blocked {
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
	other := otherMember(conv, userId)
	people, err := a.db.GetPublicUsers(ctx, []string{other})
	if err != nil {
		a.logger.Logf("bulletin: unexpected error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out := &structs.Conversation{
		Id:            conv.Id,
		OtherUser:     people[other],
		LastMessageAt: conv.LastMessageAt,
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
	writeJSON(w, http.StatusOK, structs.MessagesPage{Messages: msgs, HasMore: hasMore})
}

// requireCanMessage blocks sending into a thread when either person has
// blocked the other, or the sender is suspended or has no profile.
func (a *AppService) requireCanMessage(w http.ResponseWriter, r *http.Request, userId string, conv *structs.ConversationRow) bool {
	if !a.requireBulletinParticipant(w, r, userId) {
		return false
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
	})
	if err != nil {
		a.logger.Logf("error sending message in conversation %d: %s", conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	msg.IsMine = true
	a.notifyNewMessage(userId, otherMember(conv, userId), conv.Id, previewText(body))
	writeJSON(w, http.StatusCreated, msg)
}

// AttachConversationPayment records an SFLUV payment the sender already made
// through the normal send flow. The transfer is checked against the indexer:
// it must go from a wallet the sender owns to a wallet the other person owns.
// If the indexer has not seen it yet, it is stored as pending and checked again
// when the thread is next read.
func (a *AppService) AttachConversationPayment(w http.ResponseWriter, r *http.Request) {
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
	var req structs.PaymentMessageRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read the payment.")
		return
	}
	hash := strings.ToLower(strings.TrimSpace(req.TxHash))
	if !txHashPattern.MatchString(hash) {
		bulletinError(w, http.StatusBadRequest, "bad_tx", "That isn't a valid transaction.")
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
	}
	msg, err := a.db.InsertMessage(r.Context(), &db.NewMessage{
		ConversationId:  conv.Id,
		SenderId:        userId,
		Kind:            structs.MessageKindPayment,
		TxHash:          &hash,
		AmountSfluvBase: amountPtr,
		PaymentVerified: verified,
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
	a.notifyNewMessage(userId, other, conv.Id, preview)
	writeJSON(w, http.StatusCreated, msg)
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
// slow push service never delays the sender.
func (a *AppService) notifyNewMessage(senderId, recipientId string, conversationId int64, preview string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		people, err := a.db.GetPublicUsers(ctx, []string{senderId})
		if err != nil {
			return
		}
		title := people[senderId].PublicName
		if title == "" {
			title = "New message"
		}
		data := map[string]string{
			"type":            "bulletin_message",
			"conversation_id": strconv.FormatInt(conversationId, 10),
		}
		a.recordNotification(ctx, recipientId, "bulletin_message", title, preview, data)

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
