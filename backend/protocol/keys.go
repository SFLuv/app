package protocol

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// EnvMasterKey holds the key that encrypts every signer key in the database.
//
// It must be a value that has never been deployed before: the point of moving
// signer keys out of the environment is that an environment leak stops being
// enough on its own, and that only holds while the master key and the
// ciphertext are not exposed together.
const EnvMasterKey = "PROTOCOL_KEY_ENCRYPTION_KEY"

// GenerateSigner mints a new signing key.
//
// crypto.GenerateKey draws from crypto/rand — the operating system's CSPRNG —
// which is the only acceptable source here. math/rand would produce keys that
// are reproducible from the seed, and a key that an attacker can regenerate is
// not a key.
func GenerateSigner() (*ecdsa.PrivateKey, common.Address, error) {
	key, err := crypto.GenerateKey()
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("error generating a signing key: %w", err)
	}
	return key, pubkeyToAddress(key), nil
}

func pubkeyToAddress(key *ecdsa.PrivateKey) common.Address {
	return crypto.PubkeyToAddress(key.PublicKey)
}

// MasterKey reads and validates the encryption key from the environment.
func MasterKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv(EnvMasterKey))
	if raw == "" {
		return nil, fmt.Errorf("%s is not set, so protocol signer keys cannot be read or written", EnvMasterKey)
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(raw, "0x"))
	if err != nil {
		return nil, fmt.Errorf("%s is not hex: %w", EnvMasterKey, err)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("%s decodes to %d bytes, want 32 (a 64-character hex string)", EnvMasterKey, len(decoded))
	}
	return decoded, nil
}

// MasterKeyConfigured reports whether a usable master key is present, for the
// boot log and the admin panel to say so without handling the key itself.
func MasterKeyConfigured() bool {
	_, err := MasterKey()
	return err == nil
}

// SealSigner encrypts a signing key for storage.
//
// AES-256-GCM, with the account's own key as additional authenticated data. The
// AAD is what stops a ciphertext being moved between rows: a blob sealed for
// the faucet will not open as the redeemer admin's, so a write that swapped two
// rows produces a decryption failure rather than a wallet signing with the
// wrong key.
//
// The returned blob is nonce ‖ ciphertext, hex encoded. The nonce is random per
// seal and never reused, which GCM requires absolutely — a repeated nonce under
// one key leaks the keystream.
func SealSigner(master []byte, accountKey string, signer *ecdsa.PrivateKey) (string, error) {
	if signer == nil {
		return "", fmt.Errorf("no signing key to seal")
	}
	aead, err := newAEAD(master)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("error drawing a nonce: %w", err)
	}
	plaintext := crypto.FromECDSA(signer)
	sealed := aead.Seal(nil, nonce, plaintext, []byte(accountKey))
	// The plaintext is a copy made by FromECDSA; wiping it keeps the raw key
	// from lingering in a buffer the garbage collector has not reached yet.
	for i := range plaintext {
		plaintext[i] = 0
	}
	return hex.EncodeToString(append(nonce, sealed...)), nil
}

// OpenSigner decrypts a stored signing key.
func OpenSigner(master []byte, accountKey, blob string) (*ecdsa.PrivateKey, error) {
	aead, err := newAEAD(master)
	if err != nil {
		return nil, err
	}
	raw, err := hex.DecodeString(strings.TrimSpace(blob))
	if err != nil {
		return nil, fmt.Errorf("stored signer key is not hex: %w", err)
	}
	if len(raw) <= aead.NonceSize() {
		return nil, fmt.Errorf("stored signer key is too short to contain a nonce and a key")
	}
	nonce, ciphertext := raw[:aead.NonceSize()], raw[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(accountKey))
	if err != nil {
		// Deliberately vague about which of the three it was. The caller can
		// do nothing differently, and naming the failure mode tells an attacker
		// holding one of the two secrets which one they are missing.
		return nil, fmt.Errorf("stored signer key for %s could not be decrypted: wrong master key, wrong account, or tampered row", accountKey)
	}
	key, err := crypto.ToECDSA(plaintext)
	for i := range plaintext {
		plaintext[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("decrypted signer key for %s is not a valid key: %w", accountKey, err)
	}
	return key, nil
}

func newAEAD(master []byte) (cipher.AEAD, error) {
	if len(master) != 32 {
		return nil, fmt.Errorf("the master key is %d bytes, want 32", len(master))
	}
	block, err := aes.NewCipher(master)
	if err != nil {
		return nil, fmt.Errorf("error preparing the cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("error preparing gcm: %w", err)
	}
	return aead, nil
}
