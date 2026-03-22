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

	// Decrypt to temp file
	content, err := sopsbackend.Decrypt(cfg, "")
	if err != nil {
		fatal(err.Error())
	}

	tmpFile, err := os.CreateTemp("", "kint-vault-edit-*.env")
	if err != nil {
		fatal(err.Error())
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	tmpFile.WriteString(content + "\n")
	tmpFile.Close()
	config.RestrictFile(tmpPath)

	// Open in $EDITOR
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

	// Re-encrypt
	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		fatal(err.Error())
	}

	// Validate it parses as env
	_ = envfile.Parse(string(edited))

	if err := sopsbackend.EncryptContent(cfg, envfile.Format(envfile.Parse(string(edited)))); err != nil {
		fatal(err.Error())
	}

	output.Ok(fmt.Sprintf("Saved %s", enc))
}
