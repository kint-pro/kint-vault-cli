// Package sopsbackend wraps the vault package for CLI commands.
// Kept as a thin adapter to minimize changes across command files.
package sopsbackend

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/envfile"
	"github.com/kint-pro/kint-vault-cli/internal/vault"
)

// RunCmd executes an external command (used for $EDITOR in edit command).
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

// Decrypt reads an encrypted file and returns plaintext dotenv content.
func Decrypt(cfg *config.Config, directory string) (string, error) {
	enc := config.ResolveEnc(cfg, directory)
	if _, err := os.Stat(enc); os.IsNotExist(err) {
		return "", fmt.Errorf("No encrypted secrets: %s. Run: kint-vault push", enc)
	}

	secrets, err := vault.ReadEncryptedFile(enc)
	if err != nil {
		return "", fmt.Errorf("Decryption failed for %s. Your key may not be in recipients.\nRun: kint-vault doctor", enc)
	}

	return envfile.Format(secrets), nil
}

// EncryptFile reads a plaintext file and writes encrypted output.
func EncryptFile(cfg *config.Config, inputFile, directory string) error {
	data, err := os.ReadFile(inputFile)
	if err != nil {
		return err
	}
	secrets := envfile.Parse(string(data))
	enc := config.ResolveEnc(cfg, directory)
	return vault.WriteEncryptedFile(enc, secrets)
}

// EncryptContent encrypts content string and writes to enc file in cwd.
func EncryptContent(cfg *config.Config, content string) error {
	secrets := envfile.Parse(content)
	enc := config.EncFile(cfg)
	return vault.WriteEncryptedFile(enc, secrets)
}

// ReEncrypt decrypts and re-encrypts a file (for key rotation / recipient changes).
func ReEncrypt(cfg *config.Config, encPath string) error {
	return vault.ReEncryptFile(encPath)
}
