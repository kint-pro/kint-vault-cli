package commands

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/envfile"
	"github.com/kint-pro/kint-vault-cli/internal/sopsbackend"
)

func CmdRun(envOverride string, command []string) {
	// Strip leading "--" if present
	if len(command) > 0 && command[0] == "--" {
		command = command[1:]
	}
	if len(command) == 0 {
		fatal("No command specified. Usage: kint-vault run -- <command>")
	}

	cfg, err := config.LoadConfig(envOverride)
	if err != nil {
		fatal(err.Error())
	}

	content, err := sopsbackend.Decrypt(cfg, "")
	if err != nil {
		fatal(err.Error())
	}

	secrets := envfile.Parse(content)
	env := os.Environ()
	for k, v := range secrets {
		env = append(env, k+"="+v)
	}

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				os.Exit(status.ExitStatus())
			}
		}
		os.Exit(1)
	}
}
