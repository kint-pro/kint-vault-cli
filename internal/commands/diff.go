package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/envfile"
	"github.com/kint-pro/kint-vault-cli/internal/output"
	"github.com/kint-pro/kint-vault-cli/internal/sopsbackend"
)

func CmdDiff(envOverride string, asJSON, all bool) {
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

		allResults := make(map[string]envfile.DiffData)
		for _, enc := range encFiles {
			dir := filepath.Dir(enc)
			localPath := filepath.Join(dir, ".env")
			label, _ := filepath.Rel(root, dir)

			if _, err := os.Stat(localPath); os.IsNotExist(err) {
				if !asJSON {
					output.Info(fmt.Sprintf("%s: no local .env", label))
				}
				continue
			}

			localContent, _ := os.ReadFile(localPath)
			localDict := envfile.Parse(string(localContent))

			remoteContent, err := sopsbackend.Decrypt(cfg, dir)
			if err != nil {
				fatal(err.Error())
			}
			remoteDict := envfile.Parse(remoteContent)

			if asJSON {
				allResults[label] = envfile.ComputeDiff(localDict, remoteDict)
			} else {
				result := formatDiff(localDict, remoteDict)
				if result == "No differences" {
					output.Ok(fmt.Sprintf("%s: no differences", label))
				} else {
					output.Info(fmt.Sprintf("%s:", label))
					fmt.Println(result)
				}
			}
		}
		if asJSON {
			data, _ := json.MarshalIndent(allResults, "", "  ")
			fmt.Println(string(data))
		}
		return
	}

	localPath := ".env"
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		fatal("No local .env file to diff against")
	}

	localContent, _ := os.ReadFile(localPath)
	localDict := envfile.Parse(string(localContent))

	remoteContent, err := sopsbackend.Decrypt(cfg, "")
	if err != nil {
		fatal(err.Error())
	}
	remoteDict := envfile.Parse(remoteContent)

	if asJSON {
		data, _ := json.MarshalIndent(envfile.ComputeDiff(localDict, remoteDict), "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Println(formatDiff(localDict, remoteDict))
	}
}
