package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/output"
	"github.com/kint-pro/kint-vault-cli/internal/sopsbackend"
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
		if err := os.MkdirAll(keyFile[:len(keyFile)-len("/keys.txt")], 0o700); err != nil {
			fatal(fmt.Sprintf("Failed to create key directory: %v", err))
		}
		_, err := sopsbackend.RunCmd([]string{"age-keygen", "-o", keyFile}, true)
		if err != nil {
			fatal(fmt.Sprintf("Failed to generate age key. Is age installed?\nRun: kint-vault doctor"))
		}
		config.RestrictFile(keyFile)
		pubkey, err = config.ReadAgePubkey(keyFile)
		if err != nil {
			fatal(err.Error())
		}
		output.Ok("Generated age key")
	}

	cfg := &config.Config{Backend: "sops", Env: env}
	data, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		fatal(err.Error())
	}

	sopsPath := config.SopsConfig
	if _, err := os.Stat(sopsPath); err == nil {
		// existing .sops.yaml
		sopsData, err := os.ReadFile(sopsPath)
		if err != nil {
			fatal(err.Error())
		}
		var existing config.SopsConfigFile
		if err := yaml.Unmarshal(sopsData, &existing); err != nil {
			fatal(err.Error())
		}
		if len(existing.CreationRules) > 0 {
			age := existing.CreationRules[0].Age
			var recipients []string
			for _, r := range strings.Split(age, ",") {
				r = strings.TrimSpace(r)
				if r != "" {
					recipients = append(recipients, r)
				}
			}
			found := false
			for _, r := range recipients {
				if r == pubkey {
					found = true
					break
				}
			}
			if !found {
				recipients = append(recipients, pubkey)
				existing.CreationRules[0].Age = strings.Join(recipients, ",")
				out, _ := yaml.Marshal(&existing)
				if err := os.WriteFile(sopsPath, out, 0o644); err != nil {
					fatal(err.Error())
				}
				output.Ok(fmt.Sprintf("Added your key to %s", config.SopsConfig))
			}
		}
	} else {
		sopsConf := config.SopsConfigFile{
			CreationRules: []config.CreationRule{
				{PathRegex: `\.env`, Age: pubkey},
			},
		}
		out, _ := yaml.Marshal(&sopsConf)
		if err := os.WriteFile(sopsPath, out, 0o644); err != nil {
			fatal(err.Error())
		}
		output.Ok(fmt.Sprintf("Created %s", config.SopsConfig))
	}

	// .gitignore
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

	output.Ok(fmt.Sprintf("Initialized (sops+age, env: %s)", env))
	output.Info(fmt.Sprintf("Your public key: %s", pubkey))
	output.Info("Share this key with your team to be added as recipient")
	fmt.Println(aliasHint)
}

const aliasHint = `
Tip: Create a short alias "kv" for kint-vault:

  macOS/Linux (zsh):  echo 'alias kv="kint-vault"' >> ~/.zshrc && source ~/.zshrc
  macOS/Linux (bash): echo 'alias kv="kint-vault"' >> ~/.bashrc && source ~/.bashrc
  Windows (PS):       Set-Alias -Name kv -Value kint-vault -Scope Global`
