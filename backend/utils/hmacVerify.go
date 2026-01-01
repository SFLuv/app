package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func HmacVerify(message []byte, sig string, key string) bool {
	h := hmac.New(sha256.New, []byte(key))
	h.Write(message)

	return hex.EncodeToString(h.Sum(nil)) == sig
}
