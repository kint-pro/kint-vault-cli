package vault

import (
	"crypto/rand"
	"strings"
	"testing"
	"testing/quick"
)

func randomKey() []byte {
	k := make([]byte, 32)
	rand.Read(k)
	return k
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext string
		aad       string
	}{
		{"simple", "hello", "KEY:"},
		{"empty", "", "KEY:"},
		{"space", "hello world", "KEY:"},
		{"unicode", "héllo wörld 日本語 🔑", "KEY:"},
		{"special", `p@$$w0rd!#%^&*()`, "KEY:"},
		{"equals", "postgres://h:5432/db?opt=1", "URL:"},
		{"newline_literal", `line1\nline2`, "KEY:"},
		{"long_value", strings.Repeat("x", 10000), "BIG:"},
		{"empty_aad", "value", ""},
		{"enc_like", "ENC[AES256_GCM,data:fake,iv:fake,tag:fake,type:str]", "TRICKY:"},
		{"base64_chars", "abc+/def==", "B64:"},
		{"null_byte", "before\x00after", "KEY:"},
		{"only_spaces", "   ", "KEY:"},
		{"tab", "col1\tcol2", "KEY:"},
		{"backslash", `C:\Users\path`, "KEY:"},
		{"quotes", `"quoted" and 'single'`, "KEY:"},
		{"json", `{"key":"value","n":42}`, "KEY:"},
		{"comma_in_value", "a,b,c", "KEY:"},
		{"bracket_in_value", "ENC[notreal]", "KEY:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := randomKey()
			c := NewCipher()

			enc, err := c.encryptValue(tt.plaintext, key, tt.aad)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}

			if !isEncryptedValue(enc) {
				t.Fatalf("output not ENC format: %s", enc)
			}

			if !strings.Contains(enc, "type:str") {
				t.Fatalf("missing type tag: %s", enc)
			}

			dec, err := c.decryptValue(enc, key, tt.aad)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}

			if dec != tt.plaintext {
				t.Fatalf("roundtrip mismatch: got %q, want %q", dec, tt.plaintext)
			}
		})
	}
}

func TestEncryptDecryptQuick(t *testing.T) {
	f := func(plaintext, aad string) bool {
		key := randomKey()
		c := NewCipher()
		enc, err := c.encryptValue(plaintext, key, aad)
		if err != nil {
			return false
		}
		dec, err := c.decryptValue(enc, key, aad)
		if err != nil {
			return false
		}
		return dec == plaintext
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 500}); err != nil {
		t.Error(err)
	}
}

func TestAADMismatchFails(t *testing.T) {
	key := randomKey()
	c := NewCipher()
	enc, _ := c.encryptValue("secret", key, "KEY_A:")
	_, err := c.decryptValue(enc, key, "KEY_B:")
	if err == nil {
		t.Fatal("expected AAD mismatch to fail")
	}
}

func TestWrongKeyFails(t *testing.T) {
	c := NewCipher()
	enc, _ := c.encryptValue("secret", randomKey(), "KEY:")
	_, err := c.decryptValue(enc, randomKey(), "KEY:")
	if err == nil {
		t.Fatal("expected wrong key to fail")
	}
}

