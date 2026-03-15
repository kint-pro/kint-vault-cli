package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/output"
	"gopkg.in/yaml.v3"
)

func CmdInit(env string, force bool) {
	configPath := config.ConfigFile
	if _, err := os.Stat(configPath); err == nil && !force {
		fatal(fmt.Sprintf("%s already exists. Use --force to overwrite", configPath))
	}

	if env == "" {
		env = prompt("Default environment", "dev")
	}

	keyFile := config.AgeKeyFile()
	var pubkey string
	if _, err := os.Stat(keyFile); err == nil {
		var err error
		pubkey, err = config.ReadAgePubkey(keyFile)
		if err != nil {
			fatal(err.Error())
		}
		output.Info("Using existing age key")
	} else {
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			fatal(fmt.Sprintf("Failed to generate age key: %v", err))
		}
		pubkey = identity.Recipient().String()

		dir := filepath.Dir(keyFile)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			fatal(fmt.Sprintf("Failed to create key directory: %v", err))
		}

		keyContent := fmt.Sprintf("# created: by kint-vault\n# public key: %s\n%s\n",
			pubkey, identity.String())
		if err := os.WriteFile(keyFile, []byte(keyContent), 0o600); err != nil {
			fatal(fmt.Sprintf("Failed to write key file: %v", err))
		}
		config.RestrictFile(keyFile)
		output.Ok("Generated age key")
	}

	recipients := []string{pubkey}
	if _, err := os.Stat(configPath); err == nil {
		existing, err := config.LoadConfig("")
		if err == nil && len(existing.Recipients) > 0 {
			recipients = existing.Recipients
			found := false
			for _, r := range recipients {
				if r == pubkey {
					found = true
					break
				}
			}
			if !found {
				recipients = append(recipients, pubkey)
			}
		}
	}

	cfg := &config.Config{Env: env, Recipients: recipients}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		fatal(err.Error())
	}

	gitignore := ".gitignore"
	entries := []string{".env"}
	if _, err := os.Stat(gitignore); err == nil {
		content, _ := os.ReadFile(gitignore)
		existing := make(map[string]bool)
		for _, line := range strings.Split(string(content), "\n") {
			existing[strings.TrimSpace(line)] = true
		}
		var toAdd []string
		for _, e := range entries {
			if !existing[e] {
				toAdd = append(toAdd, e)
			}
		}
		if len(toAdd) > 0 {
			f, err := os.OpenFile(gitignore, os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				f.WriteString("\n" + strings.Join(toAdd, "\n") + "\n")
				f.Close()
			}
			output.Info(fmt.Sprintf("Added to .gitignore: %s", strings.Join(toAdd, ", ")))
		}
	} else {
		os.WriteFile(gitignore, []byte(strings.Join(entries, "\n")+"\n"), 0o644)
		output.Info("Created .gitignore")
	}

	output.Ok(fmt.Sprintf("Initialized (env: %s)", env))
	output.Info(fmt.Sprintf("Your public key: %s", pubkey))
	output.Info("Share this key with your team to be added as recipient")
}
