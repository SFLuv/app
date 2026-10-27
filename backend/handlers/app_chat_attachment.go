package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
	"github.com/go-chi/chi/v5"
	"golang.org/x/net/html"
)

const (
	// 25 MB, matching the CHECK in the 1.74 migration. One number in two
	// places is a risk; the alternative is a round trip to discover a limit
	// that never changes at runtime.
	chatMaxAttachmentBytes = 25 << 20
	// Images get a tighter limit than files, because an image is displayed
	// inline and a 25 MB photo is a phone camera's raw output rather than
	// something anybody meant to send.
	chatMaxImageBytes     = 8 << 20
	chatMaxAttachments    = 10
	chatCaptionMaxRunes   = 2000
	chatLinkFetchTimeout  = 6 * time.Second
	chatLinkMaxFetchBytes = 512 << 10
	chatLinkTextMaxRunes  = 300
	chatFileNameMaxRunes  = 160
)

// What may be sent as a file.
//
// An allowlist, not a blocklist. The alternative — refusing .exe and friends —
// is a losing game, and these are served back to other people's phones. The set
// is the formats somebody actually shares in a neighborhood chat; anything else
// is refused with a message naming what is allowed rather than silently
// dropped.
var chatAllowedFileTypes = map[string]string{
	".pdf":  "application/pdf",
	".csv":  "text/csv",
	".txt":  "text/plain",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".ppt":  "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".rtf":  "application/rtf",
	".zip":  "application/zip",
	".heic": "image/heic",
	".ics":  "text/calendar",
}

// UploadConversationAttachment stores photos or files and posts them as one
// message.
//
// Multipart, several files at once, because sending four photos is one action to
// the person doing it and four messages would read as four.
func (a *AppService) UploadConversationAttachment(w http.ResponseWriter, r *http.Request) {
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

	if err := r.ParseMultipartForm(chatMaxAttachmentBytes + (1 << 20)); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_upload", "Could not read the upload.")
		return
	}
	defer r.MultipartForm.RemoveAll()

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		// "file" as well as "files", so a client sending one does not have to
		// know it is the plural field.
		files = r.MultipartForm.File["file"]
	}
	if len(files) == 0 {
		bulletinError(w, http.StatusBadRequest, "no_files", "Choose a photo or file to send.")
		return
	}
	if len(files) > chatMaxAttachments {
		bulletinError(w, http.StatusBadRequest, "too_many",
			fmt.Sprintf("Up to %d files at a time.", chatMaxAttachments))
		return
	}

	caption := strings.TrimSpace(r.FormValue("body"))
	if utf8.RuneCountInString(caption) > chatCaptionMaxRunes {
		bulletinError(w, http.StatusBadRequest, "too_long", "Captions can be up to 2,000 characters.")
		return
	}
	var replyTo *int64
	if raw := strings.TrimSpace(r.FormValue("reply_to_id")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			replyTo = &parsed
		}
	}

	attachments := make([]db.NewAttachment, 0, len(files))
	for position, header := range files {
		attachment, problem := readUploadedAttachment(header, position)
		if problem != "" {
			bulletinError(w, http.StatusBadRequest, "bad_file", problem)
			return
		}
		attachments = append(attachments, *attachment)
	}

	msg, err := a.db.InsertMessageWithAttachments(r.Context(), &db.NewMessage{
		ConversationId: conv.Id,
		SenderId:       userId,
		Kind:           structs.MessageKindAttachment,
		Body:           caption,
		ReplyToId:      replyTo,
	}, attachments)
	if err != nil {
		a.logger.Logf("error storing attachments in conversation %d: %s", conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	msg.IsMine = true
	a.hydrateMessages(r.Context(), conv, []*structs.Message{msg}, userId)
	a.fanOutMessage(r.Context(), conv, msg, userId, attachmentPreview(attachments, caption))
	writeJSON(w, http.StatusCreated, msg)
}

// readUploadedAttachment validates one uploaded file and reads its bytes.
//
// Returns a sentence for the client on refusal, rather than an error: every
// rejection here is something the person can fix, and "that file type isn't
// supported" is more use than a 400 with no body.
func readUploadedAttachment(header *multipart.FileHeader, position int) (*db.NewAttachment, string) {
	file, err := header.Open()
	if err != nil {
		return nil, "Could not read that file."
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, chatMaxAttachmentBytes+1))
	if err != nil || len(data) == 0 {
		return nil, "That file was empty."
	}
	if len(data) > chatMaxAttachmentBytes {
		return nil, "That file is too large. The limit is 25 MB."
	}

	name := sanitizeFileName(header.Filename)
	size := len(data)

	// An image is anything that sniffs as one, whatever it is called. Checking
	// the bytes rather than the extension is what stops a .png that is really a
	// document being served back with an image content type.
	if contentType := locationIconContentType(data); contentType != "" {
		if size > chatMaxImageBytes {
			return nil, "That image is too large. The limit is 8 MB."
		}
		return &db.NewAttachment{
			Kind:        structs.AttachmentKindImage,
			Position:    position,
			ContentType: contentType,
			FileName:    name,
			FileData:    data,
			SizeBytes:   &size,
		}, ""
	}

	extension := strings.ToLower(filepath.Ext(name))
	contentType, allowed := chatAllowedFileTypes[extension]
	if !allowed {
		return nil, "That file type isn't supported. Try a PDF, document, spreadsheet, or image."
	}

	return &db.NewAttachment{
		Kind:        structs.AttachmentKindFile,
		Position:    position,
		ContentType: contentType,
		FileName:    name,
		FileData:    data,
		SizeBytes:   &size,
	}, ""
}

