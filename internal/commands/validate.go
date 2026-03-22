package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/envfile"
	"github.com/kint-pro/kint-vault-cli/internal/output"
	"github.com/kint-pro/kint-vault-cli/internal/sopsbackend"
)

type validateResult struct {
	Valid   bool     `json:"valid"`
	Missing []string `json:"missing"`
	Extra   []string `json:"extra"`
}

func validateSingle(cfg *config.Config, encDir, templatePath, label string, strict, asJSON bool) (interface{}, bool) {
	tplContent, err := os.ReadFile(templatePath)
	if err != nil {
		fatal(err.Error())
	}
	required := envfile.Parse(string(tplContent))
	requiredKeys := make(map[string]bool)
	for k := range required {
		requiredKeys[k] = true
	}

	content, err := sopsbackend.Decrypt(cfg, encDir)
	if err != nil {
		fatal(err.Error())
	}
	remoteKeys := make(map[string]bool)
	for k := range envfile.Parse(content) {
		remoteKeys[k] = true
	}

	var missing []string
	for k := range requiredKeys {
		if !remoteKeys[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)

	var extra []string
	if strict {
		for k := range remoteKeys {
			if !requiredKeys[k] {
				extra = append(extra, k)
			}
		}
		sort.Strings(extra)
	}

	if asJSON {
		return validateResult{
			Valid:   len(missing) == 0 && len(extra) == 0,
			Missing: missing,
			Extra:   extra,
		}, len(missing) == 0 && len(extra) == 0
	}

	if len(missing) == 0 && len(extra) == 0 {
		output.Ok(fmt.Sprintf("%s: all %d keys present", label, len(requiredKeys)))
		return nil, true
	}

	if len(missing) > 0 {
		output.Err(fmt.Sprintf("%s: missing %d keys:", label, len(missing)))
		for _, k := range missing {
			fmt.Printf("  - %s\n", k)
		}
	}
	if len(extra) > 0 {
		output.Info(fmt.Sprintf("%s: extra %d keys:", label, len(extra)))
		for _, k := range extra {
			fmt.Printf("  + %s\n", k)
		}
	}
	return nil, false
}

func CmdValidate(envOverride, template string, strict, asJSON, all bool) {
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

		allResults := make(map[string]interface{})
		allOk := true
		for _, enc := range encFiles {
			dir := filepath.Dir(enc)
			tplName := template
			if tplName == "" {
				tplName = config.EnvExample
			}
			tplPath := filepath.Join(dir, tplName)
			label, _ := filepath.Rel(root, dir)

			if _, err := os.Stat(tplPath); os.IsNotExist(err) {
				if !asJSON {
					output.Info(fmt.Sprintf("%s: no %s, skipping", label, filepath.Base(tplPath)))
				}
				continue
			}

			if asJSON {
				result, ok := validateSingle(cfg, dir, tplPath, label, strict, true)
				allResults[label] = result
				if !ok {
					allOk = false
				}
			} else {
				_, ok := validateSingle(cfg, dir, tplPath, label, strict, false)
				if !ok {
					allOk = false
				}
			}
		}
		if asJSON {
			data, _ := json.MarshalIndent(allResults, "", "  ")
			fmt.Println(string(data))
		}
		if !allOk {
			os.Exit(1)
		}
		return
	}

	tpl := template
	if tpl == "" {
		tpl = config.EnvExample
	}
	if _, err := os.Stat(tpl); os.IsNotExist(err) {
		fatal(fmt.Sprintf("Template not found: %s. Create a %s with required keys", tpl, config.EnvExample))
	}

	cwd, _ := os.Getwd()
	if asJSON {
		result, ok := validateSingle(cfg, cwd, tpl, config.EncFile(cfg), strict, true)
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		if !ok {
			os.Exit(1)
		}
		return
	}

	_, ok := validateSingle(cfg, cwd, tpl, config.EncFile(cfg), strict, false)
	if !ok {
		os.Exit(1)
	}
}
