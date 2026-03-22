// Package sopsbackend wraps SOPS CLI commands.
package sopsbackend

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kint-pro/kint-vault-cli/internal/config"
)

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

func Decrypt(cfg *config.Config, directory string) (string, error) {
	enc := config.ResolveEnc(cfg, directory)
	if _, err := os.Stat(enc); os.IsNotExist(err) {
		return "", fmt.Errorf("No encrypted secrets: %s. Run: kint-vault push", enc)
	}
	out, err := RunCmd([]string{
		"sops", "decrypt", "--input-type", "dotenv", "--output-type", "dotenv", enc,
	}, true)
	if err != nil {
		return "", fmt.Errorf("decryption failed for %s. Your key may not be in recipients.\nRun: kint-vault doctor", enc)
	}
	return out, nil
}

func EncryptFile(cfg *config.Config, inputFile, directory string) error {
	enc := config.ResolveEnc(cfg, directory)
	out, err := RunCmd([]string{
		"sops", "encrypt", "--input-type", "dotenv", "--output-type", "dotenv", inputFile,
	}, true)
	if err != nil {
		return fmt.Errorf("encryption failed. Is sops installed and .sops.yaml configured?\nRun: kint-vault doctor")
	}
	return config.AtomicWrite(enc, []byte(out+"\n"))
}

func EncryptContent(cfg *config.Config, content string) error {
	enc := config.EncFile(cfg)
	f, err := os.CreateTemp(".", ".env.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)

	if _, err := f.WriteString(content + "\n"); err != nil {
		f.Close()
		return err
	}
	f.Close()
	config.RestrictFile(tmpPath)

	out, err := RunCmd([]string{
		"sops", "encrypt",
		"--input-type", "dotenv", "--output-type", "dotenv",
		"--filename-override", ".env",
		tmpPath,
	}, true)
	if err != nil {
		return fmt.Errorf("encryption failed. Is sops installed and .sops.yaml configured?\nRun: kint-vault doctor")
	}
	return config.AtomicWrite(enc, []byte(out+"\n"))
}
