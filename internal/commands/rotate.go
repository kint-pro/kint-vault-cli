package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/output"
	"github.com/kint-pro/kint-vault-cli/internal/sopsbackend"
)

func CmdRotate(envOverride string, all bool) {
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
		envName := cfg.Env
		if envName == "" {
			envName = "dev"
		}
		encFiles := findAllEncFilesRecursive(root, envName)
		if len(encFiles) == 0 {
			fatal(fmt.Sprintf("No .env.%s.enc files found", envName))
		}
		for _, enc := range encFiles {
			if err := sopsbackend.ReEncrypt(cfg, enc); err != nil {
				fatal(err.Error())
			}
			rel, _ := filepath.Rel(root, enc)
			output.Ok(fmt.Sprintf("Rotated data key for %s", rel))
		}
		return
	}

	enc := config.EncFile(cfg)
	if _, err := os.Stat(enc); os.IsNotExist(err) {
		fatal(fmt.Sprintf("No encrypted secrets: %s", enc))
	}
	if err := sopsbackend.ReEncrypt(cfg, enc); err != nil {
		fatal(err.Error())
	}
	output.Ok(fmt.Sprintf("Rotated data key for %s", enc))
}
