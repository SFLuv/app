package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/SFLuv/app/backend/structs"
)

func TestSupportChatLinePreview(t *testing.T) {
	request := &structs.SupportRequest{
		Reference: "SUP-ABC123",
		Email:     "someone@example.com",
		Name:      "Sam",
		Subject:   "General question",
		Category:  structs.SupportCategoryGeneral,
		Message:   "line one\n\nline two",
	}
	line := supportChatLine(request)
	for _, want := range []string{"General question", "Sam", "SUP-ABC123", "line one line two"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q is missing %q", line, want)
		}
	}
	if strings.Contains(line, "did not reach") {
		t.Errorf("line %q reports a failure that did not happen", line)
	}

	for _, category := range []string{structs.SupportCategoryW9, structs.SupportCategoryAccountDeletion} {
		request.Category = category
		if line := supportChatLine(request); strings.Contains(line, "line one") {
			t.Errorf("%s line %q carries a preview", category, line)
		}
	}

}

func TestMessageNoticeNoReply(t *testing.T) {
	a := &AppService{}
	conv := &structs.ConversationRow{}
	for sender, want := range map[string]bool{
		structs.SupportBotUserId: true,
		"someone":                false,
	} {
		msg := &structs.Message{Kind: structs.MessageKindText, SenderName: "Name"}
		notice := a.messageNotice(context.Background(), conv, msg, sender, announcement{Line: "hi", Spoken: "said: hi"})
		if notice.NoReply != want {
			t.Errorf("sender %q: NoReply = %v, want %v", sender, notice.NoReply, want)
		}
	}
}
