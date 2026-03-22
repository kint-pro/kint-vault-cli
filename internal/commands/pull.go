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

func pullSingle(cfg *config.Config, encPath, outputPath string, force bool) {
	dir := filepath.Dir(encPath)
	content, err := sopsbackend.Decrypt(cfg, dir)
	if err != nil {
		fatal(err.Error())
	}
	data := content + "\n"

	resolved, err := filepath.Abs(outputPath)
	if err != nil {
		fatal(err.Error())
	}
	parentDir := filepath.Dir(resolved)
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		fatal(fmt.Sprintf("Directory does not exist: %s", parentDir))
	}

	if _, err := os.Stat(resolved); err == nil {
		// File exists
		existing, _ := os.ReadFile(resolved)
		localDict := envfile.Parse(string(existing))
		remoteDict := envfile.Parse(content)
		diff := formatDiff(localDict, remoteDict)
		if diff == "No differences" {
			output.Info("No differences, skipping")
			return
		}
		if !force {
			output.Warn(fmt.Sprintf("%s already exists. Differences:", outputPath))
			fmt.Println(diff)
			if !confirm("Overwrite local .env? [y/N]") {
				fatal("Aborted")
			}
		} else {
			output.Warn("Overwriting. Differences:")
			fmt.Println(diff)
		}
		output.Warn("Previous local .env:")
		fmt.Println(envfile.Format(localDict))
		fd, err := os.OpenFile(resolved, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			fatal(err.Error())
		}
		fd.WriteString(data)
		fd.Close()
	} else {
		fd, err := os.OpenFile(resolved, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			fatal(err.Error())
		}
		fd.WriteString(data)
		fd.Close()
	}

	count := len(envfile.Parse(content))
	output.Ok(fmt.Sprintf("Decrypted %d secrets → %s", count, outputPath))
}

func CmdPull(envOverride string, outputFile string, asJSON, stdout, force, all bool) {
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
		output.Info(fmt.Sprintf("Found %d encrypted file(s):", len(encFiles)))
		for _, f := range encFiles {
			rel, _ := filepath.Rel(root, f)
			fmt.Printf("  %s\n", rel)
		}
		for _, enc := range encFiles {
			out := filepath.Join(filepath.Dir(enc), ".env")
			pullSingle(cfg, enc, out, force)
		}
		return
	}

	if asJSON {
		content, err := sopsbackend.Decrypt(cfg, "")
		if err != nil {
			fatal(err.Error())
		}
		parsed := envfile.Parse(content)
		data, _ := json.MarshalIndent(parsed, "", "  ")
		fmt.Println(string(data))
		return
	}

	if stdout {
		content, err := sopsbackend.Decrypt(cfg, "")
		if err != nil {
			fatal(err.Error())
		}
		fmt.Println(content)
		return
	}

	enc := config.EncFile(cfg)
	out := outputFile
	if out == "" {
		out = ".env"
	}
	pullSingle(cfg, enc, out, force)
}
