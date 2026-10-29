package handlers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
)

// announcement is how one new message is described to the people it is for.
//
// Built once, where the message is created, and then used for the push AND the
// in-app banner AND nothing else gets to improvise: the two used to be worded
// separately, and drifted — a photo with a caption pushed as just the caption,
// a file pushed as "Sent a file" while the inbox named it, a payment banner
// said "Sent you SFLUV" with no amount. fanOutMessage adds who sent it.
type announcement struct {
	// Line is what a notification prints, without the sender:
	// "📷 Photo · look at this", "🔗 sfluv.org", "hello".
	Line string
	// Spoken is the same message as a verb phrase for a screen reader, with no
	// emoji for it to pronounce: "said: hello", "sent a photo, and said: look
	// at this". The sender's name goes in front of it.
	Spoken string
}

// withCaption folds what the sender typed into an attachment's description.
// The kind stays at the front, so a photo with a caption is still visibly a
// photo; it used to be replaced by the caption outright.
func (an announcement) withCaption(caption string) announcement {
	caption = previewText(caption)
	if caption == "" {
		return an
	}
	return announcement{
		Line:   an.Line + " · " + caption,
		Spoken: an.Spoken + ", and said: " + caption,
	}
}

// soleLinkPattern recognises a message that is nothing but one link. Mirrors
// the client's src/utils/links.ts: an explicit scheme or www. always counts, a
// bare host only for common TLDs (with an optional two-letter country part, as
// in bbc.co.uk), so "notes.md" stays a note.
var soleLinkPattern = regexp.MustCompile(
	`(?i)^(?:https?://\S+|www\.\S+|(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+(?:com|org|net|edu|gov|io|co|app|dev|ai|me|us|ly|info|biz|xyz|so|gg|fm|tv|to|news|page|site|link|city|community|social)(?:\.[a-z]{2})?(?:/\S*)?)$`,
)

var linkSchemePattern = regexp.MustCompile(`(?i)^https?://`)

// linkHost is the part of a URL a person recognises: "sfluv.org".
func linkHost(raw string) string {
	host := strings.TrimSpace(raw)
	host = linkSchemePattern.ReplaceAllString(host, "")
	host = strings.TrimPrefix(strings.TrimPrefix(host, "www."), "WWW.")
	if cut := strings.IndexAny(host, "/?#:"); cut >= 0 {
		host = host[:cut]
	}
	return host
}

// textAnnouncement describes a plain text message. A message that is only a
// link reads as the link it is rather than as a raw URL — links are sent as
// text now and rendered as cards on the client, so this is the one place a
// notification gets to say "a link" instead of "https://…".
func textAnnouncement(body string) announcement {
	trimmed := strings.TrimRight(strings.TrimSpace(body), ".,;:!?")
	if soleLinkPattern.MatchString(trimmed) {
		host := linkHost(trimmed)
		return announcement{Line: "🔗 " + host, Spoken: "shared a link to " + host}
	}
	text := previewText(body)
	return announcement{Line: text, Spoken: "said: " + text}
}

// attachmentsAnnouncement describes photos and files, named where one is.
// A video is a file to the server, but nobody calls IMG_0042.MOV by its name,
// so it is named for what it is. src/screens/MessagesScreen.tsx words the
// inbox's preview the same way.
func attachmentsAnnouncement(attachments []db.NewAttachment, caption string) announcement {
	images, videos, files := 0, 0, 0
	fileName := ""
	for _, at := range attachments {
		switch {
		case at.Kind == structs.AttachmentKindImage:
			images++
		case strings.HasPrefix(strings.ToLower(at.ContentType), "video/"):
			videos++
		default:
			files++
			fileName = strings.TrimSpace(at.FileName)
		}
	}
	var an announcement
	switch {
	case images == 1 && videos == 0 && files == 0:
		an = announcement{Line: "📷 Photo", Spoken: "sent a photo"}
	case images > 1 && videos == 0 && files == 0:
		an = announcement{Line: fmt.Sprintf("📷 %d photos", images), Spoken: fmt.Sprintf("sent %d photos", images)}
	case videos == 1 && images == 0 && files == 0:
		an = announcement{Line: "🎥 Video", Spoken: "sent a video"}
	case videos > 1 && images == 0 && files == 0:
		an = announcement{Line: fmt.Sprintf("🎥 %d videos", videos), Spoken: fmt.Sprintf("sent %d videos", videos)}
	case images+videos > 0 && files == 0:
		an = announcement{
			Line:   fmt.Sprintf("📷 %d photos and videos", images+videos),
			Spoken: fmt.Sprintf("sent %d photos and videos", images+videos),
		}
	case files == 1 && images+videos == 0 && fileName != "":
		an = announcement{Line: "📎 " + fileName, Spoken: "sent a file, " + fileName}
	case files == 1 && images+videos == 0:
		an = announcement{Line: "📎 File", Spoken: "sent a file"}
	case files > 1 && images+videos == 0:
		an = announcement{Line: fmt.Sprintf("📎 %d files", files), Spoken: fmt.Sprintf("sent %d files", files)}
	default:
		total := images + videos + files
		an = announcement{Line: fmt.Sprintf("📎 %d attachments", total), Spoken: fmt.Sprintf("sent %d attachments", total)}
	}
	return an.withCaption(caption)
}

// paymentAnnouncement describes money sent in the thread. amount is already
// formatted, or empty while the indexer has not confirmed it — a figure the
// chain has not vouched for is not put in front of the recipient as fact.
func paymentAnnouncement(amount, note string) announcement {
	an := announcement{Line: "Sent you SFLUV", Spoken: "sent you SFLUV"}
	if amount != "" {
		an = announcement{
			Line:   fmt.Sprintf("Sent you %s SFLUV", amount),
			Spoken: fmt.Sprintf("sent you %s SFLUV", amount),
		}
	}
	return an.withCaption(note)
}
