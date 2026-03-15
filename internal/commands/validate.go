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
	"github.com/kint-pro/kint-vault-cli/internal/backend"
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

	content, err := backend.Decrypt(cfg, encDir)
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

// validateSingleParallel is a goroutine-safe version that returns results without printing.
func validateSingleParallel(cfg *config.Config, encDir, templatePath, label string, strict bool) (validateResult, error) {
	tplContent, err := os.ReadFile(templatePath)
	if err != nil {
		return validateResult{}, err
	}
	required := envfile.Parse(string(tplContent))
	requiredKeys := make(map[string]bool)
	for k := range required {
		requiredKeys[k] = true
	}

	content, err := backend.Decrypt(cfg, encDir)
	if err != nil {
		return validateResult{}, err
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

	return validateResult{
		Valid:   len(missing) == 0 && len(extra) == 0,
		Missing: missing,
		Extra:   extra,
	}, nil
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

		type valEntry struct {
			label       string
			hasTpl      bool
			tplPath     string
			result      validateResult
			requiredCnt int
		}

		// Parallel validate
		results := runParallel(len(encFiles), func(i int) (string, interface{}, error) {
			dir := filepath.Dir(encFiles[i])
			tplName := template
			if tplName == "" {
				tplName = config.EnvExample
			}
			tplPath := filepath.Join(dir, tplName)
			label, _ := filepath.Rel(root, dir)

			if _, err := os.Stat(tplPath); os.IsNotExist(err) {
				return "", &valEntry{label: label, hasTpl: false, tplPath: filepath.Base(tplPath)}, nil
			}

			vr, err := validateSingleParallel(cfg, dir, tplPath, label, strict)
			if err != nil {
				return "", nil, err
			}

			// Count required keys for output
			tplContent, _ := os.ReadFile(tplPath)
			requiredCnt := len(envfile.Parse(string(tplContent)))

			return "", &valEntry{label: label, hasTpl: true, result: vr, requiredCnt: requiredCnt}, nil
		})
		if err := collectErrors(results); err != nil {
			fatal(err.Error())
		}

		allOk := true
		if asJSON {
			allResults := make(map[string]interface{})
			for _, r := range results {
				e := r.Data.(*valEntry)
				if e.hasTpl {
					allResults[e.label] = e.result
					if !e.result.Valid {
						allOk = false
					}
				}
			}
			data, _ := json.MarshalIndent(allResults, "", "  ")
			fmt.Println(string(data))
		} else {
			for _, r := range results {
				e := r.Data.(*valEntry)
				if !e.hasTpl {
					output.Info(fmt.Sprintf("%s: no %s, skipping", e.label, e.tplPath))
					continue
				}
				if e.result.Valid {
					output.Ok(fmt.Sprintf("%s: all %d keys present", e.label, e.requiredCnt))
				} else {
					if len(e.result.Missing) > 0 {
						output.Err(fmt.Sprintf("%s: missing %d keys:", e.label, len(e.result.Missing)))
						for _, k := range e.result.Missing {
							fmt.Printf("  - %s\n", k)
						}
					}
					if len(e.result.Extra) > 0 {
						output.Info(fmt.Sprintf("%s: extra %d keys:", e.label, len(e.result.Extra)))
						for _, k := range e.result.Extra {
							fmt.Printf("  + %s\n", k)
						}
					}
					allOk = false
				}
			}
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
