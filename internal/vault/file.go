package vault

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"filippo.io/age"
	"filippo.io/age/armor"
	"github.com/kint-pro/kint-vault-cli/internal/config"
)

const (
	metaPrefix     = "#KV "
	recipientKey   = "RECIPIENT_"
	dataKeyKey     = "AGE_KEY"
	macKey         = "MAC"
)

// EncryptedFile represents a parsed encrypted dotenv file.
type EncryptedFile struct {
	// Encrypted key=ENC[...] pairs (keys in cleartext)
	Values map[string]string
	// Age-encrypted data key (armored)
	WrappedDataKey string
	// Recipients used for encryption
	Recipients []string
	// HMAC of plaintext values
	MAC string
}

// WriteEncryptedFile encrypts secrets and writes the file.
func WriteEncryptedFile(path string, secrets map[string]string) error {
	recipients, err := config.GetRecipients()
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return fmt.Errorf("no recipients configured. Run: kint-vault init")
	}

	// Generate data key
	dataKey, err := GenerateDataKey()
	if err != nil {
		return err
	}

	// Encrypt all values
	encLines, err := EncryptAll(secrets, dataKey)
	if err != nil {
		return err
	}

	// Wrap data key with age for all recipients
	wrappedKey, err := wrapDataKey(dataKey, recipients)
	if err != nil {
		return err
	}

	// Compute MAC
	mac := ComputeMAC(secrets, dataKey)

	// Build file content
	var lines []string
	lines = append(lines, encLines)

	// Metadata as comments (won't interfere with dotenv parsers)
	for i, r := range recipients {
		lines = append(lines, fmt.Sprintf("%s%s%d %s", metaPrefix, recipientKey, i, r))
	}
	// Store wrapped key as single-line escaped
	lines = append(lines, fmt.Sprintf("%s%s %s", metaPrefix, dataKeyKey, escapeLine(wrappedKey)))
	lines = append(lines, fmt.Sprintf("%s%s %s", metaPrefix, macKey, mac))

	content := strings.Join(lines, "\n") + "\n"
	return config.AtomicWrite(path, []byte(content))
}

// ReadEncryptedFile reads and decrypts an encrypted dotenv file.
func ReadEncryptedFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	ef, err := parseEncryptedFile(string(data))
	if err != nil {
		return nil, err
	}

	// Unwrap data key
	dataKey, err := unwrapDataKey(ef.WrappedDataKey)
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt data key. Your key may not be in recipients")
	}

	// Decrypt all values
	secrets, err := DecryptAll(ef.Values, dataKey)
	if err != nil {
		return nil, err
	}

	// Verify MAC
	expectedMAC := ComputeMAC(secrets, dataKey)
	if ef.MAC != "" && ef.MAC != expectedMAC {
		return nil, fmt.Errorf("MAC verification failed — file may be tampered")
	}

	return secrets, nil
}

// ReEncryptFile decrypts and re-encrypts a file (for rotation / recipient changes).
func ReEncryptFile(path string) error {
	secrets, err := ReadEncryptedFile(path)
	if err != nil {
		return err
	}
	return WriteEncryptedFile(path, secrets)
}

// parseEncryptedFile parses the file format into its components.
func parseEncryptedFile(content string) (*EncryptedFile, error) {
	ef := &EncryptedFile{
		Values: make(map[string]string),
	}

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Metadata comment
		if strings.HasPrefix(line, metaPrefix) {
			meta := line[len(metaPrefix):]
			if strings.HasPrefix(meta, recipientKey) {
				// #KV RECIPIENT_0 age1...
				parts := strings.SplitN(meta, " ", 2)
				if len(parts) == 2 {
					ef.Recipients = append(ef.Recipients, strings.TrimSpace(parts[1]))
				}
			} else if strings.HasPrefix(meta, dataKeyKey+" ") {
				ef.WrappedDataKey = unescapeLine(strings.TrimPrefix(meta, dataKeyKey+" "))
			} else if strings.HasPrefix(meta, macKey+" ") {
				ef.MAC = strings.TrimPrefix(meta, macKey+" ")
			}
			continue
		}

		// Skip other comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Key=Value
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := line[:idx]
		value := line[idx+1:]
		ef.Values[key] = value
	}

	if ef.WrappedDataKey == "" {
		return nil, fmt.Errorf("no encrypted data key found in file")
	}

	return ef, nil
}

// wrapDataKey encrypts the data key with age for all recipients.
func wrapDataKey(dataKey []byte, recipientKeys []string) (string, error) {
	var recipients []age.Recipient
	for _, pk := range recipientKeys {
		r, err := age.ParseX25519Recipient(pk)
		if err != nil {
			return "", fmt.Errorf("invalid recipient %s: %v", pk, err)
		}
		recipients = append(recipients, r)
	}

	var buf bytes.Buffer
	armorWriter := armor.NewWriter(&buf)
	w, err := age.Encrypt(armorWriter, recipients...)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(dataKey); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	if err := armorWriter.Close(); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// unwrapDataKey decrypts the data key using available age identities.
func unwrapDataKey(wrappedKey string) ([]byte, error) {
	identities, err := loadIdentities()
	if err != nil {
		return nil, err
	}

	reader := armor.NewReader(strings.NewReader(wrappedKey))
	decrypted, err := age.Decrypt(reader, identities...)
	if err != nil {
		return nil, err
	}

	return io.ReadAll(decrypted)
}

// loadIdentities loads age identities from SOPS_AGE_KEY env var or key file.
func loadIdentities() ([]age.Identity, error) {
	if keyData := os.Getenv("SOPS_AGE_KEY"); keyData != "" {
		return age.ParseIdentities(strings.NewReader(keyData))
	}
	keyFile := config.AgeKeyFile()
	f, err := os.Open(keyFile)
	if err != nil {
		return nil, fmt.Errorf("cannot open age key: %v. Run: kint-vault init", err)
	}
	defer f.Close()
	return age.ParseIdentities(f)
}

// escapeLine replaces newlines with \\n for single-line storage.
func escapeLine(s string) string {
	return strings.ReplaceAll(s, "\n", "\\n")
}

// unescapeLine restores newlines.
func unescapeLine(s string) string {
	return strings.ReplaceAll(s, "\\n", "\n")
}

// ParseEncryptedKeys extracts the cleartext keys from an encrypted file
// (without decrypting values). Useful for validation without a key.
func ParseEncryptedKeys(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var keys []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx > 0 {
			keys = append(keys, line[:idx])
		}
	}
	sort.Strings(keys)
	return keys, nil
}
