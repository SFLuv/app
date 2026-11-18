package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/mail"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// Receiving email.
//
// We do not mean to take support by email: every line we open ourselves is a
// form or a chat. But people reply to the email we send, and write to support@
// and admin@ because those are the addresses printed on things, and until now
// all of that went into an inbox nobody was told about, or to no_reply@, which
// went nowhere. So mail is received like everything else: a Mailgun route
// forwards each message to this endpoint, it is kept as a support request with
// a reference, and the hub routes it by the address it was sent to.
//
// Set up outside the app (see docs/NOTIFICATION_HUB.md): MX records for the
// receiving domain, a Mailgun route forwarding to /mailgun/inbound, the
// webhook signing key in MAILGUN_WEBHOOK_SIGNING_KEY, and forwarding from the
// support@ and admin@ inboxes to the receiving domain.

const (
	inboundMaxBody      = 25 << 20
	inboundMaxSkew      = 15 * time.Minute
	inboundMaxSubject   = 200
	inboundTokenHistory = 2048
)

// inboundEmailSigningKey is Mailgun's HTTP webhook signing key.
func inboundEmailSigningKey() string {
	return strings.TrimSpace(os.Getenv("MAILGUN_WEBHOOK_SIGNING_KEY"))
}

// inboundEmailConfigured is whether the backend can verify, and so accept,
// forwarded mail.
func inboundEmailConfigured() bool {
	return inboundEmailSigningKey() != ""
}

// inboundTokens remembers recently used webhook tokens, so a captured request
// cannot be replayed inside the timestamp window. Per process, which is the
// window that matters: a replay against another instance would also need to
// fall inside fifteen minutes and still finds the message id already stored.
var inboundTokens = struct {
	sync.Mutex
	seen  map[string]time.Time
	order []string
}{seen: map[string]time.Time{}}

func inboundTokenFresh(token string) bool {
	inboundTokens.Lock()
	defer inboundTokens.Unlock()
	if _, used := inboundTokens.seen[token]; used {
		return false
	}
	inboundTokens.seen[token] = time.Now()
	inboundTokens.order = append(inboundTokens.order, token)
	if len(inboundTokens.order) > inboundTokenHistory {
		oldest := inboundTokens.order[0]
		inboundTokens.order = inboundTokens.order[1:]
		delete(inboundTokens.seen, oldest)
	}
	return true
}

// verifyMailgunSignature checks the signature Mailgun puts on every forward:
// HMAC-SHA256 of timestamp+token under the webhook signing key.
func verifyMailgunSignature(key, timestamp, token, signature string) bool {
	if key == "" || timestamp == "" || token == "" || signature == "" {
		return false
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if skew := time.Since(time.Unix(seconds, 0)); skew > inboundMaxSkew || skew < -inboundMaxSkew {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(timestamp + token))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(strings.ToLower(signature)))
}

// inboundHeaders reads Mailgun's message-headers field, a JSON list of
// [name, value] pairs, into a lookup by lower-cased name.
func inboundHeaders(raw string) map[string]string {
	out := map[string]string{}
	var pairs [][]string
	if json.Unmarshal([]byte(raw), &pairs) != nil {
		return out
	}
	for _, pair := range pairs {
		if len(pair) != 2 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(pair[0]))
		if _, seen := out[name]; !seen {
			out[name] = strings.TrimSpace(pair[1])
		}
	}
	return out
}

// inboundTopic is the topic a message is routed by: the mailbox it was sent
// to.
func inboundTopic(recipient string) (string, string) {
	for _, candidate := range strings.Split(recipient, ",") {
		address, err := mail.ParseAddress(strings.TrimSpace(candidate))
		if err != nil {
			continue
		}
		local := strings.ToLower(strings.SplitN(address.Address, "@", 2)[0])
		local = strings.SplitN(local, "+", 2)[0]
		switch local {
		case "support":
			return structs.NotificationTopicEmailSupport, address.Address
		case "admin":
			return structs.NotificationTopicEmailAdmin, address.Address
		case "no_reply", "noreply", "no-reply", "reply", "replies":
			return structs.NotificationTopicEmailReply, address.Address
		default:
			return structs.NotificationTopicEmailOther, address.Address
		}
	}
	return structs.NotificationTopicEmailOther, strings.TrimSpace(recipient)
}

// inboundFilterReason says why a message should be kept but not put in front
// of anybody: Mailgun thinks it is spam, it was sent by a machine, or it is our
// own mail coming back. Empty for mail from a person.
func inboundFilterReason(sender string, headers map[string]string, spamFlag string) string {
	if strings.EqualFold(strings.TrimSpace(spamFlag), "yes") {
		return "Mailgun flagged it as spam"
	}
	if auto := strings.ToLower(headers["auto-submitted"]); auto != "" && auto != "no" {
		return "sent automatically (" + auto + ")"
	}
	if headers["x-autoreply"] != "" || headers["x-autorespond"] != "" {
		return "an automatic reply"
	}
	switch strings.ToLower(headers["precedence"]) {
	case "bulk", "junk", "auto_reply":
		return "bulk or automatic mail"
	}
	lower := strings.ToLower(sender)
	if strings.HasPrefix(lower, "mailer-daemon@") || strings.HasPrefix(lower, "postmaster@") {
		return "a bounce"
	}
	if strings.EqualFold(lower, utils.NotificationFromEmail()) {
		return "our own mail coming back"
	}
	return ""
}

// supportReferencePattern finds one of our references in a subject, which is
// how a reply to a support thread says what it is about.
var supportReferencePattern = regexp.MustCompile(`\bSUP-[A-Z0-9]{6}\b`)

