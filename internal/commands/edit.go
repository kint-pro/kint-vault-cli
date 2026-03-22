package commands

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/output"
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

	cmd := exec.Command("sops", "edit", "--input-type", "dotenv", "--output-type", "dotenv", enc)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		fatal(fmt.Sprintf("Edit failed (exit %d)", exitCode))
	}
	output.Ok(fmt.Sprintf("Saved %s", enc))
}
