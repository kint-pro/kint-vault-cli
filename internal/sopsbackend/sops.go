// Package sopsbackend provides native age encryption for dotenv files.
// No external sops or age binaries required.
package sopsbackend

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"filippo.io/age"
	"filippo.io/age/armor"
	"github.com/kint-pro/kint-vault-cli/internal/config"
)

// RunCmd executes an external command (still used for edit via $EDITOR).
func RunCmd(cmd []string, capture bool) (string, error) {
	c := exec.Command(cmd[0], cmd[1:]...)
	if !capture {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		err := c.Run()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return "", fmt.Errorf("Command failed: %s exited with code %d", cmd[0], exitErr.ExitCode())
			}
			return "", fmt.Errorf("Command not found: %s. Is it installed?", cmd[0])
		}
		return "", nil
	}
	c.Stderr = os.Stderr
	out, err := c.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("Command failed: %s exited with code %d", cmd[0], exitErr.ExitCode())
		}
		return "", fmt.Errorf("Command not found: %s. Is it installed?", cmd[0])
	}
	return strings.TrimSpace(string(out)), nil
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

// loadRecipients parses age public keys from the config.
func loadRecipients() ([]age.Recipient, error) {
	pubkeys, err := config.GetRecipients()
	if err != nil {
		return nil, err
	}
	if len(pubkeys) == 0 {
		return nil, fmt.Errorf("no recipients configured. Run: kint-vault init")
	}
	var recipients []age.Recipient
	for _, pk := range pubkeys {
		r, err := age.ParseX25519Recipient(pk)
		if err != nil {
			return nil, fmt.Errorf("invalid recipient %s: %v", pk, err)
		}
		recipients = append(recipients, r)
	}
	return recipients, nil
}

// Decrypt decrypts the encrypted env file and returns the plaintext content.
func Decrypt(cfg *config.Config, directory string) (string, error) {
	enc := config.ResolveEnc(cfg, directory)
	if _, err := os.Stat(enc); os.IsNotExist(err) {
		return "", fmt.Errorf("No encrypted secrets: %s. Run: kint-vault push", enc)
	}

	data, err := os.ReadFile(enc)
	if err != nil {
		return "", err
	}

	identities, err := loadIdentities()
	if err != nil {
		return "", fmt.Errorf("Decryption failed for %s. %v\nRun: kint-vault doctor", enc, err)
	}

	var reader io.Reader = bytes.NewReader(data)
	// Try armored first, fall back to binary
	armorReader := armor.NewReader(bytes.NewReader(data))
	if _, err := armorReader.Read(make([]byte, 1)); err == nil {
		// Reset and use armor reader
		reader = armor.NewReader(bytes.NewReader(data))
	} else {
		reader = bytes.NewReader(data)
	}

	decrypted, err := age.Decrypt(reader, identities...)
	if err != nil {
		return "", fmt.Errorf("Decryption failed for %s. Your key may not be in recipients.\nRun: kint-vault doctor", enc)
	}

	plaintext, err := io.ReadAll(decrypted)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(plaintext)), nil
}

// EncryptToFile encrypts content and writes it to the encrypted env file.
func EncryptToFile(cfg *config.Config, plaintext, directory string) error {
	recipients, err := loadRecipients()
	if err != nil {
		return fmt.Errorf("Encryption failed: %v\nRun: kint-vault doctor", err)
	}

	var buf bytes.Buffer
	armorWriter := armor.NewWriter(&buf)

	w, err := age.Encrypt(armorWriter, recipients...)
	if err != nil {
		return fmt.Errorf("Encryption failed: %v", err)
	}
	if _, err := w.Write([]byte(plaintext + "\n")); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	if err := armorWriter.Close(); err != nil {
		return err
	}

	enc := config.ResolveEnc(cfg, directory)
	return config.AtomicWrite(enc, buf.Bytes())
}

// EncryptFile reads a plaintext file, encrypts it, and writes the enc file.
func EncryptFile(cfg *config.Config, inputFile, directory string) error {
	data, err := os.ReadFile(inputFile)
	if err != nil {
		return err
	}
	return EncryptToFile(cfg, strings.TrimSpace(string(data)), directory)
}

// EncryptContent encrypts content string and writes to enc file in cwd.
func EncryptContent(cfg *config.Config, content string) error {
	return EncryptToFile(cfg, content, "")
}

// ReEncrypt decrypts and re-encrypts a file (for key rotation / recipient changes).
func ReEncrypt(cfg *config.Config, encPath string) error {
	// Read the encrypted file
	data, err := os.ReadFile(encPath)
	if err != nil {
		return err
	}

	// Decrypt
	identities, err := loadIdentities()
	if err != nil {
		return err
	}

	var reader io.Reader = bytes.NewReader(data)
	armorReader := armor.NewReader(bytes.NewReader(data))
	if _, err := armorReader.Read(make([]byte, 1)); err == nil {
		reader = armor.NewReader(bytes.NewReader(data))
	} else {
		reader = bytes.NewReader(data)
	}

	decrypted, err := age.Decrypt(reader, identities...)
	if err != nil {
		return fmt.Errorf("decryption failed: %v", err)
	}
	plaintext, err := io.ReadAll(decrypted)
	if err != nil {
		return err
	}

	// Re-encrypt with current recipients
	recipients, err := loadRecipients()
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	armorWriter := armor.NewWriter(&buf)
	w, err := age.Encrypt(armorWriter, recipients...)
	if err != nil {
		return err
	}
	if _, err := w.Write(plaintext); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	if err := armorWriter.Close(); err != nil {
		return err
	}

	return config.AtomicWrite(encPath, buf.Bytes())
}
