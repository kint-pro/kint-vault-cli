package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/output"
	"github.com/kint-pro/kint-vault-cli/internal/backend"
)

func CmdAddRecipient(pubkey string) {
	if !strings.HasPrefix(pubkey, "age1") {
		fatal("Invalid age public key (must start with age1)")
	}

	cfg, err := config.LoadConfig("")
	if err != nil {
		fatal(err.Error())
	}

	for _, r := range cfg.Recipients {
		if r == pubkey {
			fatal("Recipient already exists")
		}
	}

	cfg.Recipients = append(cfg.Recipients, pubkey)
	if err := config.SaveConfig(cfg); err != nil {
		fatal(err.Error())
	}

	truncated := pubkey
	if len(truncated) > 20 {
		truncated = truncated[:20] + "..."
	}
	output.Ok(fmt.Sprintf("Added recipient: %s", truncated))

	configPath, err := config.FindConfigPath()
	if err != nil {
		fatal(err.Error())
	}
	root := filepath.Dir(configPath)
	for _, enc := range findAllEncFilesAnyEnv(root) {
		if err := backend.ReEncrypt(nil, enc); err != nil {
			fatal(err.Error())
		}
		rel, _ := filepath.Rel(root, enc)
		output.Ok(fmt.Sprintf("Updated keys in %s", rel))
	}
}

func CmdRemoveRecipient(pubkey string) {
	cfg, err := config.LoadConfig("")
	if err != nil {
		fatal(err.Error())
	}

	idx := -1
	for i, r := range cfg.Recipients {
		if r == pubkey {
			idx = i
			break
		}
	}
	if idx < 0 {
		fatal("Recipient not found")
	}
	if len(cfg.Recipients) == 1 {
		fatal("Cannot remove last recipient")
	}

	cfg.Recipients = append(cfg.Recipients[:idx], cfg.Recipients[idx+1:]...)
	if err := config.SaveConfig(cfg); err != nil {
		fatal(err.Error())
	}

	truncated := pubkey
	if len(truncated) > 20 {
		truncated = truncated[:20] + "..."
	}
	output.Ok(fmt.Sprintf("Removed recipient: %s", truncated))

	configPath, err := config.FindConfigPath()
	if err != nil {
		fatal(err.Error())
	}
	root := filepath.Dir(configPath)
	for _, enc := range findAllEncFilesAnyEnv(root) {
		if err := backend.ReEncrypt(nil, enc); err != nil {
			fatal(err.Error())
		}
		rel, _ := filepath.Rel(root, enc)
		output.Ok(fmt.Sprintf("Updated keys and rotated data key in %s", rel))
	}
	output.Info("Removed recipients can still decrypt old versions from git history")
}
