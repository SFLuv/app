package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotificationTestModeWritesEmailWithoutMailgun(t *testing.T) {
	outputDir := t.TempDir()
	t.Setenv("NOTIFICATION_TEST_MODE", "true")
	t.Setenv("NOTIFICATION_TEST_OUTPUT_DIR", outputDir)
	t.Setenv("MAILGUN_DOMAIN", "")
	t.Setenv("MAILGUN_API_KEY", "")

	sender := NewEmailSender()
	if sender == nil {
		t.Fatal("NewEmailSender returned nil in notification test mode")
	}

	err := sender.SendEmail("user@example.com", "Test User", "Hello Test", "<html><body>Hello</body></html>", "from@example.com", "SFLuv")
	if err != nil {
		t.Fatalf("SendEmail returned error: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(outputDir, "*email-hello-test.html"))
	if err != nil {
		t.Fatalf("Glob returned error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one test email file, got %d: %#v", len(matches), matches)
	}

	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(content), "Subject: Hello Test") || !strings.Contains(string(content), "<body>Hello</body>") {
		t.Fatalf("test email content missing metadata or body: %s", string(content))
	}
}

func TestWriteTestPushNotification(t *testing.T) {
	outputDir := t.TempDir()
	t.Setenv("NOTIFICATION_TEST_OUTPUT_DIR", outputDir)

	path, err := WriteTestPushNotification("ExponentPushToken[test]", "Payment received", "1.00 SFLUV", map[string]string{"hash": "0xabc"})
	if err != nil {
		t.Fatalf("WriteTestPushNotification returned error: %v", err)
	}
	if filepath.Dir(path) != outputDir {
		t.Fatalf("push notification path = %q, want dir %q", path, outputDir)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(content), "Title: Payment received") || !strings.Contains(string(content), `"hash": "0xabc"`) {
		t.Fatalf("test push content missing fields: %s", string(content))
	}
}
