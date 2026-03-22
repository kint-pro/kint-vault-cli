// Package vault implements per-value AES-256-GCM encryption with age key wrapping.
//
// File format:
//
//	KEY1=ENC[AES256_GCM,data:<b64>,iv:<b64>,tag:<b64>,type:str]
//	KEY2=ENC[AES256_GCM,data:<b64>,iv:<b64>,tag:<b64>,type:str]
//	kv_age_recipient_0=age1abc...
//	kv_age_key=<age-armored encrypted data key, newlines escaped>
//	kv_mac=ENC[AES256_GCM,data:<b64>,iv:<b64>,tag:<b64>,type:str]
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const nonceSize = 32

var encValueRe = regexp.MustCompile(`^ENC\[AES256_GCM,data:([^,]*),iv:([^,]+),tag:([^,]+),type:([^\]]+)\]$`)

type stashKey struct {
	additionalData string
	plaintext      string
}

type Cipher struct {
	stash map[stashKey][]byte
}

func NewCipher() *Cipher {
	return &Cipher{stash: make(map[stashKey][]byte)}
}

func newGCM(dataKey []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithNonceSize(block, nonceSize)
}

func (c *Cipher) encryptValue(plaintext string, dataKey []byte, aad string) (string, error) {
	gcm, err := newGCM(dataKey)
	if err != nil {
		return "", err
	}

	sk := stashKey{additionalData: aad, plaintext: plaintext}
	nonce, cached := c.stash[sk]
	if !cached {
		nonce = make([]byte, nonceSize)
		if _, err := rand.Read(nonce); err != nil {
			return "", err
		}
		c.stash[sk] = nonce
	}

	sealed := gcm.Seal(nil, nonce, []byte(plaintext), []byte(aad))

	tagSize := gcm.Overhead()
	ct := sealed[:len(sealed)-tagSize]
	tag := sealed[len(sealed)-tagSize:]

	return fmt.Sprintf("ENC[AES256_GCM,data:%s,iv:%s,tag:%s,type:str]",
		base64.StdEncoding.EncodeToString(ct),
		base64.StdEncoding.EncodeToString(nonce),
		base64.StdEncoding.EncodeToString(tag),
	), nil
}

func (c *Cipher) decryptValue(encrypted string, dataKey []byte, aad string) (string, error) {
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

	gcm, err := newGCM(dataKey)
	if err != nil {
		return "", err
	}

	sealed := append(ct, tag...)
	plainBytes, err := gcm.Open(nil, nonce, sealed, []byte(aad))
	if err != nil {
		return "", fmt.Errorf("decryption failed (wrong key or tampered data)")
	}

	plaintext := string(plainBytes)
	c.stash[stashKey{additionalData: aad, plaintext: plaintext}] = nonce
	return plaintext, nil
}

func generateDataKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

func computeMAC(secrets map[string]string, dataKey []byte) string {
	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	mac := hmac.New(sha256.New, dataKey)
	lenBuf := make([]byte, 4)
	for _, k := range keys {
		kBytes := []byte(k)
		vBytes := []byte(secrets[k])
		binary.BigEndian.PutUint32(lenBuf, uint32(len(kBytes)))
		mac.Write(lenBuf)
		mac.Write(kBytes)
		binary.BigEndian.PutUint32(lenBuf, uint32(len(vBytes)))
		mac.Write(lenBuf)
		mac.Write(vBytes)
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func isEncryptedValue(value string) bool {
	return encValueRe.MatchString(value)
}

func (c *Cipher) encryptAll(secrets map[string]string, dataKey []byte) (string, error) {
	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	for _, k := range keys {
		aad := k + ":"
		enc, err := c.encryptValue(secrets[k], dataKey, aad)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt %s: %v", k, err)
		}
		lines = append(lines, fmt.Sprintf("%s=%s", k, enc))
	}

	return strings.Join(lines, "\n"), nil
}

func (c *Cipher) decryptAll(encrypted map[string]string, dataKey []byte) (map[string]string, error) {
	result := make(map[string]string)
	for k, v := range encrypted {
		if !isEncryptedValue(v) {
			return nil, fmt.Errorf("value for %s is not encrypted", k)
		}
		aad := k + ":"
		plain, err := c.decryptValue(v, dataKey, aad)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt %s: %v", k, err)
		}
		result[k] = plain
	}
	return result, nil
}

func (c *Cipher) encryptMAC(mac string, dataKey []byte) (string, error) {
	return c.encryptValue(mac, dataKey, "kv_mac:")
}

func (c *Cipher) decryptMAC(encrypted string, dataKey []byte) (string, error) {
	return c.decryptValue(encrypted, dataKey, "kv_mac:")
}
