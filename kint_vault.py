#!/usr/bin/env python3
"""kint-vault: Unified secrets management CLI."""

import argparse
import json
import os
import subprocess
import sys
from pathlib import Path

import yaml

CONFIG_FILE = ".kint-vault.yaml"
ENV_EXAMPLE = ".env.example"


# --- Output ---

class C:
    RED = "\033[91m"
    GREEN = "\033[92m"
    BLUE = "\033[94m"
    RESET = "\033[0m"


def _color_enabled():
    return sys.stdout.isatty() and os.environ.get("NO_COLOR") is None


def ok(msg: str):
    if _color_enabled():
        print(f"{C.GREEN}✓{C.RESET} {msg}")
    else:
        print(f"OK: {msg}")


def err(msg: str):
    if _color_enabled():
        print(f"{C.RED}✗{C.RESET} {msg}", file=sys.stderr)
    else:
        print(f"ERROR: {msg}", file=sys.stderr)


def info(msg: str):
    if _color_enabled():
        print(f"{C.BLUE}→{C.RESET} {msg}")
    else:
        print(msg)


# --- Config ---

def find_config() -> Path:
    path = Path.cwd()
    while path != path.parent:
        candidate = path / CONFIG_FILE
        if candidate.exists():
            return candidate
        path = path.parent
    raise SystemExit(f"No {CONFIG_FILE} found. Run: kint-vault init")


def load_config(env_override: str = None) -> dict:
    config_path = find_config()
    with open(config_path) as f:
        config = yaml.safe_load(f)
    if env_override:
        config["env"] = env_override
    return config


# --- Key Name Conversion ---

def _env_to_az(key: str) -> str:
    return key.replace("_", "-").lower()


def _az_to_env(name: str) -> str:
    return name.replace("-", "_").upper()


# --- Backend ---

def run_cmd(cmd: list[str], capture: bool = True) -> str:
    try:
        result = subprocess.run(cmd, capture_output=capture, text=True, check=True)
        return result.stdout.strip() if capture else ""
    except FileNotFoundError:
        raise SystemExit(f"Command not found: {cmd[0]}. Is it installed?")
    except subprocess.CalledProcessError as e:
        raise SystemExit(f"Command failed: {e.stderr.strip() or e.stdout.strip()}")


def _vault_name(config: dict) -> str:
    env = config.get("env", "dev")
    return f"{config['vault']}-{env}"


def az_pull(config: dict) -> str:
    vault = _vault_name(config)
    names_raw = run_cmd([
        "az", "keyvault", "secret", "list",
        "--vault-name", vault,
        "--query", "[].name", "-o", "tsv",
    ])
    if not names_raw:
        return ""
    lines = []
    for az_name in sorted(names_raw.splitlines()):
        value = run_cmd([
            "az", "keyvault", "secret", "show",
            "--vault-name", vault,
            "--name", az_name,
            "--query", "value", "-o", "tsv",
        ])
        env_key = _az_to_env(az_name)
        lines.append(f"{env_key}={value}")
    return "\n".join(lines)


def az_push(config: dict, env_file: str):
    vault = _vault_name(config)
    secrets = _parse_env(Path(env_file).read_text())
    for key, value in secrets.items():
        az_name = _env_to_az(key)
        run_cmd([
            "az", "keyvault", "secret", "set",
            "--vault-name", vault,
            "--name", az_name,
            "--value", value,
            "--output", "none",
        ])


def az_run(config: dict, command: list[str]):
    secrets_str = az_pull(config)
    env = os.environ.copy()
    env.update(_parse_env(secrets_str))
    result = subprocess.run(command, env=env)
    raise SystemExit(result.returncode)


def az_set(config: dict, key: str, value: str):
    vault = _vault_name(config)
    az_name = _env_to_az(key)
    run_cmd([
        "az", "keyvault", "secret", "set",
        "--vault-name", vault,
        "--name", az_name,
        "--value", value,
        "--output", "none",
    ])


def az_get(config: dict, key: str) -> str:
    vault = _vault_name(config)
    az_name = _env_to_az(key)
    return run_cmd([
        "az", "keyvault", "secret", "show",
        "--vault-name", vault,
        "--name", az_name,
        "--query", "value", "-o", "tsv",
    ])


def az_list_keys(config: dict) -> str:
    vault = _vault_name(config)
    names_raw = run_cmd([
        "az", "keyvault", "secret", "list",
        "--vault-name", vault,
        "--query", "[].name", "-o", "tsv",
    ])
    if not names_raw:
        return ""
    return "\n".join(sorted(_az_to_env(n) for n in names_raw.splitlines()))


