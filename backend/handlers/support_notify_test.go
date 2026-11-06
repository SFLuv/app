package handlers

import (
	"reflect"
	"strings"
	"testing"

	"github.com/SFLuv/app/backend/structs"
)

func TestSupportChatsFor(t *testing.T) {
	tech := []supportChat{supportChatTechnical}
	admin := []supportChat{supportChatAdmin}
	both := []supportChat{supportChatAdmin, supportChatTechnical}

	cases := []struct {
		name, source, category string
		want                   []supportChat
	}{
		{"website technical", structs.SupportSourceWebsite, structs.SupportCategoryTechnical, tech},
		{"website account deletion", structs.SupportSourceWebsite, structs.SupportCategoryAccountDeletion, tech},
		{"website general", structs.SupportSourceWebsite, structs.SupportCategoryGeneral, admin},
		{"website merchant", structs.SupportSourceWebsite, structs.SupportCategoryMerchant, admin},
		{"website no category", structs.SupportSourceWebsite, "", admin},
		{"app form, any topic", structs.SupportSourceApp, structs.SupportCategoryEvents, tech},
		{"website w9", structs.SupportSourceWebsite, structs.SupportCategoryW9, both},
		{"app w9", structs.SupportSourceApp, structs.SupportCategoryW9, both},
	}
	for _, c := range cases {
		if got := supportChatsFor(c.source, c.category); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSupportChatLinePreview(t *testing.T) {
	request := &structs.SupportRequest{
		Reference: "SUP-ABC123",
		Email:     "someone@example.com",
		Name:      "Sam",
		Subject:   "General question",
		Category:  structs.SupportCategoryGeneral,
		Message:   "line one\n\nline two",
	}
	line := supportChatLine(request, false)
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
		if line := supportChatLine(request, false); strings.Contains(line, "line one") {
			t.Errorf("%s line %q carries a preview", category, line)
		}
	}

	if line := supportChatLine(request, true); !strings.Contains(line, "did not reach support@") {
		t.Errorf("line %q does not report the failed inbox email", line)
	}
}