// ReceiveInboundEmail takes one message forwarded by a Mailgun route.
//
// Status codes are for Mailgun, which retries anything but a 2xx or a 406. A
// message we could not store gets a 500 so it is retried; one we will never
// take (a bad signature) gets a 406 so it is not.
func (a *AppService) ReceiveInboundEmail(w http.ResponseWriter, r *http.Request) {
	key := inboundEmailSigningKey()
	if key == "" {
		// Retried by Mailgun, so mail waits rather than being lost while this
		// is being set up.
		a.logger.Logf("inbound email: refused — MAILGUN_WEBHOOK_SIGNING_KEY is not set")
		http.Error(w, "inbound email is not configured", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, inboundMaxBody)
	if err := r.ParseMultipartForm(8 << 20); err != nil && err != http.ErrNotMultipart {
		a.logger.Logf("inbound email: could not read the forward: %s", err)
		http.Error(w, "could not read that", http.StatusBadRequest)
		return
	}
	if r.MultipartForm == nil {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "could not read that", http.StatusBadRequest)
			return
		}
	}
	field := func(name string) string { return strings.TrimSpace(r.FormValue(name)) }

	if !verifyMailgunSignature(key, field("timestamp"), field("token"), field("signature")) || !inboundTokenFresh(field("token")) {
		a.logger.Logf("inbound email: refused a forward with a bad, stale or reused signature")
		http.Error(w, "bad signature", http.StatusNotAcceptable)
		return
	}

	headers := inboundHeaders(r.FormValue("message-headers"))
	header := func(name string) string {
		if value := headers[strings.ToLower(name)]; value != "" {
			return value
		}
		return field(name)
	}

	messageId := header("Message-Id")
	if messageId == "" {
		messageId = "token:" + field("token")
	}
	if existing, err := a.db.SupportRequestByEmailMessageId(r.Context(), messageId); err != nil {
		a.logger.Logf("inbound email: %s", err)
		http.Error(w, "could not store that", http.StatusInternalServerError)
		return
	} else if existing != nil {
		writeJSON(w, http.StatusOK, map[string]string{"reference": existing.Reference, "duplicate": "true"})
		return
	}

	fromHeader := field("from")
	senderAddress := field("sender")
	senderName := ""
	if parsed, err := mail.ParseAddress(fromHeader); err == nil {
		senderAddress = parsed.Address
		senderName = parsed.Name
	}
	if replyTo := header("Reply-To"); replyTo != "" {
		if parsed, err := mail.ParseAddress(replyTo); err == nil {
			senderAddress = parsed.Address
		}
	}
	if senderAddress == "" {
		senderAddress = "unknown@unknown"
	}

	topic, mailbox := inboundTopic(field("recipient"))
	subject := truncate(oneLineText(field("subject")), inboundMaxSubject)
	message := field("stripped-text")
	if message == "" {
		message = field("body-plain")
	}
	if message == "" {
		message = "(No text. The message may be HTML only or attachments only.)"
	}
	message = truncate(message, supportMaxMessage)

	filtered := inboundFilterReason(field("sender"), headers, header("X-Mailgun-Sflag"))

	request, err := a.db.CreateSupportRequest(r.Context(), structs.SupportRequestSubmission{
		Email:   senderAddress,
		Name:    truncate(senderName, supportMaxName),
		Subject: subject,
		Message: message,
	}, structs.SupportSourceEmail, "")
	if err != nil {
		a.logger.Logf("inbound email from %s: could not store it: %s", senderAddress, err)
		http.Error(w, "could not store that", http.StatusInternalServerError)
		return
	}
	if recorded, err := a.db.SetSupportRequestEmailMessageId(r.Context(), request.Id, messageId); err != nil {
		a.logger.Logf("inbound email %s: %s", request.Reference, err)
	} else if !recorded {
		// A retry of the same delivery got there first and posted it. This
		// copy is archived so the queue shows the message once.
		_, _ = a.db.SetSupportRequestArchived(r.Context(), request.Id, true, "duplicate email delivery")
		writeJSON(w, http.StatusOK, map[string]string{"reference": request.Reference, "duplicate": "true"})
		return
	}

	fields := []structs.NotificationField{{Label: "To", Value: mailbox}}
	if subject != "" {
		fields = append(fields, structs.NotificationField{Label: "Subject", Value: subject})
	}
	if ref := supportReferencePattern.FindString(strings.ToUpper(field("subject"))); ref != "" && ref != request.Reference {
		fields = append(fields, structs.NotificationField{Label: "About", Value: ref})
	}
	if count, _ := strconv.Atoi(field("attachment-count")); count > 0 {
		fields = append(fields, structs.NotificationField{
			Label: "Attachments",
			Value: strconv.Itoa(count) + " not kept here; the original is in Mailgun's stored messages for three days",
		})
	}

	title := "Email to " + mailbox
	if topic == structs.NotificationTopicEmailReply {
		title = "Reply to our email"
	}
	if subject != "" {
		title += ": " + subject
	}

	opts := supportRequestAlert{Topic: topic, Title: title, Fields: fields}
	if filtered != "" {
		// Kept, archived, and announced only to whoever routes the filtered
		// topic, so nothing is thrown away and nobody is woken for it.
		opts.Topic = structs.NotificationTopicEmailFiltered
		opts.Informational = true
		opts.Fields = append(opts.Fields, structs.NotificationField{Label: "Filtered", Value: filtered})
		_, _ = a.db.SetSupportRequestArchived(r.Context(), request.Id, true, "mail filter: "+filtered)
	}
	a.postSupportRequestAlert(r.Context(), request, opts)

	a.logger.Logf("inbound email %s from %s to %s%s", request.Reference, senderAddress, mailbox,
		map[bool]string{true: " (filtered: " + filtered + ")", false: ""}[filtered != ""])
	writeJSON(w, http.StatusOK, map[string]string{"reference": request.Reference})
}
