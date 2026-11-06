package handlers

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

// Telling the right people that something reached the support inbox.
//
// Everything is answered from support@. The chats only say that something came
// in and what it is about, so nobody has to keep the inbox open to find out:
// Technical Support gets anything about the app, Admin gets the rest.

// supportChat is one of the group chats notifications go to. Its value is the
// environment variable holding that chat's conversation id.
type supportChat string

const (
	supportChatTechnical supportChat = "SUPPORT_CHAT_TECHNICAL_ID"
	supportChatAdmin     supportChat = "SUPPORT_CHAT_ADMIN_ID"

	supportChatPreviewRunes = 140
)

// supportChatsFor is the routing table.
//
// Source comes first because anything sent from inside the app goes to
// Technical Support whatever topic was picked; they pass along what is not
// theirs.
func supportChatsFor(source, category string) []supportChat {
	if category == structs.SupportCategoryW9 {
		return []supportChat{supportChatAdmin, supportChatTechnical}
	}
	if source == structs.SupportSourceApp ||
		category == structs.SupportCategoryTechnical ||
		category == structs.SupportCategoryAccountDeletion {
		return []supportChat{supportChatTechnical}
	}
	return []supportChat{supportChatAdmin}
}

// supportChatLine is what the chat shows and what the phone says.
//
// Tax and account questions carry no preview: everybody in the chat can read
// it, it stays in the database, and it can show on a locked phone.
func supportChatLine(request *structs.SupportRequest, inboxFailed bool) string {
	topic := strings.TrimSpace(request.Subject)
	if topic == "" {
		topic = "No topic"
	}
	from := oneLineText(request.Name)
	if from == "" {
		from = request.Email
	}

	line := fmt.Sprintf("New support message: %s · %s · %s", topic, from, request.Reference)
	if request.Category != structs.SupportCategoryW9 && request.Category != structs.SupportCategoryAccountDeletion {
		line += "\n" + truncateRunes(oneLineText(request.Message), supportChatPreviewRunes)
	}
	if inboxFailed {
		line += "\nThis did not reach support@. Look it up by its reference."
	}
	return line
}

func oneLineText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// notifySupportRequest tells the chats about a request.
//
// The older staff alert only goes out when no chat was told. Once the chats are
// set up nobody gets told twice, and until then, or if they are ever both
// broken, a request still reaches somebody.
func (a *AppService) notifySupportRequest(ctx context.Context, request *structs.SupportRequest, inboxFailed bool) {
	chats := supportChatsFor(request.Source, request.Category)
	if a.notifySupportChats(ctx, chats, supportChatLine(request, inboxFailed)) == 0 {
		a.postSupportRequestAlert(ctx, request)
	}
}

// notifySupportChats posts a line to each chat and says how many it reached.
func (a *AppService) notifySupportChats(ctx context.Context, chats []supportChat, line string) int {
	reached := 0
	for _, chat := range chats {
		id, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(string(chat))), 10, 64)
		if err != nil || id <= 0 {
			a.logger.Logf("support notification not posted: %s is not set to a conversation id", chat)
			continue
		}
		if err := a.postSupportChatLine(ctx, id, line); err != nil {
			a.logger.Logf("support notification not posted to conversation %d: %s", id, err)
			continue
		}
		reached++
	}
	return reached
}

func (a *AppService) postSupportChatLine(ctx context.Context, conversationId int64, line string) error {
	conv, err := a.db.GetGroupConversation(ctx, conversationId)
	if err != nil {
		return err
	}
	msg, err := a.db.InsertSystemMessage(ctx, conv.Id, structs.SupportBotUserId, line)
	if err != nil {
		return err
	}
	// Named here because the bot has no public profile to look a name up from.
	msg.SenderName = "SFLuv"
	a.fanOutMessage(ctx, conv, msg, structs.SupportBotUserId, announcement{
		Line:   oneLineText(line),
		Spoken: "said: " + oneLineText(line),
	})
	return nil
}

// sendSupportInboxEmail puts a request in the support inbox.
//
// From our Mailgun domain, because sfluv.org has no SPF or DMARC for Mailgun to
// send as. Reply-To is the person, so answering it in Gmail reaches them.
func (a *AppService) sendSupportInboxEmail(request *structs.SupportRequest) error {
	sender := utils.NewEmailSender()
	if sender == nil {
		return fmt.Errorf("no mailer configured")
	}

	topic := strings.TrimSpace(request.Subject)
	if topic == "" {
		topic = "No topic"
	}
	account := ""
	if request.UserId != nil {
		account = strings.TrimSpace(*request.UserId)
	}

	row := func(label, value string) string {
		if value == "" {
			return ""
		}
		return fmt.Sprintf(`<tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:140px; vertical-align:top;">%s</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; white-space:pre-wrap;">%s</td>
  </tr>`, utils.EscapeEmailHTML(label), utils.EscapeEmailHTML(value))
	}
	content := `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">` +
		row("Reference", request.Reference) +
		row("Topic", topic) +
		row("From", oneLineText(request.Name)) +
		row("Email", request.Email) +
		row("Sent from", request.Source) +
		row("Account", account) +
		row("Message", request.Message) +
		`</table>`

	html := utils.BuildStyledEmailWithFooter(
		"New support message",
		"Reply to this email to answer them.",
		content,
		"Reference "+request.Reference,
	)
	subject := fmt.Sprintf("[%s] %s (%s)", topic, truncateRunes(oneLineText(request.Message), 60), request.Reference)
	return sender.SendEmailWithReplyTo(utils.SupportInboxEmail(), "SFLuv Support", subject, html,
		utils.NotificationFromEmail(), "SFLuv Support Form", request.Email)
}
