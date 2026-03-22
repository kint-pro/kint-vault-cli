package commands

import (
	"fmt"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/output"
)

func CmdEnv(name string) {
	cfg, err := config.LoadConfig("")
	if err != nil {
		fatal(err.Error())
	}

	if name != "" {
		if err := config.ValidateEnvName(name); err != nil {
			fatal(err.Error())
		}
		cfg.Env = name
		if err := config.SaveConfig(cfg); err != nil {
			fatal(err.Error())
		}
		output.Ok(fmt.Sprintf("Switched to environment: %s", name))
	} else {
		env := cfg.Env
		if env == "" {
			env = "dev"
		}
		fmt.Println(env)
	}
}
