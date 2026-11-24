package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/structs"
)

// What a support request looks like in a team chat, and the support bot that
// posts it.
//
// Who hears about a request is the notification hub's routing: each category
// is its own topic, and a team that wants requests in a group chat names the
// chat in its settings. That replaces SUPPORT_CHAT_TECHNICAL_ID and
// SUPPORT_CHAT_ADMIN_ID, which hard-wired two chats and a routing table here,
// and the copy every request used to send to support@, which is now a team's
// email address if anybody wants it.

const supportChatPreviewRunes = 500

// supportChatLine is what the chat shows and what the phone says.
//
// Tax and account questions carry no preview: everybody in the chat can read
// it, it stays in the database, and it can show on a locked phone.
func supportChatLine(request *structs.SupportRequest) string {
	topic := strings.TrimSpace(request.Subject)
	if topic == "" {
		topic = "No topic"
	}
	from := oneLineText(request.Name)
	if from == "" {
		from = request.Email
	}

	line := fmt.Sprintf("%s\nFrom: %s\nReference: %s", topic, from, request.Reference)
	if request.Category != structs.SupportCategoryW9 && request.Category != structs.SupportCategoryAccountDeletion {
		line += "\n\n" + truncateRunes(strings.TrimSpace(request.Message), supportChatPreviewRunes)
	}
	return line
}

func oneLineText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// PostSupportBotLine posts one line into a group chat as the support bot. The
// hub delivers team chats through it.
func (a *AppService) PostSupportBotLine(ctx context.Context, conversationId int64, line string) error {
	conv, err := a.db.GetGroupConversation(ctx, conversationId)
	if err != nil {
		return err
	}
	msg, err := a.db.InsertMessage(ctx, &db.NewMessage{
		ConversationId: conv.Id,
		SenderId:       structs.SupportBotUserId,
		Kind:           structs.MessageKindText,
		Body:           line,
	})
	if err != nil {
		return err
	}
	msg.SenderName = structs.SupportBotName
	a.fanOutMessage(ctx, conv, msg, structs.SupportBotUserId, announcement{
		Line:   oneLineText(line),
		Spoken: "said: " + oneLineText(line),
	})
	return nil
}
