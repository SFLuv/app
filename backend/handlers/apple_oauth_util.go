package handlers

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const defaultAppleClientID = "org.sfluv.wallet"

var appleHTTPClient = &http.Client{
	Timeout: 12 * time.Second,
}

func oauthEncryptionKey() ([]byte, error) {
	if configured := strings.TrimSpace(os.Getenv("APP_DATA_ENCRYPTION_KEY")); configured != "" {
		if decoded, err := base64.StdEncoding.DecodeString(configured); err == nil && len(decoded) == 32 {
			return decoded, nil
		}
		if len(configured) == 32 {
			return []byte(configured), nil
		}
		sum := sha256.Sum256([]byte(configured))
		return sum[:], nil
	}

	appSecret := strings.TrimSpace(os.Getenv("PRIVY_APP_SECRET"))
	if appSecret == "" {
		return nil, fmt.Errorf("APP_DATA_ENCRYPTION_KEY or PRIVY_APP_SECRET is required for oauth credential storage")
	}
	sum := sha256.Sum256([]byte(appSecret))
	return sum[:], nil
}

func encryptSensitiveValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}

	key, err := oauthEncryptionKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	sealed := gcm.Seal(nonce, nonce, []byte(value), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptSensitiveValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}

	key, err := oauthEncryptionKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	if len(decoded) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted oauth credential is invalid")
	}

	nonce := decoded[:gcm.NonceSize()]
	ciphertext := decoded[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func appleClientID() string {
	configured := strings.TrimSpace(os.Getenv("APPLE_CLIENT_ID"))
	if configured != "" {
		return configured
	}
	return defaultAppleClientID
}

func appleTeamID() string {
	return strings.TrimSpace(os.Getenv("APPLE_TEAM_ID"))
}

func appleKeyID() string {
	return strings.TrimSpace(os.Getenv("APPLE_KEY_ID"))
}

func applePrivateKeyPEM() string {
	return strings.ReplaceAll(strings.TrimSpace(os.Getenv("APPLE_PRIVATE_KEY")), `\n`, "\n")
}

func buildAppleClientSecret() (string, error) {
	clientID := appleClientID()
	teamID := appleTeamID()
	keyID := appleKeyID()
	privateKeyPEM := applePrivateKeyPEM()
	if clientID == "" || teamID == "" || keyID == "" || privateKeyPEM == "" {
		return "", fmt.Errorf("APPLE_CLIENT_ID, APPLE_TEAM_ID, APPLE_KEY_ID, and APPLE_PRIVATE_KEY are required for apple token revocation")
	}

	privateKey, err := jwt.ParseECPrivateKeyFromPEM([]byte(privateKeyPEM))
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss": teamID,
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"aud": "https://appleid.apple.com",
		"sub": clientID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = keyID
	token.Header["typ"] = "JWT"

	return token.SignedString(privateKey)
}

func revokeAppleToken(ctx context.Context, clientSecret string, token string, tokenTypeHint string) error {
	form := url.Values{}
	form.Set("client_id", appleClientID())
	form.Set("client_secret", clientSecret)
	form.Set("token", strings.TrimSpace(token))
	if strings.TrimSpace(tokenTypeHint) != "" {
		form.Set("token_type_hint", strings.TrimSpace(tokenTypeHint))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://appleid.apple.com/auth/revoke", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	res, err := appleHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	if len(body) == 0 {
		return fmt.Errorf("apple token revoke failed with status %d", res.StatusCode)
	}

	var response map[string]any
	if err := json.Unmarshal(body, &response); err == nil {
		if rawError, ok := response["error"].(string); ok && rawError == "invalid_grant" {
			return nil
		}
	}

	return fmt.Errorf("apple token revoke failed with status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
}