// sanitizeFileName keeps the name a person recognises and nothing a filesystem
// could act on.
//
// The name is only ever a label and a Content-Disposition value — nothing is
// written to disk — but it is echoed back to other clients, so a path or a
// newline in it has no business surviving.
func sanitizeFileName(raw string) string {
	name := filepath.Base(strings.TrimSpace(raw))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' || r == '"' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "attachment"
	}
	if utf8.RuneCountInString(name) > chatFileNameMaxRunes {
		name = truncateRunes(name, chatFileNameMaxRunes)
	}
	return name
}

// attachmentPreview is the push text for a message whose content is its cards.
func attachmentPreview(attachments []db.NewAttachment, caption string) string {
	if caption != "" {
		return previewText(caption)
	}
	images, files := 0, 0
	for _, at := range attachments {
		if at.Kind == structs.AttachmentKindImage {
			images++
		} else {
			files++
		}
	}
	switch {
	case images > 0 && files == 0 && images == 1:
		return "Sent a photo"
	case images > 0 && files == 0:
		return fmt.Sprintf("Sent %d photos", images)
	case files == 1 && images == 0:
		return "Sent a file"
	case files > 0 && images == 0:
		return fmt.Sprintf("Sent %d files", files)
	default:
		return "Sent an attachment"
	}
}

