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

		type diffResult struct {
			label     string
			hasLocal  bool
			diffText  string
			diffData  envfile.DiffData
		}

		// Parallel decrypt + diff
		results := runParallel(len(encFiles), func(i int) (string, interface{}, error) {
			dir := filepath.Dir(encFiles[i])
			localPath := filepath.Join(dir, ".env")
			label, _ := filepath.Rel(root, dir)

			if _, err := os.Stat(localPath); os.IsNotExist(err) {
				return "", &diffResult{label: label, hasLocal: false}, nil
			}

			localContent, _ := os.ReadFile(localPath)
			localDict := envfile.Parse(string(localContent))

			remoteContent, err := sopsbackend.Decrypt(cfg, dir)
			if err != nil {
				return "", nil, err
			}
			remoteDict := envfile.Parse(remoteContent)

			return "", &diffResult{
				label:    label,
				hasLocal: true,
				diffText: formatDiff(localDict, remoteDict),
				diffData: envfile.ComputeDiff(localDict, remoteDict),
			}, nil
		})
		if err := collectErrors(results); err != nil {
			fatal(err.Error())
		}

		if asJSON {
			allResults := make(map[string]envfile.DiffData)
			for _, r := range results {
				d := r.Data.(*diffResult)
				if d.hasLocal {
					allResults[d.label] = d.diffData
				}
			}
			data, _ := json.MarshalIndent(allResults, "", "  ")
			fmt.Println(string(data))
		} else {
			for _, r := range results {
				d := r.Data.(*diffResult)
				if !d.hasLocal {
					output.Info(fmt.Sprintf("%s: no local .env", d.label))
					continue
				}
				if d.diffText == "No differences" {
					output.Ok(fmt.Sprintf("%s: no differences", d.label))
				} else {
					output.Info(fmt.Sprintf("%s:", d.label))
					fmt.Println(d.diffText)
				}
			}
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
