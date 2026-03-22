package commands

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/envfile"
	"github.com/kint-pro/kint-vault-cli/internal/output"
	"github.com/kint-pro/kint-vault-cli/internal/sopsbackend"
)

func CmdEdit(envOverride string) {
	cfg, err := config.LoadConfig(envOverride)
	if err != nil {
		fatal(err.Error())
	}

	enc := config.EncFile(cfg)
	if _, err := os.Stat(enc); os.IsNotExist(err) {
		fatal(fmt.Sprintf("No encrypted secrets: %s", enc))
	}

	result, err := sopsbackend.DecryptForUpdate(cfg)
	if err != nil {
		fatal(err.Error())
	}
	if result == nil {
		fatal(fmt.Sprintf("No encrypted secrets: %s", enc))
	}

	tmpFile, err := os.CreateTemp("", "kint-vault-edit-*.env")
	if err != nil {
		fatal(err.Error())
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	tmpFile.WriteString(envfile.Format(result.Secrets) + "\n")
	tmpFile.Close()
	config.RestrictFile(tmpPath)

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		fatal(fmt.Sprintf("Edit failed (exit %d)", exitCode))
	}

	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		fatal(err.Error())
	}

	secrets := envfile.Parse(string(edited))
	if err := sopsbackend.EncryptContentWithKey(cfg, envfile.Format(secrets), result.DataKey, result.Cipher); err != nil {
		fatal(err.Error())
	}

	output.Ok(fmt.Sprintf("Saved %s", enc))
}
