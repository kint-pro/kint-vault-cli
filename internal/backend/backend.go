package backend

import (
	"fmt"
	"os"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/envfile"
	"github.com/kint-pro/kint-vault-cli/internal/vault"
)


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

func DecryptForUpdate(cfg *config.Config) (*vault.DecryptResult, error) {
	enc := config.EncFile(cfg)
	if _, err := os.Stat(enc); os.IsNotExist(err) {
		return nil, nil
	}
	result, err := vault.DecryptForUpdate(enc)
	if err != nil {
		return nil, fmt.Errorf("Decryption failed for %s. Your key may not be in recipients.\nRun: kint-vault doctor", enc)
	}
	return result, nil
}

func EncryptFile(cfg *config.Config, inputFile, directory string) error {
	data, err := os.ReadFile(inputFile)
	if err != nil {
		return err
	}
	secrets := envfile.Parse(string(data))
	enc := config.ResolveEnc(cfg, directory)

	if existing, err := vault.DecryptForUpdate(enc); err == nil && existing != nil {
		return vault.WriteEncryptedFileWithKey(enc, secrets, existing.DataKey, existing.Cipher)
	}

	return vault.WriteEncryptedFile(enc, secrets)
}

func EncryptContent(cfg *config.Config, content string) error {
	secrets := envfile.Parse(content)
	enc := config.EncFile(cfg)
	return vault.WriteEncryptedFile(enc, secrets)
}

func EncryptContentWithKey(cfg *config.Config, content string, dataKey []byte, c *vault.Cipher) error {
	secrets := envfile.Parse(content)
	enc := config.EncFile(cfg)
	return vault.WriteEncryptedFileWithKey(enc, secrets, dataKey, c)
}

func ReEncrypt(cfg *config.Config, encPath string) error {
	return vault.ReEncryptFile(encPath)
}
