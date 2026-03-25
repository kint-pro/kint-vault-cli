package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kint-pro/kint-vault-cli/internal/commands"
)

var version = "dev"

func usage() {
	fmt.Fprintf(os.Stderr, `kint-vault — Unified secrets management CLI

Usage:
  kint-vault <command> [flags]

Commands:
  init              Initialize project config, generate age key
  pull              Decrypt secrets to .env
  push              Encrypt .env to vault
  run               Run a command with secrets injected as env vars
  set               Set one or more secrets (KEY=VALUE ...)
  get               Get a single secret value
  delete            Delete one or more secrets
  list              List all secret keys
  diff              Show differences between local .env and encrypted vault
  edit              Edit encrypted secrets in $EDITOR
  rotate            Rotate data encryption key
  add-recipient     Add a team member's age public key
  remove-recipient  Remove a team member's age public key
  validate          Check secrets against .env.example template
  doctor            Verify installation and setup
  env               Show or switch the active environment

Flags:
  --version         Show version
  --help            Show this help

`)
}

func reorderArgs(fs *flag.FlagSet, args []string) []string {
	known := make(map[string]bool)
	hasValue := make(map[string]bool)
	fs.VisitAll(func(f *flag.Flag) {
		known[f.Name] = true
		if f.DefValue == "false" || f.DefValue == "true" {
			return
		}
		hasValue[f.Name] = true
	})

	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i:]...)
			break
		}
		name := ""
		if strings.HasPrefix(a, "--") {
			name = strings.TrimPrefix(a, "--")
			if idx := strings.Index(name, "="); idx >= 0 {
				name = name[:idx]
			}
		} else if strings.HasPrefix(a, "-") && len(a) > 1 {
			name = strings.TrimPrefix(a, "-")
			if idx := strings.Index(name, "="); idx >= 0 {
				name = name[:idx]
			}
		}
		if name != "" && known[name] {
			flags = append(flags, a)
			if hasValue[name] && !strings.Contains(a, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			positional = append(positional, a)
		}
	}
	return append(flags, positional...)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		fmt.Fprintf(os.Stderr, `Tip: Create a short alias "kv" for kint-vault:

  macOS/Linux (zsh):  echo 'alias kv="kint-vault"' >> ~/.zshrc && source ~/.zshrc
  macOS/Linux (bash): echo 'alias kv="kint-vault"' >> ~/.bashrc && source ~/.bashrc
  Windows (PS):       Set-Alias -Name kv -Value kint-vault -Scope Global

`)
		os.Exit(1)
	}

	if os.Args[1] == "--version" || os.Args[1] == "-v" {
		fmt.Printf("kint-vault %s\n", version)
		return
	}
	if os.Args[1] == "--help" || os.Args[1] == "-h" {
		usage()
		return
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "init":
		fs := flag.NewFlagSet("init", flag.ExitOnError)
		env := fs.String("env", "", "Default environment")
		force := fs.Bool("force", false, "Overwrite existing config")
		fs.Parse(reorderArgs(fs, args))
		commands.CmdInit(*env, *force)

	case "pull":
		fs := flag.NewFlagSet("pull", flag.ExitOnError)
		env := fs.String("env", "", "Override environment")
		output := fs.String("output", "", "Output file (default: .env)")
		fs.StringVar(output, "o", "", "Output file (default: .env)")
		asJSON := fs.Bool("json", false, "Output as JSON")
		stdout := fs.Bool("stdout", false, "Print to stdout only")
		force := fs.Bool("force", false, "Overwrite existing .env file")
		all := fs.Bool("all", false, "Decrypt all services in monorepo")
		fs.Parse(reorderArgs(fs, args))
		commands.CmdPull(*env, *output, *asJSON, *stdout, *force, *all)

	case "push":
		fs := flag.NewFlagSet("push", flag.ExitOnError)
		env := fs.String("env", "", "Override environment")
		file := fs.String("file", "", "File to encrypt (default: .env)")
		fs.StringVar(file, "f", "", "File to encrypt (default: .env)")
		yes := fs.Bool("yes", false, "Skip confirmation")
		fs.BoolVar(yes, "y", false, "Skip confirmation")
		all := fs.Bool("all", false, "Encrypt all .env files in monorepo")
		fs.Parse(reorderArgs(fs, args))
		commands.CmdPush(*env, *file, *yes, *all)

	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		env := fs.String("env", "", "Override environment")
		var flagArgs, cmdArgs []string
		dashIdx := -1
		for i, a := range args {
			if a == "--" {
				dashIdx = i
				break
			}
		}
		if dashIdx >= 0 {
			flagArgs = args[:dashIdx]
			cmdArgs = args[dashIdx:] // includes "--"
		} else {
			flagArgs = args
		}
		fs.Parse(flagArgs)
		remaining := append(fs.Args(), cmdArgs...)
		commands.CmdRun(*env, remaining)

	case "set":
		fs := flag.NewFlagSet("set", flag.ExitOnError)
		env := fs.String("env", "", "Override environment")
		fs.Parse(reorderArgs(fs, args))
		if fs.NArg() == 0 {
			fmt.Fprintln(os.Stderr, "Usage: kint-vault set KEY=VALUE [KEY=VALUE ...]")
			os.Exit(1)
		}
		commands.CmdSet(*env, fs.Args())

	case "get":
		fs := flag.NewFlagSet("get", flag.ExitOnError)
		env := fs.String("env", "", "Override environment")
		fs.Parse(reorderArgs(fs, args))
		if fs.NArg() == 0 {
			fmt.Fprintln(os.Stderr, "Usage: kint-vault get KEY")
			os.Exit(1)
		}
		commands.CmdGet(*env, fs.Arg(0))

	case "delete":
		fs := flag.NewFlagSet("delete", flag.ExitOnError)
		env := fs.String("env", "", "Override environment")
		yes := fs.Bool("yes", false, "Skip confirmation")
		fs.BoolVar(yes, "y", false, "Skip confirmation")
		fs.Parse(reorderArgs(fs, args))
		if fs.NArg() == 0 {
			fmt.Fprintln(os.Stderr, "Usage: kint-vault delete KEY [KEY ...]")
			os.Exit(1)
		}
		commands.CmdDelete(*env, fs.Args(), *yes)

	case "list":
		fs := flag.NewFlagSet("list", flag.ExitOnError)
		env := fs.String("env", "", "Override environment")
		asJSON := fs.Bool("json", false, "Output as JSON")
		all := fs.Bool("all", false, "List keys for all services in monorepo")
		fs.Parse(reorderArgs(fs, args))
		commands.CmdList(*env, *asJSON, *all)

	case "diff":
		fs := flag.NewFlagSet("diff", flag.ExitOnError)
		env := fs.String("env", "", "Override environment")
		asJSON := fs.Bool("json", false, "Output as JSON")
		all := fs.Bool("all", false, "Diff all services in monorepo")
		fs.Parse(reorderArgs(fs, args))
		commands.CmdDiff(*env, *asJSON, *all)

	case "edit":
		fs := flag.NewFlagSet("edit", flag.ExitOnError)
		env := fs.String("env", "", "Override environment")
		fs.Parse(reorderArgs(fs, args))
		commands.CmdEdit(*env)

	case "rotate":
		fs := flag.NewFlagSet("rotate", flag.ExitOnError)
		env := fs.String("env", "", "Override environment")
		all := fs.Bool("all", false, "Rotate all services in monorepo")
		fs.Parse(reorderArgs(fs, args))
		commands.CmdRotate(*env, *all)

	case "add-recipient":
		fs := flag.NewFlagSet("add-recipient", flag.ExitOnError)
		fs.Parse(args)
		if fs.NArg() == 0 {
			fmt.Fprintln(os.Stderr, "Usage: kint-vault add-recipient AGE_PUBLIC_KEY")
			os.Exit(1)
		}
		commands.CmdAddRecipient(fs.Arg(0))

	case "remove-recipient":
		fs := flag.NewFlagSet("remove-recipient", flag.ExitOnError)
		fs.Parse(args)
		if fs.NArg() == 0 {
			fmt.Fprintln(os.Stderr, "Usage: kint-vault remove-recipient AGE_PUBLIC_KEY")
			os.Exit(1)
		}
		commands.CmdRemoveRecipient(fs.Arg(0))

	case "validate":
		fs := flag.NewFlagSet("validate", flag.ExitOnError)
		env := fs.String("env", "", "Override environment")
		template := fs.String("template", "", "Template file (default: .env.example)")
		fs.StringVar(template, "t", "", "Template file (default: .env.example)")
		strict := fs.Bool("strict", false, "Fail on extra keys too")
		asJSON := fs.Bool("json", false, "Output as JSON")
		all := fs.Bool("all", false, "Validate all services in monorepo")
		fs.Parse(reorderArgs(fs, args))
		commands.CmdValidate(*env, *template, *strict, *asJSON, *all)

	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ExitOnError)
		env := fs.String("env", "", "Override environment")
		fs.Parse(reorderArgs(fs, args))
		commands.CmdDoctor(*env)

	case "env":
		fs := flag.NewFlagSet("env", flag.ExitOnError)
		fs.Parse(args)
		name := ""
		if fs.NArg() > 0 {
			name = fs.Arg(0)
		}
		commands.CmdEnv(name)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		usage()
		os.Exit(1)
	}
}