def az_diff(config: dict) -> str:
    remote = az_pull(config)
    local_path = Path(".env")
    if not local_path.exists():
        return "No local .env file to diff against"
    local_dict = _parse_env(local_path.read_text())
    remote_dict = _parse_env(remote)
    return _format_diff(local_dict, remote_dict)


def az_doctor(config: dict) -> list[tuple[str, bool, str]]:
    checks = []
    try:
        run_cmd(["az", "version"])
        checks.append(("Azure CLI installed", True, ""))
    except SystemExit:
        checks.append(("Azure CLI installed", False, "Install: brew install azure-cli"))
        return checks
    try:
        run_cmd(["az", "account", "show"])
        checks.append(("Authenticated", True, ""))
    except SystemExit:
        checks.append(("Authenticated", False, "Run: az login"))
        return checks
    vault = _vault_name(config)
    try:
        vault_info = run_cmd([
            "az", "keyvault", "show",
            "--name", vault,
            "--query", "properties.enableRbacAuthorization", "-o", "tsv",
        ])
        if vault_info.lower() == "true":
            checks.append(("RBAC enabled", True, vault))
        else:
            checks.append(("RBAC enabled", False, f"Run: az keyvault update --name {vault} --enable-rbac-authorization true"))
    except SystemExit:
        checks.append(("Vault exists", False, f"Vault '{vault}' not found"))
        return checks
    try:
        run_cmd(["az", "keyvault", "secret", "list", "--vault-name", vault, "--maxresults", "1"])
        checks.append(("Secrets readable", True, ""))
    except SystemExit:
        checks.append(("Secrets readable", False, "Need role: Key Vault Secrets User or Officer"))
    return checks


# --- Helpers ---

def _parse_env(content: str) -> dict[str, str]:
    result = {}
    for line in content.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            continue
        key, value = line.split("=", 1)
        result[key.strip()] = value.strip().strip("\"'")
    return result


def _format_diff(local: dict, remote: dict) -> str:
    lines = []
    all_keys = sorted(set(local) | set(remote))
    for key in all_keys:
        if key in local and key not in remote:
            lines.append(f"  - {key} (local only)")
        elif key not in local and key in remote:
            lines.append(f"  + {key} (remote only)")
        elif local[key] != remote[key]:
            lines.append(f"  ~ {key} (modified)")
    return "\n".join(lines) if lines else "No differences"


# --- Commands ---

def cmd_init(args):
    config_path = Path(CONFIG_FILE)
    if config_path.exists() and not args.force:
        raise SystemExit(f"{CONFIG_FILE} already exists. Use --force to overwrite")

    vault = args.vault or _prompt("Vault name prefix (e.g. kv-myapp)")
    env = args.env or _prompt("Default environment", "dev")

    config = {"backend": "azure", "vault": vault, "env": env}
    with open(config_path, "w") as f:
        yaml.dump(config, f, default_flow_style=False)

    gitignore = Path(".gitignore")
    if gitignore.exists():
        content = gitignore.read_text()
        if not any(line.strip() == ".env" for line in content.splitlines()):
            with open(gitignore, "a") as f:
                f.write("\n.env\n")
            info("Added .env to .gitignore")
    else:
        gitignore.write_text(".env\n")
        info("Created .gitignore with .env")

    ok(f"Initialized (azure/{vault}-{env})")


def cmd_pull(args):
    config = load_config(args.env)
    secrets = az_pull(config)

    if args.json:
        print(json.dumps(_parse_env(secrets), indent=2))
        return

    if args.stdout:
        print(secrets)
        return

    output = args.output or ".env"
    Path(output).write_text(secrets + "\n")
    ok(f"Pulled {len(_parse_env(secrets))} secrets → {output}")


def cmd_push(args):
    config = load_config(args.env)
    env_file = args.file or ".env"
    if not Path(env_file).exists():
        raise SystemExit(f"File not found: {env_file}")
    az_push(config, env_file)
    count = len(_parse_env(Path(env_file).read_text()))
    ok(f"Pushed {count} secrets → {_vault_name(config)}")


def cmd_run(args):
    config = load_config(args.env)
    az_run(config, args.command)


def cmd_set(args):
    config = load_config(args.env)
    for pair in args.pairs:
        if "=" not in pair:
            raise SystemExit(f"Invalid format: {pair}. Use KEY=VALUE")
        key, value = pair.split("=", 1)
        az_set(config, key, value)
        ok(f"Set {key}")


def cmd_get(args):
    config = load_config(args.env)
    print(az_get(config, args.key))


def cmd_list(args):
    config = load_config(args.env)
    keys = az_list_keys(config)
    if args.json:
        print(json.dumps(keys.splitlines(), indent=2))
    else:
        print(keys)


def cmd_diff(args):
    config = load_config(args.env)
    print(az_diff(config))