func TestDecryptInvalidFormats(t *testing.T) {
	key := randomKey()
	c := NewCipher()

	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"plaintext", "just a string"},
		{"partial", "ENC[AES256_GCM,data:abc"},
		{"missing_type", "ENC[AES256_GCM,data:abc,iv:def,tag:ghi]"},
		{"wrong_prefix", "XXX[AES256_GCM,data:abc,iv:def,tag:ghi,type:str]"},
		{"bad_base64_data", "ENC[AES256_GCM,data:!!!,iv:AAAA,tag:BBBB,type:str]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.decryptValue(tt.input, key, "KEY:")
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestIsEncryptedValue(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"ENC[AES256_GCM,data:abc,iv:def,tag:ghi,type:str]", true},
		{"ENC[AES256_GCM,data:,iv:def,tag:ghi,type:str]", true},
		{"plaintext", false},
		{"", false},
		{"ENC[AES256_GCM,data:abc]", false},
		{"ENC[AES256_GCM,data:abc,iv:def,tag:ghi]", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isEncryptedValue(tt.input); got != tt.expected {
				t.Fatalf("isEncryptedValue(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestStashReusesNonce(t *testing.T) {
	key := randomKey()
	c := NewCipher()

	enc1, _ := c.encryptValue("same", key, "KEY:")
	enc2, _ := c.encryptValue("same", key, "KEY:")

	if enc1 != enc2 {
		t.Fatal("stash should produce identical ciphertext for same plaintext+aad")
	}
}

func TestStashDifferentAADProducesDifferentCiphertext(t *testing.T) {
	key := randomKey()
	c := NewCipher()

	enc1, _ := c.encryptValue("same", key, "KEY_A:")
	enc2, _ := c.encryptValue("same", key, "KEY_B:")

	if enc1 == enc2 {
		t.Fatal("different AAD should produce different ciphertext")
	}
}

func TestStashPopulatedByDecrypt(t *testing.T) {
	key := randomKey()
	c1 := NewCipher()

	enc, _ := c1.encryptValue("secret", key, "KEY:")

	c2 := NewCipher()
	_, _ = c2.decryptValue(enc, key, "KEY:")

	enc2, _ := c2.encryptValue("secret", key, "KEY:")

	if enc != enc2 {
		t.Fatal("decrypt should populate stash so re-encrypt produces same ciphertext")
	}
}

func TestEncryptAllDecryptAll(t *testing.T) {
	key := randomKey()

	secrets := map[string]string{
		"A": "1",
		"B": "hello world",
		"C": "",
		"D": "p@$$w0rd!#%^&*()",
	}

	c := NewCipher()
	encLines, err := c.encryptAll(secrets, key)
	if err != nil {
		t.Fatalf("encryptAll: %v", err)
	}

	lines := strings.Split(encLines, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(lines))
	}

	for _, line := range lines {
		if !strings.Contains(line, "=ENC[") {
			t.Fatalf("line not encrypted: %s", line)
		}
	}

	encrypted := make(map[string]string)
	for _, line := range lines {
		idx := strings.Index(line, "=")
		encrypted[line[:idx]] = line[idx+1:]
	}

	c2 := NewCipher()
	decrypted, err := c2.decryptAll(encrypted, key)
	if err != nil {
		t.Fatalf("decryptAll: %v", err)
	}

	for k, v := range secrets {
		if decrypted[k] != v {
			t.Fatalf("key %s: got %q, want %q", k, decrypted[k], v)
		}
	}
}

func TestDecryptAllRejectsPlaintext(t *testing.T) {
	key := randomKey()
	c := NewCipher()

	encrypted := map[string]string{
		"PLAIN": "not_encrypted",
	}

	_, err := c.decryptAll(encrypted, key)
	if err == nil {
		t.Fatal("expected error for plaintext value")
	}
	if !strings.Contains(err.Error(), "PLAIN") {
		t.Fatalf("error should mention key name: %v", err)
	}
}

func TestEncryptAllSorted(t *testing.T) {
	key := randomKey()
	c := NewCipher()

	secrets := map[string]string{"Z": "1", "A": "2", "M": "3"}
	encLines, _ := c.encryptAll(secrets, key)
	lines := strings.Split(encLines, "\n")

	if !strings.HasPrefix(lines[0], "A=") {
		t.Fatalf("expected sorted, first line: %s", lines[0])
	}
	if !strings.HasPrefix(lines[1], "M=") {
		t.Fatalf("expected sorted, second line: %s", lines[1])
	}
	if !strings.HasPrefix(lines[2], "Z=") {
		t.Fatalf("expected sorted, third line: %s", lines[2])
	}
}

func TestComputeMAC(t *testing.T) {
	key := randomKey()

	secrets := map[string]string{"A": "1", "B": "2"}
	mac1 := computeMAC(secrets, key)
	mac2 := computeMAC(secrets, key)

	if mac1 != mac2 {
		t.Fatal("same input should produce same MAC")
	}

	if len(mac1) != 64 {
		t.Fatalf("MAC should be 64 hex chars, got %d", len(mac1))
	}
}

func TestComputeMACDifferentKey(t *testing.T) {
	secrets := map[string]string{"A": "1"}
	mac1 := computeMAC(secrets, randomKey())
	mac2 := computeMAC(secrets, randomKey())

	if mac1 == mac2 {
		t.Fatal("different keys should produce different MACs")
	}
}

func TestComputeMACDifferentValues(t *testing.T) {
	key := randomKey()
	mac1 := computeMAC(map[string]string{"A": "1"}, key)
	mac2 := computeMAC(map[string]string{"A": "2"}, key)

	if mac1 == mac2 {
		t.Fatal("different values should produce different MACs")
	}
}

func TestComputeMACDifferentKeys(t *testing.T) {
	key := randomKey()
	mac1 := computeMAC(map[string]string{"A": "1"}, key)
	mac2 := computeMAC(map[string]string{"B": "1"}, key)

	if mac1 == mac2 {
		t.Fatal("different key names should produce different MACs")
	}
}

func TestComputeMACOrderIndependent(t *testing.T) {
	key := randomKey()
	mac1 := computeMAC(map[string]string{"A": "1", "B": "2"}, key)
	mac2 := computeMAC(map[string]string{"B": "2", "A": "1"}, key)

	if mac1 != mac2 {
		t.Fatal("MAC should be order-independent (sorted internally)")
	}
}

func TestComputeMACBoundaryConfusion(t *testing.T) {
	key := randomKey()
	mac1 := computeMAC(map[string]string{"AB": "C"}, key)
	mac2 := computeMAC(map[string]string{"A": "BC"}, key)

	if mac1 == mac2 {
		t.Fatal("length-prefix should prevent key/value boundary confusion")
	}
}

func TestComputeMACEmpty(t *testing.T) {
	key := randomKey()
	mac := computeMAC(map[string]string{}, key)
	if len(mac) != 64 {
		t.Fatalf("empty MAC should still be 64 hex chars, got %d", len(mac))
	}
}

func TestEncryptMACRoundtrip(t *testing.T) {
	key := randomKey()
	c := NewCipher()

	mac := computeMAC(map[string]string{"A": "1"}, key)
	encMAC, err := c.encryptMAC(mac, key)
	if err != nil {
		t.Fatalf("encryptMAC: %v", err)
	}

	if !isEncryptedValue(encMAC) {
		t.Fatal("encrypted MAC should be ENC format")
	}

	decMAC, err := c.decryptMAC(encMAC, key)
	if err != nil {
		t.Fatalf("decryptMAC: %v", err)
	}

	if decMAC != mac {
		t.Fatalf("MAC roundtrip: got %q, want %q", decMAC, mac)
	}
}

func TestGenerateDataKey(t *testing.T) {
	k1, err := generateDataKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(k1) != 32 {
		t.Fatalf("key should be 32 bytes, got %d", len(k1))
	}

	k2, _ := generateDataKey()
	if string(k1) == string(k2) {
		t.Fatal("two generated keys should differ")
	}
}
