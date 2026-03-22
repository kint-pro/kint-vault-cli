package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/output"
	"github.com/kint-pro/kint-vault-cli/internal/sopsbackend"
)

func CmdAddRecipient(pubkey string) {
	if !strings.HasPrefix(pubkey, "age1") {
		fatal("Invalid age public key (must start with age1)")
	}

	sops, err := config.LoadSopsConfig()
	if err != nil {
		fatal(err.Error())
	}

	if len(sops.CreationRules) == 0 {
		fatal("No creation_rules in .sops.yaml")
	}

	existing := sops.CreationRules[0].Age
	var recipients []string
	for _, r := range strings.Split(existing, ",") {
		r = strings.TrimSpace(r)
		if r != "" {
			recipients = append(recipients, r)
		}
	}

	for _, r := range recipients {
		if r == pubkey {
			fatal("Recipient already exists")
		}
	}

	recipients = append(recipients, pubkey)
	sops.CreationRules[0].Age = strings.Join(recipients, ",")

	if err := config.SaveSopsConfig(sops); err != nil {
		fatal(err.Error())
	}

	truncated := pubkey
	if len(truncated) > 20 {
		truncated = truncated[:20] + "..."
	}
	output.Ok(fmt.Sprintf("Added recipient: %s", truncated))

	// Re-encrypt all files with updated recipients
	configPath, err := config.FindConfigPath()
	if err != nil {
		fatal(err.Error())
	}
	root := filepath.Dir(configPath)
	for _, enc := range findAllEncFilesAnyEnv(root) {
		if err := sopsbackend.ReEncrypt(nil, enc); err != nil {
			fatal(err.Error())
		}
		rel, _ := filepath.Rel(root, enc)
		output.Ok(fmt.Sprintf("Updated keys in %s", rel))
	}
}

func CmdRemoveRecipient(pubkey string) {
	sops, err := config.LoadSopsConfig()
	if err != nil {
		fatal(err.Error())
	}

	found := false
	for i, rule := range sops.CreationRules {
		var recipients []string
		for _, r := range strings.Split(rule.Age, ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				recipients = append(recipients, r)
			}
		}
		idx := -1
		for j, r := range recipients {
			if r == pubkey {
				idx = j
				break
			}
		}
		if idx >= 0 {
			if len(recipients) == 1 {
				fatal("Cannot remove last recipient")
			}
			recipients = append(recipients[:idx], recipients[idx+1:]...)
			sops.CreationRules[i].Age = strings.Join(recipients, ",")
			found = true
			break
		}
	}

	if !found {
		fatal("Recipient not found")
	}

	if err := config.SaveSopsConfig(sops); err != nil {
		fatal(err.Error())
	}

	truncated := pubkey
	if len(truncated) > 20 {
		truncated = truncated[:20] + "..."
	}
	output.Ok(fmt.Sprintf("Removed recipient: %s", truncated))

	// Re-encrypt all files with updated recipients (also rotates data key)
	configPath, err := config.FindConfigPath()
	if err != nil {
		fatal(err.Error())
	}
	root := filepath.Dir(configPath)
	for _, enc := range findAllEncFilesAnyEnv(root) {
		if err := sopsbackend.ReEncrypt(nil, enc); err != nil {
			fatal(err.Error())
		}
		rel, _ := filepath.Rel(root, enc)
		output.Ok(fmt.Sprintf("Updated keys and rotated data key in %s", rel))
	}
	output.Info("Removed recipients can still decrypt old versions from git history")
}
