package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/kint-pro/kint-vault-cli/internal/config"
	"github.com/kint-pro/kint-vault-cli/internal/envfile"
	"github.com/kint-pro/kint-vault-cli/internal/output"
	"github.com/kint-pro/kint-vault-cli/internal/sopsbackend"
)

func CmdDoctor(envOverride string) {
	cfg, err := config.LoadConfig(envOverride)
	if err != nil {
		fatal(err.Error())
	}

	var checks []output.Check

	checks = append(checks, output.Check{
		Name:   fmt.Sprintf("Config file (%s)", config.ConfigFile),
		Passed: true,
		Detail: "sops",
	})

	sopsVersion, err := sopsbackend.RunCmd([]string{"sops", "--version", "--disable-version-check"}, true)
	if err != nil {
		checks = append(checks, output.Check{Name: "sops installed", Passed: false, Detail: config.InstallHint("sops")})
		output.PrintChecks(checks)
		os.Exit(1)
	}
	checks = append(checks, output.Check{Name: "sops installed", Passed: true, Detail: sopsVersion})

	_, err = sopsbackend.RunCmd([]string{"age", "--version"}, true)
	if err != nil {
		checks = append(checks, output.Check{Name: "age installed", Passed: false, Detail: config.InstallHint("age")})
		output.PrintChecks(checks)
		os.Exit(1)
	}
	checks = append(checks, output.Check{Name: "age installed", Passed: true})

	ageKeyEnv := os.Getenv("SOPS_AGE_KEY")
	keyFile := config.AgeKeyFile()
	var pubkey string

	if ageKeyEnv != "" {
		prefix := "# public key: "
		for _, line := range strings.Split(ageKeyEnv, "\n") {
			if strings.HasPrefix(line, prefix) {
				pubkey = strings.TrimSpace(line[len(prefix):])
				break
			}
		}
		detail := "set"
		if pubkey != "" && len(pubkey) > 20 {
			detail = pubkey[:20] + "..."
		} else if pubkey != "" {
			detail = pubkey
		}
		checks = append(checks, output.Check{Name: "age key (SOPS_AGE_KEY)", Passed: true, Detail: detail})
	} else if _, err := os.Stat(keyFile); err == nil {
		var err error
		pubkey, err = config.ReadAgePubkey(keyFile)
		if err != nil {
			fatal(err.Error())
		}
		detail := pubkey
		if len(detail) > 20 {
			detail = detail[:20] + "..."
		}
		checks = append(checks, output.Check{Name: "age key exists", Passed: true, Detail: detail})
	} else {
		checks = append(checks, output.Check{Name: "age key exists", Passed: false, Detail: "Run: kint-vault init"})
		output.PrintChecks(checks)
		os.Exit(1)
	}

	recipients, err := config.GetRecipients()
	if err != nil {
		checks = append(checks, output.Check{
			Name:   fmt.Sprintf("%s valid", config.SopsConfig),
			Passed: false,
			Detail: "Run: kint-vault init",
		})
		output.PrintChecks(checks)
		os.Exit(1)
	}
	checks = append(checks, output.Check{
		Name:   fmt.Sprintf("%s valid", config.SopsConfig),
		Passed: true,
		Detail: fmt.Sprintf("%d recipient(s)", len(recipients)),
	})

	if pubkey == "" {
		checks = append(checks, output.Check{
			Name:   "Your key in recipients",
			Passed: true,
			Detail: "skipped (no public key to check)",
		})
	} else {
		found := false
		for _, r := range recipients {
			if r == pubkey {
				found = true
				break
			}
		}
		if found {
			checks = append(checks, output.Check{Name: "Your key in recipients", Passed: true})
		} else {
			checks = append(checks, output.Check{
				Name:   "Your key in recipients",
				Passed: false,
				Detail: "Ask team to run: kint-vault add-recipient <your-key>",
			})
		}
	}

	enc := config.EncFile(cfg)
	if _, err := os.Stat(enc); err == nil {
		checks = append(checks, output.Check{
			Name:   fmt.Sprintf("Encrypted file (%s)", enc),
			Passed: true,
		})
		content, err := sopsbackend.Decrypt(cfg, "")
		if err != nil {
			checks = append(checks, output.Check{
				Name:   "Decryption works",
				Passed: false,
				Detail: "Your key may not be in recipients",
			})
		} else {
			count := len(envfile.Parse(content))
			checks = append(checks, output.Check{
				Name:   "Decryption works",
				Passed: true,
				Detail: fmt.Sprintf("%d secrets", count),
			})
		}
	} else {
		checks = append(checks, output.Check{
			Name:   fmt.Sprintf("Encrypted file (%s)", enc),
			Passed: false,
			Detail: "Run: kint-vault push",
		})
	}

	output.PrintChecks(checks)

	allPassed := true
	for _, c := range checks {
		if !c.Passed {
			allPassed = false
			break
		}
	}
	if !allPassed {
		os.Exit(1)
	}
}
