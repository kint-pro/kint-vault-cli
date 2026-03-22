// Package vault implements per-value AES-256-GCM encryption with age key wrapping.
//
// File format (SOPS-inspired, keys in cleartext, values encrypted):
//
//	KEY1=ENC[AES256_GCM,data:<b64>,iv:<b64>,tag:<b64>]
//	KEY2=ENC[AES256_GCM,data:<b64>,iv:<b64>,tag:<b64>]
//	#KV_AGE_RECIPIENT_0 age1abc...
//	#KV_AGE_KEY <age-armored encrypted data key>
//	#KV_MAC <hex hmac of all plaintext values>
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// encValueRe matches ENC[AES256_GCM,data:...,iv:...,tag:...]
var encValueRe = regexp.MustCompile(`^ENC\[AES256_GCM,data:([^,]*),iv:([^,]+),tag:([^\]]+)\]$`)

// EncryptValue encrypts a single value with AES-256-GCM.
func EncryptValue(plaintext string, dataKey []byte) (string, error) {
	block, err := aes.NewCipher(dataKey)
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

	// GCM seal appends the tag to the ciphertext
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Split ciphertext and tag (last 16 bytes)
	tagSize := gcm.Overhead()
	ct := sealed[:len(sealed)-tagSize]
	tag := sealed[len(sealed)-tagSize:]

	return fmt.Sprintf("ENC[AES256_GCM,data:%s,iv:%s,tag:%s]",
		base64.StdEncoding.EncodeToString(ct),
		base64.StdEncoding.EncodeToString(nonce),
		base64.StdEncoding.EncodeToString(tag),
	), nil
}

// DecryptValue decrypts a single ENC[...] value.
func DecryptValue(encrypted string, dataKey []byte) (string, error) {
	m := encValueRe.FindStringSubmatch(encrypted)
	if m == nil {
		return "", fmt.Errorf("invalid encrypted value format")
	}

	ct, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		return "", fmt.Errorf("invalid data: %v", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(m[2])
	if err != nil {
		return "", fmt.Errorf("invalid iv: %v", err)
	}
	tag, err := base64.StdEncoding.DecodeString(m[3])
	if err != nil {
		return "", fmt.Errorf("invalid tag: %v", err)
	}

	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Reconstruct sealed data: ciphertext + tag
	sealed := append(ct, tag...)
	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed (wrong key or tampered data)")
	}

	return string(plaintext), nil
}

// GenerateDataKey creates a random 32-byte AES-256 key.
func GenerateDataKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// ComputeMAC computes HMAC-SHA256 over all plaintext values (sorted by key).
func ComputeMAC(secrets map[string]string, dataKey []byte) string {
	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	mac := hmac.New(sha256.New, dataKey)
	for _, k := range keys {
		mac.Write([]byte(k))
		mac.Write([]byte(secrets[k]))
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// IsEncryptedValue checks if a value is in ENC[...] format.
func IsEncryptedValue(value string) bool {
	return encValueRe.MatchString(value)
}

// EncryptAll encrypts a map of secrets, returning encrypted key=value lines + metadata lines.
func EncryptAll(secrets map[string]string, dataKey []byte) (string, error) {
	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	for _, k := range keys {
		enc, err := EncryptValue(secrets[k], dataKey)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt %s: %v", k, err)
		}
		lines = append(lines, fmt.Sprintf("%s=%s", k, enc))
	}

	return strings.Join(lines, "\n"), nil
}

// DecryptAll decrypts all ENC[...] values in a parsed map.
func DecryptAll(encrypted map[string]string, dataKey []byte) (map[string]string, error) {
	result := make(map[string]string)
	for k, v := range encrypted {
		if IsEncryptedValue(v) {
			plain, err := DecryptValue(v, dataKey)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt %s: %v", k, err)
			}
			result[k] = plain
		} else {
			result[k] = v
		}
	}
	return result, nil
}
