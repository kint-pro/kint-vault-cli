package commands

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/envfile"
	"github.com/kint-pro/kint-vault-cli/internal/output"
	"github.com/kint-pro/kint-vault-cli/internal/sopsbackend"
)

func CmdSet(envOverride string, pairs []string) {
	cfg, err := config.LoadConfig(envOverride)
	if err != nil {
		fatal(err.Error())
	}

	result, err := sopsbackend.DecryptForUpdate(cfg)
	if err != nil {
		fatal(err.Error())
	}

	var secrets map[string]string
	if result != nil {
		secrets = result.Secrets
	} else {
		secrets = make(map[string]string)
	}

	for _, pair := range pairs {
		if !strings.Contains(pair, "=") {
			fatal(fmt.Sprintf("Invalid format: %s. Use KEY=VALUE", pair))
		}
		idx := strings.Index(pair, "=")
		key := pair[:idx]
		if strings.HasPrefix(key, "kv_") {
			fatal(fmt.Sprintf("Key %q uses reserved prefix \"kv_\"", key))
		}
		secrets[key] = pair[idx+1:]
		output.Ok(fmt.Sprintf("Set %s", key))
	}

	if result != nil {
		if err := sopsbackend.EncryptContentWithKey(cfg, envfile.Format(secrets), result.DataKey, result.Cipher); err != nil {
			fatal(err.Error())
		}
	} else {
		if err := sopsbackend.EncryptContent(cfg, envfile.Format(secrets)); err != nil {
			fatal(err.Error())
		}
	}
}

func CmdGet(envOverride, key string) {
	cfg, err := config.LoadConfig(envOverride)
	if err != nil {
		fatal(err.Error())
	}

	content, err := sopsbackend.Decrypt(cfg, "")
	if err != nil {
		fatal(err.Error())
	}

	secrets := envfile.Parse(content)
	val, ok := secrets[key]
	if !ok {
		fatal(fmt.Sprintf("Key not found: %s", key))
	}
	fmt.Println(val)
}

func CmdDelete(envOverride string, keys []string, yes bool) {
	cfg, err := config.LoadConfig(envOverride)
	if err != nil {
		fatal(err.Error())
	}

	result, err := sopsbackend.DecryptForUpdate(cfg)
	if err != nil {
		fatal(err.Error())
	}
	if result == nil {
		fatal(fmt.Sprintf("No encrypted secrets: %s. Run: kint-vault push", config.EncFile(cfg)))
	}

	secrets := result.Secrets

	if !yes {
		output.Info(fmt.Sprintf("Will delete from %s:", config.EncFile(cfg)))
		for _, key := range keys {
			fmt.Printf("  %s\n", key)
		}
		if !confirm("Continue? [y/N]") {
			fatal("Aborted")
		}
	}

	for _, key := range keys {
		if _, ok := secrets[key]; !ok {
			fatal(fmt.Sprintf("Key not found: %s", key))
		}
		delete(secrets, key)
		output.Ok(fmt.Sprintf("Deleted %s", key))
	}

	if err := sopsbackend.EncryptContentWithKey(cfg, envfile.Format(secrets), result.DataKey, result.Cipher); err != nil {
		fatal(err.Error())
	}
}

func CmdList(envOverride string, asJSON, all bool) {
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

		type listData struct {
			label string
			keys  []string
		}
		results := runParallel(len(encFiles), func(i int) (string, interface{}, error) {
			dir := filepath.Dir(encFiles[i])
			label, _ := filepath.Rel(root, dir)
			content, err := sopsbackend.Decrypt(cfg, dir)
			if err != nil {
				return "", nil, err
			}
			keys := envfile.SortedKeys(envfile.Parse(content))
			return "", &listData{label: label, keys: keys}, nil
		})
		if err := collectErrors(results); err != nil {
			fatal(err.Error())
		}

		if asJSON {
			result := make(map[string][]string)
			for _, r := range results {
				d := r.Data.(*listData)
				result[d.label] = d.keys
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			for _, r := range results {
				d := r.Data.(*listData)
				output.Info(fmt.Sprintf("%s (%d keys):", d.label, len(d.keys)))
				for _, k := range d.keys {
					fmt.Printf("  %s\n", k)
				}
			}
		}
		return
	}

	content, err := sopsbackend.Decrypt(cfg, "")
	if err != nil {
		fatal(err.Error())
	}

	keys := envfile.SortedKeys(envfile.Parse(content))
	if asJSON {
		data, _ := json.MarshalIndent(keys, "", "  ")
		fmt.Println(string(data))
	} else {
		for _, k := range keys {
			fmt.Println(k)
		}
	}
}
