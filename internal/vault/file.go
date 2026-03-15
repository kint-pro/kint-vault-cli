package vault

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"
	"filippo.io/age/armor"
	"github.com/kint-pro/kint-vault-cli/internal/config"
)

const (
	formatVersion  = "1"
	metaPrefix     = "kv_"
	recipientKey   = "age_recipient_"
	dataKeyKey     = "age_key"
	macKey         = "mac"
	versionKey     = "version"
)

type encryptedFile struct {
	values         map[string]string
	wrappedDataKey string
	recipients     []string
	mac            string
	version        string
}

type DecryptResult struct {
	Secrets map[string]string
	DataKey []byte
	Cipher  *Cipher
}

func writeFile(path string, secrets map[string]string, dataKey []byte, c *Cipher) error {
	for k := range secrets {
		if strings.HasPrefix(k, metaPrefix) {
			return fmt.Errorf("key %q uses reserved prefix %q", k, metaPrefix)
		}
	}

	recipients, err := config.GetRecipients()
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return fmt.Errorf("no recipients configured. Run: kint-vault init")
	}

	encLines, err := c.encryptAll(secrets, dataKey)
	if err != nil {
		return err
	}

	wrappedKey, err := wrapDataKey(dataKey, recipients)
	if err != nil {
		return err
	}

	mac := computeMAC(secrets, dataKey)
	encMAC, err := c.encryptMAC(mac, dataKey)
	if err != nil {
		return err
	}

	var lines []string
	lines = append(lines, encLines)

	for i, r := range recipients {
		lines = append(lines, fmt.Sprintf("%s%s%d=%s", metaPrefix, recipientKey, i, r))
	}
	lines = append(lines, fmt.Sprintf("%s%s=%s", metaPrefix, dataKeyKey, escapeLine(wrappedKey)))
	lines = append(lines, fmt.Sprintf("%s%s=%s", metaPrefix, macKey, encMAC))
	lines = append(lines, fmt.Sprintf("%s%s=%s", metaPrefix, versionKey, formatVersion))

	content := strings.Join(lines, "\n") + "\n"
	return config.AtomicWrite(path, []byte(content))
}

func WriteEncryptedFile(path string, secrets map[string]string) error {
	dataKey, err := generateDataKey()
	if err != nil {
		return err
	}
	return writeFile(path, secrets, dataKey, NewCipher())
}

func WriteEncryptedFileWithKey(path string, secrets map[string]string, dataKey []byte, c *Cipher) error {
	return writeFile(path, secrets, dataKey, c)
}

func decryptFile(path string) (*DecryptResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	ef, err := parseFile(string(data))
	if err != nil {
		return nil, err
	}

	dataKey, err := unwrapDataKey(ef.wrappedDataKey)
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt data key. Your key may not be in recipients")
	}

	c := NewCipher()
	secrets, err := c.decryptAll(ef.values, dataKey)
	if err != nil {
		return nil, err
	}

	expectedMAC := computeMAC(secrets, dataKey)
	if ef.mac == "" {
		return nil, fmt.Errorf("missing MAC — file may be tampered or corrupted")
	}
	storedMAC := ef.mac
	if isEncryptedValue(storedMAC) {
		decMAC, err := c.decryptMAC(storedMAC, dataKey)
		if err != nil {
			return nil, fmt.Errorf("MAC decryption failed — file may be tampered")
		}
		storedMAC = decMAC
	}
	if storedMAC != expectedMAC {
		return nil, fmt.Errorf("MAC verification failed — file may be tampered")
	}

	return &DecryptResult{Secrets: secrets, DataKey: dataKey, Cipher: c}, nil
}

func ReadEncryptedFile(path string) (map[string]string, error) {
	result, err := decryptFile(path)
	if err != nil {
		return nil, err
	}
	return result.Secrets, nil
}

func DecryptForUpdate(path string) (*DecryptResult, error) {
	return decryptFile(path)
}

func ReEncryptFile(path string) error {
	secrets, err := ReadEncryptedFile(path)
	if err != nil {
		return err
	}
	return WriteEncryptedFile(path, secrets)
}

func parseFile(content string) (*encryptedFile, error) {
	ef := &encryptedFile{
		values: make(map[string]string),
	}

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "#") {
			continue
		}

		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := line[:idx]
		value := line[idx+1:]

		if strings.HasPrefix(key, metaPrefix) {
			metaKey := key[len(metaPrefix):]
			if strings.HasPrefix(metaKey, recipientKey) {
				ef.recipients = append(ef.recipients, strings.TrimSpace(value))
			} else if metaKey == dataKeyKey {
				ef.wrappedDataKey = unescapeLine(value)
			} else if metaKey == macKey {
				ef.mac = value
			} else if metaKey == versionKey {
				ef.version = value
			}
			continue
		}

		ef.values[key] = value
	}

	if ef.wrappedDataKey == "" {
		return nil, fmt.Errorf("no encrypted data key found in file")
	}

	return ef, nil
}


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

func escapeLine(s string) string {
	return strings.ReplaceAll(s, "\n", "\\n")
}

func unescapeLine(s string) string {
	return strings.ReplaceAll(s, "\\n", "\n")
}

