package handlers

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// ── Answering from the notification ─────────────────────────────────────────
//
// iOS lets somebody answer a chat push without opening the app: pull the
// banner down, type or dictate, Send. Whatever handles that reply cannot count
// on the app's session. The app is woken in the background and may not have
// finished starting; the notification's own content extension (2.5) never has
// a session at all.
//
// So a chat push carries a signed thread URL, as it carries signed picture
// URLs (push_images.go). It names one conversation and the one person the push
// was for, and until it expires it lets them do two things there:
//
//   GET   /push-thread/{id}   the newest page of messages, which the expanded
//                             notification shows above the reply field;
//   POST  /push-thread/{id}   a text reply, sent as them.
//
// Both run the ordinary handlers, so membership, blocks, the length limit and
// everything else they check is checked here too. The signature only says who
// the push was for; this re-checks that they may still read and post — an
// active account, the privacy policy accepted, the chat beta — as the session
// routes do.
//
// A forged, expired or altered URL is a bare 404, as with pictures.

// pushThreadTTL matches the pictures': a push that sat on a phone that was off
// over a weekend can still be answered.
const pushThreadTTL = 72 * time.Hour

// pushThreadURL is the signed thread URL for one recipient, or "" when this
// server has no public origin to put in it.
func pushThreadURL(conversationId int64, recipientId string) string {
	base := publicBackendBase()
	if base == "" || conversationId <= 0 || recipientId == "" {
		return ""
	}
	id := strconv.FormatInt(conversationId, 10)
	exp := strconv.FormatInt(time.Now().Add(pushThreadTTL).Unix(), 10)
	q := url.Values{}
	q.Set("r", recipientId)
	q.Set("exp", exp)
	q.Set("sig", signPushImage("t", id, recipientId, exp))
	return base + "/push-thread/" + id + "?" + q.Encode()
}

// asPushThreadRecipient checks a signed thread URL and, if it holds, runs next
// as the person it was signed for.
func (a *AppService) asPushThreadRecipient(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		id, recipient, exp, sig := chi.URLParam(r, "id"), q.Get("r"), q.Get("exp"), q.Get("sig")
		expires, err := strconv.ParseInt(exp, 10, 64)
		if err != nil || time.Now().Unix() > expires || id == "" || recipient == "" || sig == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		want := signPushImage("t", id, recipient, exp)
		if subtle.ConstantTimeCompare([]byte(want), []byte(sig)) != 1 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		ctx := r.Context()
		if !a.UserIsActive(ctx, recipient) ||
			!a.UserHasAcceptedPrivacyPolicy(ctx, recipient) ||
			!a.BetaFeatureAllowed(ctx, recipient, structs.BetaFeatureChat) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next(w, r.WithContext(context.WithValue(ctx, "userDid", recipient)))
	}
}

// GetPushThread is the newest page of a thread, for the expanded notification.
func (a *AppService) GetPushThread(w http.ResponseWriter, r *http.Request) {
	a.asPushThreadRecipient(a.GetConversationMessages)(w, r)
}

// PostPushThreadReply sends a reply typed into the notification. The body is
// the ordinary send's, {"body": "..."}.
//
// Sending marks the thread read for the sender, so a reply clears the thread's
// unread messages — which the phone did not see happen. Its icon is told.
func (a *AppService) PostPushThreadReply(w http.ResponseWriter, r *http.Request) {
	a.asPushThreadRecipient(func(w http.ResponseWriter, r *http.Request) {
		recorder := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		a.SendConversationMessage(recorder, r)
		if recorder.Status() == http.StatusCreated {
			if userId, ok := r.Context().Value("userDid").(string); ok {
				a.syncBadge(userId)
			}
		}
	})(w, r)
}
