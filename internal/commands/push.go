package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/envfile"
	"github.com/kint-pro/kint-vault-cli/internal/output"
	"github.com/kint-pro/kint-vault-cli/internal/sopsbackend"
)

func pushSingle(cfg *config.Config, envFile string, yes bool) {
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		fatal(fmt.Sprintf("File not found: %s. Create a .env file with your secrets first.", envFile))
	}
	content, err := os.ReadFile(envFile)
	if err != nil {
		fatal(err.Error())
	}
	secrets := envfile.Parse(string(content))
	encName := config.EncFile(cfg)
	dir := filepath.Dir(envFile)
	encPath := filepath.Join(dir, encName)
	isNew := true
	if _, err := os.Stat(encPath); err == nil {
		isNew = false
	}

	if !yes {
		if isNew {
			output.Info(fmt.Sprintf("Creating new environment: %s", cfg.Env))
		}
		output.Info(fmt.Sprintf("Will encrypt %d secrets → %s:", len(secrets), encPath))
		for _, key := range envfile.SortedKeys(secrets) {
			fmt.Printf("  %s\n", key)
		}
		if !confirm("Continue? [y/N]") {
			fatal("Aborted")
		}
	}

	if err := sopsbackend.EncryptFile(cfg, envFile, dir); err != nil {
		fatal(err.Error())
	}

	if isNew {
		output.Ok(fmt.Sprintf("Created %s with %d secrets", encPath, len(secrets)))
	} else {
		output.Ok(fmt.Sprintf("Encrypted %d secrets → %s", len(secrets), encPath))
	}
}

func CmdPush(envOverride, file string, yes, all bool) {
	cfg, err := config.LoadConfig(envOverride)
	if err != nil {
		fatal(err.Error())
	}

	if all {
		configPath, err := config.FindConfigPath()
		if err != nil {
			fatal(err.Error())
		}
		root := filepath.Dir(configPath)
		envFiles := findAllEnvFiles(root)
		if len(envFiles) == 0 {
			fatal("No .env files found")
		}
		output.Info(fmt.Sprintf("Found %d .env file(s):", len(envFiles)))
		for _, f := range envFiles {
			content, _ := os.ReadFile(f)
			count := len(envfile.Parse(string(content)))
			rel, _ := filepath.Rel(root, f)
			fmt.Printf("  %s (%d secrets)\n", rel, count)
		}
		if !yes {
			if !confirm("Encrypt all? [y/N]") {
				fatal("Aborted")
			}
		}
		for _, f := range envFiles {
			pushSingle(cfg, f, true)
		}
		return
	}

	envFile := file
	if envFile == "" {
		envFile = ".env"
	}
	pushSingle(cfg, envFile, yes)
}
