package commands

import (
	"fmt"
	"os"

	"github.com/kint-pro/kint-vault-cli/internal/config"
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

	_, err = sopsbackend.RunCmd([]string{
		"sops", "edit", "--input-type", "dotenv", "--output-type", "dotenv", enc,
	}, false)
	if err != nil {
		fatal(fmt.Sprintf("Edit failed (%v)", err))
	}
	output.Ok(fmt.Sprintf("Saved %s", enc))
}