// GetConversationAttachment serves an attachment's bytes to a member of the
// conversation it belongs to.
func (a *AppService) GetConversationAttachment(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	attachmentId, err := strconv.ParseInt(chi.URLParam(r, "attachmentId"), 10, 64)
	if err != nil || attachmentId <= 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Membership is enforced inside the query, not here — see the comment on
	// GetAttachmentBlobForMember. The conversation id in the path is not
	// consulted for authorization at all, which is why a mismatched one cannot
	// be used to reach somebody else's file.
	blob, err := a.db.GetAttachmentBlobForMember(r.Context(), attachmentId, userId)
	if errors.Is(err, db.ErrAttachmentNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		a.logger.Logf("error loading attachment %d: %s", attachmentId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", blob.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	// nosniff matters more here than on the other file routes: this is the one
	// that accepts arbitrary types from one user and serves them to another.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// inline for images so a thread can render them; attachment for everything
	// else so a document opens in a viewer rather than executing in a webview.
	disposition := "attachment"
	if strings.HasPrefix(blob.ContentType, "image/") {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("%s; filename*=UTF-8''%s", disposition, url.PathEscape(blob.FileName)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(blob.Data)
}

// AttachConversationReference posts a message whose content is a pointer at
// something else: a transaction, a merchant, an event, a support ticket, or a
// link to scrape.
func (a *AppService) AttachConversationReference(w http.ResponseWriter, r *http.Request) {
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

	var req structs.AttachmentReferenceRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<15)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read that request.")
		return
	}
	if !structs.IsAttachmentKind(req.Kind) {
		bulletinError(w, http.StatusBadRequest, "bad_kind", "That isn't something that can be attached.")
		return
	}
	if req.Kind == structs.AttachmentKindImage || req.Kind == structs.AttachmentKindFile {
		bulletinError(w, http.StatusBadRequest, "bad_kind",
			"Photos and files are sent to the upload endpoint.")
		return
	}

	caption := strings.TrimSpace(req.Body)
	if utf8.RuneCountInString(caption) > chatCaptionMaxRunes {
		bulletinError(w, http.StatusBadRequest, "too_long", "Captions can be up to 2,000 characters.")
		return
	}

	ctx := r.Context()
	attachment := db.NewAttachment{Kind: req.Kind}
	preview := ""

	switch req.Kind {
	case structs.AttachmentKindTransaction:
		hash := strings.TrimSpace(req.TxHash)
		if !txHashPattern.MatchString(hash) {
			bulletinError(w, http.StatusBadRequest, "bad_hash", "That isn't a transaction hash.")
			return
		}
		// Deliberately NOT checked for being the sender's own transfer.
		//
		// AttachConversationPayment does check, because it claims "I paid you"
		// and the claim has to be true. This claims only "here is a
		// transaction", which is a pointer at public chain data — the same thing
		// as pasting an explorer link, and no more sensitive. What it must not
		// do is carry the memo, and resolveTransactionCard is where that is
		// enforced.
		attachment.TxHash = &hash
		preview = "Shared a transaction"

	case structs.AttachmentKindMerchant:
		if req.LocationId == nil || *req.LocationId <= 0 {
			bulletinError(w, http.StatusBadRequest, "bad_reference", "Choose a merchant.")
			return
		}
		location, err := a.db.GetLocation(ctx, uint64(*req.LocationId))
		if err != nil || location == nil || !location.Approval {
			bulletinError(w, http.StatusBadRequest, "bad_reference", "That merchant could not be found.")
			return
		}
		attachment.LocationId = req.LocationId
		preview = "Shared " + location.Name

	case structs.AttachmentKindEvent:
		eventId := strings.TrimSpace(req.EventId)
		if eventId == "" {
			bulletinError(w, http.StatusBadRequest, "bad_reference", "Choose an event.")
			return
		}
		event, _ := a.lookupLinkedEvent(ctx, eventId)
		if event == nil {
			bulletinError(w, http.StatusBadRequest, "bad_reference", "That event could not be found.")
			return
		}
		attachment.EventId = &eventId
		preview = "Shared " + event.Title

	case structs.AttachmentKindSupport:
		if req.SupportRequestId == nil || *req.SupportRequestId <= 0 {
			bulletinError(w, http.StatusBadRequest, "bad_reference", "Choose a support request.")
			return
		}
		// Only staff may attach a support ticket: it is the staff-side half of
		// the reply flow, and a ticket reference in a chat is a claim to be
		// answering it. Anybody in the thread can then SEE it, which is the
		// point.
		if !a.IsAdmin(ctx, userId) {
			bulletinError(w, http.StatusForbidden, "not_allowed", "Only SFLuv staff can attach a support request.")
			return
		}
		request, err := a.db.GetSupportRequest(ctx, *req.SupportRequestId)
		if err != nil || request == nil {
			bulletinError(w, http.StatusBadRequest, "bad_reference", "That support request could not be found.")
			return
		}
		attachment.SupportRequestId = req.SupportRequestId
		preview = "About your message to SFLuv"

	case structs.AttachmentKindLink:
		target, problem := normalizeLinkURL(req.LinkURL)
		if problem != "" {
			bulletinError(w, http.StatusBadRequest, "bad_link", problem)
			return
		}
		attachment.LinkURL = target
		// Scraped now and stored, never re-fetched. A card that changed under a
		// message would be a way to rewrite what somebody said.
		meta := a.fetchLinkMetadata(ctx, target)
		attachment.LinkTitle = meta.Title
		attachment.LinkDescription = meta.Description
		attachment.LinkImageURL = meta.ImageURL
		attachment.LinkSiteName = meta.SiteName
		preview = meta.Title
		if preview == "" {
			preview = target
		}
	}

	if caption != "" {
		preview = previewText(caption)
	}

	msg, err := a.db.InsertMessageWithAttachments(ctx, &db.NewMessage{
		ConversationId: conv.Id,
		SenderId:       userId,
		Kind:           structs.MessageKindAttachment,
		Body:           caption,
		ReplyToId:      req.ReplyToId,
	}, []db.NewAttachment{attachment})
	if err != nil {
		a.logger.Logf("error attaching a %s reference in conversation %d: %s", req.Kind, conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	msg.IsMine = true
	a.hydrateMessages(ctx, conv, []*structs.Message{msg}, userId)
	a.fanOutMessage(ctx, conv, msg, userId, preview)
	writeJSON(w, http.StatusCreated, msg)
}

// SearchConversationFiles lists the photos and files shared in a thread, for the
// info screen.
func (a *AppService) SearchConversationFiles(w http.ResponseWriter, r *http.Request) {
	userId, ok := bulletinViewer(w, r)
	if !ok {
		return
	}
	conv, ok := a.loadMemberConversation(w, r, userId)
	if !ok {
		return
	}

	files, err := a.db.SearchConversationFiles(r.Context(), conv.Id, r.URL.Query().Get("q"), 100)
	if err != nil {
		a.logger.Logf("error listing files in conversation %d: %s", conv.Id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	senderIds := make([]string, 0, len(files))
	for _, f := range files {
		senderIds = append(senderIds, f.SenderId)
	}
	names, _ := a.db.GetSenderNames(r.Context(), senderIds)

	type fileRow struct {
		structs.MessageAttachment
		MessageId  int64  `json:"message_id"`
		SenderId   string `json:"sender_id"`
		SenderName string `json:"sender_name,omitempty"`
		CreatedAt  int64  `json:"created_at"`
	}
	out := make([]fileRow, 0, len(files))
	for _, f := range files {
		out = append(out, fileRow{
			MessageAttachment: f.Attachment,
			MessageId:         f.MessageId,
			SenderId:          f.SenderId,
			SenderName:        names[f.SenderId],
			CreatedAt:         f.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// ── Link metadata ────────────────────────────────────────────────────────────

type linkMetadata struct {
	Title       string
	Description string
	ImageURL    string
	SiteName    string
}

// normalizeLinkURL accepts what a person would paste and returns something safe
// to fetch, or a sentence saying why not.
//
// ── Why the scheme is decided by parsing, not by looking for "://" ──────────
//
// The obvious version of this prepends "https://" to anything without "://".
// That is wrong in a way that is easy to miss and bad when it bites:
// "mailto:a@b.com" contains no "://", so it becomes "https://mailto:a@b.com",
// which is a perfectly valid URL whose host is b.com and whose USERINFO is
// "mailto:a". So a pasted mailto turned into an authenticated fetch of somebody
// else's domain, with a credential pair in the request line — from a URL a user
// supplied, on the one path in this backend that fetches user-supplied URLs.
//
// Parsing first and only defaulting the scheme when there genuinely is none
// fixes that class, and userinfo is refused outright rather than stripped:
// nothing legitimate needs it in a shared link, and quietly removing it would
// turn a hostile input into a successful request to a host the person naming
// the credentials chose.
func normalizeLinkURL(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "Paste a link first."
	}
	if len(raw) > 2048 {
		return "", "That link is too long."
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "That doesn't look like a link."
	}
	// No scheme at all — "example.org/thing" — lands entirely in Path. Only
	// then is https assumed.
	if parsed.Scheme == "" {
		parsed, err = url.Parse("https://" + raw)
		if err != nil {
			return "", "That doesn't look like a link."
		}
	}

	// http and https only. A mailto, a tel, or an app scheme in a card that
	// another person taps is a way to make their phone do something they did
	// not choose.
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "Only web links can be shared."
	}
	if parsed.Host == "" {
		return "", "That doesn't look like a link."
	}
	if parsed.User != nil {
		return "", "That doesn't look like a link."
	}
	return parsed.String(), ""
}

// fetchLinkMetadata reads a page's Open Graph tags.
//
// ── Why this is written so defensively ──────────────────────────────────────
//
// This is the only place in the backend where a user supplies a URL and the
// server fetches it. That is server-side request forgery by construction, and
// the mitigations are all here:
//
//   - A dialler that refuses to connect to a private, loopback, link-local or
//     unspecified address. This is checked at CONNECT time, on the resolved IP,
//     not by inspecting the hostname — a hostname check is defeated by a DNS
//     record pointing at 169.254.169.254, and checking the resolved address is
//     what closes that.
//   - Redirects re-checked the same way, since a public URL may redirect to a
//     private one.
//   - A short timeout and a read cap, so a slow or enormous page cannot hold a
//     request or fill memory.
//   - No credentials, no cookies, and the response is parsed for four strings
//     and thrown away.
//
// A failure returns empty metadata rather than an error: the link is still
// shared, just as a bare URL. Somebody pasting a link to something only they can
// reach should get a plain link, not a refusal.
func (a *AppService) fetchLinkMetadata(ctx context.Context, target string) linkMetadata {
	ctx, cancel := context.WithTimeout(ctx, chatLinkFetchTimeout)
	defer cancel()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext:         safeDialContext,
			DisableKeepAlives:   true,
			TLSHandshakeTimeout: chatLinkFetchTimeout,
		},
		// Followed, but every hop goes through safeDialContext again.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("refusing a non-web redirect")
			}
			return nil
		},
		Timeout: chatLinkFetchTimeout,
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return linkMetadata{}
	}
	// Identifying the fetcher is the polite thing and makes us blockable, which
	// is correct: a site that does not want previews built should be able to say
	// so.
	request.Header.Set("User-Agent", "SFLuvBot/1.0 (+https://sfluv.org; link previews)")
	request.Header.Set("Accept", "text/html,application/xhtml+xml")

	response, err := client.Do(request)
	if err != nil {
		return linkMetadata{}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return linkMetadata{}
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "" &&
		!strings.Contains(strings.ToLower(contentType), "html") {
		return linkMetadata{}
	}

	meta := parseLinkMetadata(io.LimitReader(response.Body, chatLinkMaxFetchBytes))
	if meta.SiteName == "" {
		if parsed, err := url.Parse(target); err == nil {
			meta.SiteName = strings.TrimPrefix(parsed.Host, "www.")
		}
	}
	// A relative og:image is resolved against the page, since the client will
	// load it directly.
	if meta.ImageURL != "" {
		if base, err := url.Parse(target); err == nil {
			if resolved, err := base.Parse(meta.ImageURL); err == nil &&
				(resolved.Scheme == "http" || resolved.Scheme == "https") {
				meta.ImageURL = resolved.String()
			} else {
				meta.ImageURL = ""
			}
		}
	}
	return meta
}

// safeDialContext refuses to connect to anything that is not a public address.
//
// The check is on the address being dialled, after resolution, which is the only
// place it works: by this point the hostname has already become an IP, so a DNS
// answer pointing inward is caught here rather than trusted.
func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{Timeout: chatLinkFetchTimeout}
	var lastErr error = errors.New("no usable address")
	for _, candidate := range addresses {
		if !isPublicIP(candidate.IP) {
			lastErr = fmt.Errorf("refusing to connect to %s", candidate.IP)
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// isPublicIP reports whether an address is one we are willing to fetch from.
func isPublicIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	// Carrier-grade NAT (100.64.0.0/10) is not covered by IsPrivate and is
	// where a cloud provider's internal services often sit.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return false
	}
	return true
}

// parseLinkMetadata pulls the four strings a card needs out of a page's head.
//
// Open Graph first, then Twitter cards, then the plain <title> and
// <meta name="description">, which is the order of how reliable they are.
// Stops at </head>: everything it wants is there, and parsing a whole page to
// find nothing more is wasted work on a path with a timeout.
func parseLinkMetadata(body io.Reader) linkMetadata {
	meta := linkMetadata{}
	tokenizer := html.NewTokenizer(body)
	inTitle := false

	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return meta
		case html.TextToken:
			if inTitle && meta.Title == "" {
				meta.Title = collapseWhitespace(string(tokenizer.Text()))
			}
		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			switch string(name) {
			case "title":
				inTitle = false
			case "head":
				return meta
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := tokenizer.TagName()
			switch string(name) {
			case "title":
				inTitle = true
			case "body":
				return meta
			case "meta":
				if !hasAttr {
					continue
				}
				property, content := "", ""
				for {
					key, value, more := tokenizer.TagAttr()
					switch strings.ToLower(string(key)) {
					case "property", "name":
						property = strings.ToLower(string(value))
					case "content":
						content = string(value)
					}
					if !more {
						break
					}
				}
				content = collapseWhitespace(content)
				if content == "" {
					continue
				}
				switch property {
				case "og:title", "twitter:title":
					meta.Title = content
				case "og:description", "twitter:description", "description":
					if meta.Description == "" || strings.HasPrefix(property, "og:") {
						meta.Description = content
					}
				case "og:image", "og:image:secure_url", "twitter:image":
					if meta.ImageURL == "" {
						meta.ImageURL = content
					}
				case "og:site_name":
					meta.SiteName = content
				}
			}
		}

		meta.Title = truncateRunes(meta.Title, chatLinkTextMaxRunes)
		meta.Description = truncateRunes(meta.Description, chatLinkTextMaxRunes)
	}
}

func collapseWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// PreviewLink scrapes a URL's metadata without posting anything.
//
// Exists so the composer can show the card BEFORE the message is sent — the
// attachment tray's "Link" option was removed in favour of detecting a pasted
// URL in the text field, and a card that only appeared after sending gave
// nobody a chance to see what they were about to share.
//
// ── Why this is safe to expose, and what it still costs ─────────────────────
//
// It is the same fetcher AttachConversationReference already uses, with the
// same guards: http(s) only, no credentials in the URL, a resolved-IP check
// that refuses private, loopback, link-local and carrier-grade-NAT addresses,
// redirects re-checked per hop, a six-second timeout and a 512 KB read cap.
//
// What is genuinely new is that it can be called without sending a message, so
// it is a slightly cheaper way to make us fetch a URL. Mitigated by being
// behind auth AND the chat beta, and bounded by the same timeout — but it is
// the reason this is a separate endpoint rather than a general-purpose one, and
// the reason it returns metadata only and never the page.
//
// An unreachable or hostile URL returns empty metadata with 200, not an error:
// the composer should fall back to sending a plain link, not block the message.
func (a *AppService) PreviewLink(w http.ResponseWriter, r *http.Request) {
	if _, ok := bulletinViewer(w, r); !ok {
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<13)).Decode(&req); err != nil {
		bulletinError(w, http.StatusBadRequest, "bad_request", "Could not read that request.")
		return
	}

	target, problem := normalizeLinkURL(req.URL)
	if problem != "" {
		bulletinError(w, http.StatusBadRequest, "bad_link", problem)
		return
	}

	meta := a.fetchLinkMetadata(r.Context(), target)
	writeJSON(w, http.StatusOK, map[string]string{
		"url":         target,
		"title":       meta.Title,
		"description": meta.Description,
		"image_url":   meta.ImageURL,
		"site_name":   meta.SiteName,
	})
}