def cmd_validate(args):
    config = load_config(args.env)

    template = args.template or ENV_EXAMPLE
    if not Path(template).exists():
        raise SystemExit(f"Template not found: {template}. Create a {ENV_EXAMPLE} with required keys")

    required = set(_parse_env(Path(template).read_text()).keys())
    remote_keys = set(_parse_env(az_pull(config)).keys())

    missing = sorted(required - remote_keys)
    extra = sorted(remote_keys - required) if args.strict else []

    if not missing and not extra:
        ok(f"All {len(required)} keys present")
        return

    if missing:
        err(f"Missing {len(missing)} keys:")
        for k in missing:
            print(f"  - {k}")
    if extra:
        info(f"Extra {len(extra)} keys (not in template):")
        for k in extra:
            print(f"  + {k}")

    if missing:
        raise SystemExit(1)


def cmd_doctor(args):
    config = load_config(args.env)
    checks = az_doctor(config)
    checks.insert(0, (f"Config file ({CONFIG_FILE})", True, "azure"))

    all_ok = True
    for name, passed, detail in checks:
        status = f"{C.GREEN}✓{C.RESET}" if passed else f"{C.RED}✗{C.RESET}"
        if not _color_enabled():
            status = "OK" if passed else "FAIL"
        suffix = f" ({detail})" if detail else ""
        print(f"  {status} {name}{suffix}")
        if not passed:
            all_ok = False

    if not all_ok:
        raise SystemExit(1)


def cmd_env(args):
    config = load_config()
    if args.name:
        config_path = find_config()
        config["env"] = args.name
        with open(config_path, "w") as f:
            yaml.dump(config, f, default_flow_style=False)
        ok(f"Switched to environment: {args.name}")
    else:
        print(config.get("env", "dev"))


def _prompt(label: str, default: str = "") -> str:
    suffix = f" [{default}]" if default else ""
    value = input(f"{label}{suffix}: ").strip()
    if not value and default:
        return default
    if not value:
        raise SystemExit(f"{label} is required")
    return value


# --- Parser ---

def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="kint-vault", description="Unified secrets management CLI")
    parser.add_argument("--version", action="version", version="%(prog)s 0.1.0")
    sub = parser.add_subparsers(dest="command", metavar="<command>")

    p = sub.add_parser("init", help="Initialize project config")
    p.add_argument("--vault")
    p.add_argument("--env", default=None)
    p.add_argument("--force", action="store_true")

    p = sub.add_parser("pull", help="Pull secrets to .env")
    p.add_argument("--env")
    p.add_argument("--output", "-o", help="Output file (default: .env)")
    p.add_argument("--json", action="store_true", help="Output as JSON")
    p.add_argument("--stdout", action="store_true", help="Print to stdout only")

    p = sub.add_parser("push", help="Push .env to remote")
    p.add_argument("--env")
    p.add_argument("--file", "-f", default=None, help="File to push (default: .env)")

    p = sub.add_parser("run", help="Run command with injected secrets")
    p.add_argument("--env")
    p.add_argument("command", nargs=argparse.REMAINDER, metavar="-- command")

    p = sub.add_parser("set", help="Set secrets (KEY=VALUE ...)")
    p.add_argument("--env")
    p.add_argument("pairs", nargs="+", metavar="KEY=VALUE")

    p = sub.add_parser("get", help="Get a secret value")
    p.add_argument("--env")
    p.add_argument("key", metavar="KEY")

    p = sub.add_parser("list", help="List secret keys")
    p.add_argument("--env")
    p.add_argument("--json", action="store_true")

    p = sub.add_parser("diff", help="Show local vs remote differences")
    p.add_argument("--env")

    p = sub.add_parser("validate", help="Validate secrets against template")
    p.add_argument("--env")
    p.add_argument("--template", "-t", help=f"Template file (default: {ENV_EXAMPLE})")
    p.add_argument("--strict", action="store_true", help="Fail on extra keys too")

    p = sub.add_parser("doctor", help="Check connectivity and auth")
    p.add_argument("--env")

    p = sub.add_parser("env", help="Show or switch environment")
    p.add_argument("name", nargs="?", help="Environment to switch to")

    return parser


COMMANDS = {
    "init": cmd_init,
    "pull": cmd_pull,
    "push": cmd_push,
    "run": cmd_run,
    "set": cmd_set,
    "get": cmd_get,
    "list": cmd_list,
    "diff": cmd_diff,
    "validate": cmd_validate,
    "doctor": cmd_doctor,
    "env": cmd_env,
}


def main():
    parser = build_parser()
    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        raise SystemExit(1)

    COMMANDS[args.command](args)


if __name__ == "__main__":
    main()
