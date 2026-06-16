package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

const (
	notificationTestModeEnvKey      = "NOTIFICATION_TEST_MODE"
	notificationTestingModeEnvKey   = "NOTIFICATION_TESTING_MODE"
	notificationTestOutputDirEnvKey = "NOTIFICATION_TEST_OUTPUT_DIR"
	defaultNotificationTestDir      = "test-notifications"
)

var testNotificationSequence uint64

func NotificationTestModeEnabled() bool {
	return truthyEnv(notificationTestModeEnvKey) ||
		truthyEnv(notificationTestingModeEnvKey) ||
		truthyEnv("SFLUV_NOTIFICATION_TEST_MODE")
}

func WriteTestEmailNotification(
	toEmail string,
	toName string,
	subject string,
	htmlContent string,
	fromEmail string,
	fromName string,
	attachments []EmailAttachment,
) (string, error) {
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	metadata := []string{
		"SFLuv notification test email",
		"Created: " + createdAt,
		"To: " + formatEmailIdentity(toName, toEmail),
		"From: " + formatEmailIdentity(fromName, fromEmail),
		"Subject: " + subject,
	}
	if len(attachments) > 0 {
		metadata = append(metadata, "Attachments:")
		for _, attachment := range attachments {
			filename := strings.TrimSpace(attachment.Filename)
			if filename == "" {
				filename = "(unnamed)"
			}
			metadata = append(metadata, fmt.Sprintf("- %s (%d bytes)", filename, len(attachment.Data)))
		}
	}

	content := []byte(testHTMLComment(metadata) + "\n" + htmlContent)
	return writeTestNotificationFile("email", subject, ".html", content)
}

func WriteTestPushNotification(token string, title string, body string, data map[string]string) (string, error) {
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	dataJSON, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	content := fmt.Sprintf(`SFLuv notification test push
Created: %s
To token: %s
Title: %s
Body: %s
Data:
%s
`, createdAt, token, title, body, string(dataJSON))

	return writeTestNotificationFile("push", title, ".txt", []byte(content))
}

func notificationTestOutputDir() string {
	if dir := strings.TrimSpace(os.Getenv(notificationTestOutputDirEnvKey)); dir != "" {
		return dir
	}
	if root, err := GetProjectRoot(); err == nil {
		return filepath.Join(root, defaultNotificationTestDir)
	}
	return defaultNotificationTestDir
}

func writeTestNotificationFile(kind string, title string, extension string, content []byte) (string, error) {
	dir := notificationTestOutputDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	sequence := atomic.AddUint64(&testNotificationSequence, 1)
	timestamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	base := sanitizeNotificationTestFilename(fmt.Sprintf("%s-%06d-%s-%s", timestamp, sequence, kind, title))
	if base == "" {
		base = fmt.Sprintf("%s-%06d-%s", timestamp, sequence, kind)
	}
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}

	path := filepath.Join(dir, base+extension)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func formatEmailIdentity(name string, email string) string {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" {
		return email
	}
	if email == "" {
		return name
	}
	return fmt.Sprintf("%s <%s>", name, email)
}

func testHTMLComment(lines []string) string {
	escaped := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.ReplaceAll(line, "--", "- -")
		line = strings.ReplaceAll(line, "\r", " ")
		escaped = append(escaped, line)
	}
	return "<!--\n" + strings.Join(escaped, "\n") + "\n-->"
}

func sanitizeNotificationTestFilename(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastSeparator := false

	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastSeparator = false
			continue
		}
		if r == '.' || r == '_' {
			builder.WriteRune(r)
			lastSeparator = false
			continue
		}
		if !lastSeparator {
			builder.WriteByte('-')
			lastSeparator = true
		}
	}

	filename := strings.Trim(builder.String(), "-_.")
	if len(filename) > 120 {
		filename = strings.Trim(filename[:120], "-_.")
	}
	return filename
}

func truthyEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}
