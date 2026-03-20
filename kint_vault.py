#!/usr/bin/env python3
"""kint-vault: Unified secrets management CLI."""

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
from importlib.metadata import version
from pathlib import Path

import yaml

CONFIG_FILE = ".kint-vault.yaml"
SOPS_CONFIG = ".sops.yaml"
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


# --- Helpers ---

def run_cmd(cmd: list[str], capture: bool = True) -> str:
    try:
        result = subprocess.run(cmd, capture_output=capture, text=True, check=True)
        return result.stdout.strip() if capture else ""
    except FileNotFoundError:
        raise SystemExit(f"Command not found: {cmd[0]}. Is it installed?")
    except subprocess.CalledProcessError as e:
        raise SystemExit(f"Command failed: {cmd[0]} exited with code {e.returncode}")


def _restrict_file(path):
    if sys.platform == "win32":
        os.chmod(str(path), 0o444)
    else:
        os.chmod(str(path), 0o600)


def _validate_env_name(env: str):
    if not re.fullmatch(r'[a-zA-Z0-9_-]+', env):
        raise SystemExit(f"Invalid environment name: {env!r}")


def _enc_file(config: dict) -> str:
    env = config.get("env", "dev")
    _validate_env_name(env)
    return f".env.{env}.enc"


def _age_key_file() -> Path:
    env_path = os.environ.get("SOPS_AGE_KEY_FILE")
    if env_path:
        return Path(env_path)
    xdg = os.environ.get("XDG_CONFIG_HOME")
    if xdg:
        return Path(xdg) / "sops" / "age" / "keys.txt"
    if sys.platform == "win32":
        return Path(os.environ.get("APPDATA", Path.home() / "AppData" / "Roaming")) / "sops" / "age" / "keys.txt"
    if sys.platform == "darwin":
        return Path.home() / "Library" / "Application Support" / "sops" / "age" / "keys.txt"
    return Path.home() / ".config" / "sops" / "age" / "keys.txt"


def _read_age_pubkey(key_file: Path) -> str:
    for line in key_file.read_text().splitlines():
        if line.startswith("# public key:"):
            return line.split(":", 1)[1].strip()
    raise SystemExit(f"No public key found in {key_file}")


def _load_sops_config() -> dict:
    path = Path.cwd()
    while path != path.parent:
        candidate = path / SOPS_CONFIG
        if candidate.exists():
            with open(candidate) as f:
                return yaml.safe_load(f)
        path = path.parent
    raise SystemExit(f"No {SOPS_CONFIG} found. Run: kint-vault init")


def _save_sops_config(config: dict):
    path = Path.cwd()
    while path != path.parent:
        candidate = path / SOPS_CONFIG
        if candidate.exists():
            with tempfile.NamedTemporaryFile("w", dir=candidate.parent, suffix=".tmp", delete=False) as f:
                yaml.dump(config, f, default_flow_style=False)
                tmp = f.name
            os.replace(tmp, candidate)
            return
        path = path.parent
    raise SystemExit(f"No {SOPS_CONFIG} found")


def _get_recipients() -> list[str]:
    sops = _load_sops_config()
    for rule in sops.get("creation_rules", []):
        age = rule.get("age", "")
        if age:
            return [r.strip() for r in age.split(",") if r.strip()]
    return []


def _parse_env(content: str) -> dict[str, str]:
    result = {}
    for line in content.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            continue
        key, value = line.split("=", 1)
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in ('"', "'"):
            value = value[1:-1]
        result[key.strip()] = value
    return result


def _format_env(secrets: dict[str, str]) -> str:
    return "\n".join(f"{k}={v}" for k, v in sorted(secrets.items()))


def _find_all_enc_files() -> list[Path]:
    return sorted(Path.cwd().glob(".env.*.enc"))


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


def _print_checks(checks: list[tuple[str, bool, str]]):
    for name, passed, detail in checks:
        status = f"{C.GREEN}✓{C.RESET}" if passed else f"{C.RED}✗{C.RESET}"
        if not _color_enabled():
            status = "OK" if passed else "FAIL"
        suffix = f" ({detail})" if detail else ""
        print(f"  {status} {name}{suffix}")


def _install_hint(tool: str) -> str:
    if sys.platform == "darwin":
        return f"Install: brew install {tool}"
    if sys.platform == "win32":
        pkg = "Mozilla.SOPS" if tool == "sops" else "FiloSottile.age"
        return f"Install: winget install {pkg}"
    return f"Install: https://github.com/{'getsops/sops' if tool == 'sops' else 'FiloSottile/age'}/releases"


def _prompt(label: str, default: str = "") -> str:
    suffix = f" [{default}]" if default else ""
    value = input(f"{label}{suffix}: ").strip()
    if not value and default:
        return default
    if not value:
        raise SystemExit(f"{label} is required")
    return value


# --- SOPS Backend ---

def sops_decrypt(config: dict) -> str:
    enc = _enc_file(config)
    if not Path(enc).exists():
        raise SystemExit(f"No encrypted secrets: {enc}. Run: kint-vault push")
    try:
        return run_cmd([
            "sops", "decrypt", "--input-type", "dotenv", "--output-type", "dotenv", enc,
        ])
    except SystemExit:
        raise SystemExit(
            f"Decryption failed for {enc}. Your key may not be in recipients.\n"
            f"Run: kint-vault doctor"
        )


def sops_encrypt_file(config: dict, input_file: str):
    enc = _enc_file(config)
    try:
        encrypted = run_cmd([
            "sops", "encrypt", "--input-type", "dotenv", "--output-type", "dotenv", input_file,
        ])
    except SystemExit:
        raise SystemExit(
            f"Encryption failed. Is sops installed and .sops.yaml configured?\n"
            f"Run: kint-vault doctor"
        )
    Path(enc).write_text(encrypted + "\n")


def sops_encrypt_content(config: dict, content: str):
    enc = _enc_file(config)
    fd, tmp_path = tempfile.mkstemp(prefix=".env.", suffix=".tmp", dir=".")
    try:
        os.write(fd, (content + "\n").encode())
        os.close(fd)
        _restrict_file(tmp_path)
        try:
            encrypted = run_cmd([
                "sops", "encrypt",
                "--input-type", "dotenv", "--output-type", "dotenv",
                "--filename-override", ".env",
                tmp_path,
            ])
        except SystemExit:
            raise SystemExit(
                f"Encryption failed. Is sops installed and .sops.yaml configured?\n"
                f"Run: kint-vault doctor"
            )
        Path(enc).write_text(encrypted + "\n")
    finally:
        Path(tmp_path).unlink(missing_ok=True)


# --- Commands ---

def cmd_init(args):
    config_path = Path(CONFIG_FILE)
    if config_path.exists() and not args.force:
        raise SystemExit(f"{CONFIG_FILE} already exists. Use --force to overwrite")

    env = args.env or _prompt("Default environment", "dev")

    key_file = _age_key_file()
    if key_file.exists():
        pubkey = _read_age_pubkey(key_file)
        info(f"Using existing age key")
    else:
        try:
            key_file.parent.mkdir(parents=True, exist_ok=True)
            run_cmd(["age-keygen", "-o", str(key_file)])
        except SystemExit:
            raise SystemExit(
                f"Failed to generate age key. Is age installed?\n"
                f"Run: kint-vault doctor"
            )
        _restrict_file(key_file)
        pubkey = _read_age_pubkey(key_file)
        ok(f"Generated age key")

    config = {"backend": "sops", "env": env}
    with open(config_path, "w") as f:
        yaml.dump(config, f, default_flow_style=False)

    sops_path = Path(SOPS_CONFIG)
    if sops_path.exists():
        existing_sops = yaml.safe_load(sops_path.read_text()) or {}
        recipients = []
        for rule in existing_sops.get("creation_rules", []):
            age = rule.get("age", "")
            recipients = [r.strip() for r in age.split(",") if r.strip()]
            break
        if pubkey not in recipients:
            recipients.append(pubkey)
            for rule in existing_sops.get("creation_rules", []):
                rule["age"] = ",".join(recipients)
                break
            with open(sops_path, "w") as f:
                yaml.dump(existing_sops, f, default_flow_style=False)
            ok(f"Added your key to {SOPS_CONFIG}")
    else:
        sops_config = {
            "creation_rules": [
                {"path_regex": "\\.env", "age": pubkey}
            ]
        }
        with open(sops_path, "w") as f:
            yaml.dump(sops_config, f, default_flow_style=False)
        ok(f"Created {SOPS_CONFIG}")

    gitignore = Path(".gitignore")
    entries = [".env"]
    if gitignore.exists():
        content = gitignore.read_text()
        existing = {line.strip() for line in content.splitlines()}
        to_add = [e for e in entries if e not in existing]
        if to_add:
            with open(gitignore, "a") as f:
                f.write("\n" + "\n".join(to_add) + "\n")
            info(f"Added to .gitignore: {', '.join(to_add)}")
    else:
        gitignore.write_text("\n".join(entries) + "\n")
        info("Created .gitignore")

    ok(f"Initialized (sops+age, env: {env})")
    info(f"Your public key: {pubkey}")
    info("Share this key with your team to be added as recipient")


def cmd_pull(args):
    config = load_config(args.env)
    content = sops_decrypt(config)

    if args.json:
        print(json.dumps(_parse_env(content), indent=2))
        return

    if args.stdout:
        print(content)
        return

    output = args.output or ".env"
    output_path = Path(output).resolve()
    data = (content + "\n").encode()
    if args.force:
        fd = os.open(output_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    else:
        try:
            fd = os.open(output_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        except FileExistsError:
            raise SystemExit(f"{output} already exists. Use --force to overwrite")
    try:
        os.write(fd, data)
    finally:
        os.close(fd)
    ok(f"Decrypted {len(_parse_env(content))} secrets → {output}")


def cmd_push(args):
    config = load_config(args.env)
    env_file = args.file or ".env"
    if not Path(env_file).exists():
        raise SystemExit(f"File not found: {env_file}. Create a .env file with your secrets first.")
    secrets = _parse_env(Path(env_file).read_text())
    enc = _enc_file(config)
    is_new = not Path(enc).exists()
    if not args.yes:
        if is_new:
            info(f"Creating new environment: {config.get('env', 'dev')}")
        info(f"Will encrypt {len(secrets)} secrets → {enc}:")
        for key in sorted(secrets):
            print(f"  {key}")
        answer = input("Continue? [y/N] ").strip().lower()
        if answer != "y":
            raise SystemExit("Aborted")
    sops_encrypt_file(config, env_file)
    if is_new:
        ok(f"Created {enc} with {len(secrets)} secrets")
    else:
        ok(f"Encrypted {len(secrets)} secrets → {enc}")


def cmd_run(args):
    command = args.command
    if command and command[0] == "--":
        command = command[1:]
    if not command:
        raise SystemExit("No command specified. Usage: kint-vault run -- <command>")
    config = load_config(args.env)
    content = sops_decrypt(config)
    env = os.environ.copy()
    env.update(_parse_env(content))
    result = subprocess.run(command, env=env)
    raise SystemExit(result.returncode)


def cmd_set(args):
    config = load_config(args.env)
    enc = _enc_file(config)
    secrets = {}
    if Path(enc).exists():
        secrets = _parse_env(sops_decrypt(config))
    for pair in args.pairs:
        if "=" not in pair:
            raise SystemExit(f"Invalid format: {pair}. Use KEY=VALUE")
        key, value = pair.split("=", 1)
        secrets[key] = value
        ok(f"Set {key}")
    sops_encrypt_content(config, _format_env(secrets))


def cmd_get(args):
    config = load_config(args.env)
    secrets = _parse_env(sops_decrypt(config))
    if args.key not in secrets:
        raise SystemExit(f"Key not found: {args.key}")
    print(secrets[args.key])


def cmd_delete(args):
    config = load_config(args.env)
    secrets = _parse_env(sops_decrypt(config))
    if not args.yes:
        info(f"Will delete from {_enc_file(config)}:")
        for key in args.keys:
            print(f"  {key}")
        answer = input("Continue? [y/N] ").strip().lower()
        if answer != "y":
            raise SystemExit("Aborted")
    for key in args.keys:
        if key not in secrets:
            raise SystemExit(f"Key not found: {key}")
        del secrets[key]
        ok(f"Deleted {key}")
    sops_encrypt_content(config, _format_env(secrets))


def cmd_list(args):
    config = load_config(args.env)
    keys = sorted(_parse_env(sops_decrypt(config)).keys())
    if args.json:
        print(json.dumps(keys, indent=2))
    else:
        print("\n".join(keys))


def cmd_diff(args):
    config = load_config(args.env)
    local_path = Path(".env")
    if not local_path.exists():
        raise SystemExit("No local .env file to diff against")
    local_dict = _parse_env(local_path.read_text())
    remote_dict = _parse_env(sops_decrypt(config))
    print(_format_diff(local_dict, remote_dict))


def cmd_edit(args):
    config = load_config(args.env)
    enc = _enc_file(config)
    if not Path(enc).exists():
        raise SystemExit(f"No encrypted secrets: {enc}")
    result = subprocess.run(["sops", "edit", "--input-type", "dotenv", "--output-type", "dotenv", enc])
    if result.returncode != 0:
        raise SystemExit(f"Edit failed (exit {result.returncode})")
    ok(f"Saved {enc}")


def cmd_rotate(args):
    config = load_config(args.env)
    enc = _enc_file(config)
    if not Path(enc).exists():
        raise SystemExit(f"No encrypted secrets: {enc}")
    run_cmd(["sops", "rotate", "--input-type", "dotenv", "--output-type", "dotenv", "-i", enc])
    ok(f"Rotated data key for {enc}")


def cmd_add_recipient(args):
    pubkey = args.key
    if not pubkey.startswith("age1"):
        raise SystemExit("Invalid age public key (must start with age1)")

    sops = _load_sops_config()
    for rule in sops.get("creation_rules", []):
        existing = rule.get("age", "")
        recipients = [r.strip() for r in existing.split(",") if r.strip()]
        if pubkey in recipients:
            raise SystemExit("Recipient already exists")
        recipients.append(pubkey)
        rule["age"] = ",".join(recipients)
        break
    else:
        raise SystemExit("No creation_rules in .sops.yaml")

    _save_sops_config(sops)
    ok(f"Added recipient: {pubkey[:20]}...")

    for enc in _find_all_enc_files():
        run_cmd(["sops", "updatekeys", "--input-type", "dotenv", "-y", str(enc)])
        ok(f"Updated keys in {enc.name}")


def cmd_remove_recipient(args):
    pubkey = args.key

    sops = _load_sops_config()
    found = False
    for rule in sops.get("creation_rules", []):
        existing = rule.get("age", "")
        recipients = [r.strip() for r in existing.split(",") if r.strip()]
        if pubkey in recipients:
            recipients.remove(pubkey)
            if not recipients:
                raise SystemExit("Cannot remove last recipient")
            rule["age"] = ",".join(recipients)
            found = True
            break

    if not found:
        raise SystemExit("Recipient not found")

    _save_sops_config(sops)
    ok(f"Removed recipient: {pubkey[:20]}...")

    for enc in _find_all_enc_files():
        run_cmd(["sops", "updatekeys", "--input-type", "dotenv", "-y", str(enc)])
        run_cmd(["sops", "rotate", "--input-type", "dotenv", "--output-type", "dotenv", "-i", str(enc)])
        ok(f"Updated keys and rotated data key in {enc.name}")
    info("Removed recipients can still decrypt old versions from git history")


def cmd_validate(args):
    config = load_config(args.env)

    template = args.template or ENV_EXAMPLE
    if not Path(template).exists():
        raise SystemExit(f"Template not found: {template}. Create a {ENV_EXAMPLE} with required keys")

    required = set(_parse_env(Path(template).read_text()).keys())
    remote_keys = set(_parse_env(sops_decrypt(config)).keys())

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

    if missing or extra:
        raise SystemExit(1)


def cmd_doctor(args):
    config = load_config(args.env)
    checks = []

    checks.append((f"Config file ({CONFIG_FILE})", True, "sops"))

    try:
        sops_version = run_cmd(["sops", "--version"])
        checks.append(("sops installed", True, sops_version))
    except SystemExit:
        checks.append(("sops installed", False, _install_hint("sops")))
        _print_checks(checks)
        raise SystemExit(1)

    try:
        run_cmd(["age", "--version"])
        checks.append(("age installed", True, ""))
    except SystemExit:
        checks.append(("age installed", False, _install_hint("age")))
        _print_checks(checks)
        raise SystemExit(1)

    key_file = _age_key_file()
    if key_file.exists():
        pubkey = _read_age_pubkey(key_file)
        checks.append(("age key exists", True, f"{pubkey[:20]}..."))
    else:
        checks.append(("age key exists", False, "Run: kint-vault init"))
        _print_checks(checks)
        raise SystemExit(1)

    try:
        recipients = _get_recipients()
        checks.append((f"{SOPS_CONFIG} valid", True, f"{len(recipients)} recipient(s)"))
    except SystemExit:
        checks.append((f"{SOPS_CONFIG} valid", False, "Run: kint-vault init"))
        _print_checks(checks)
        raise SystemExit(1)

    if pubkey in recipients:
        checks.append(("Your key in recipients", True, ""))
    else:
        checks.append(("Your key in recipients", False, "Ask team to run: kint-vault add-recipient <your-key>"))

    enc = _enc_file(config)
    if Path(enc).exists():
        checks.append((f"Encrypted file ({enc})", True, ""))
        try:
            content = sops_decrypt(config)
            count = len(_parse_env(content))
            checks.append(("Decryption works", True, f"{count} secrets"))
        except SystemExit:
            checks.append(("Decryption works", False, "Your key may not be in recipients"))
    else:
        checks.append((f"Encrypted file ({enc})", False, "Run: kint-vault push"))

    _print_checks(checks)
    if not all(passed for _, passed, _ in checks):
        raise SystemExit(1)


def cmd_env(args):
    config_path = find_config()
    with open(config_path) as f:
        config = yaml.safe_load(f)
    if args.name:
        config["env"] = args.name
        with open(config_path, "w") as f:
            yaml.dump(config, f, default_flow_style=False)
        ok(f"Switched to environment: {args.name}")
    else:
        print(config.get("env", "dev"))


# --- Parser ---

def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="kint-vault", description="Unified secrets management CLI")
    parser.add_argument("--version", action="version", version=f"%(prog)s {version('kint-vault-cli')}")
    sub = parser.add_subparsers(dest="command", metavar="<command>")

    p = sub.add_parser("init", help="Initialize project config")
    p.add_argument("--env", default=None)
    p.add_argument("--force", action="store_true")

    p = sub.add_parser("pull", help="Decrypt secrets to .env")
    p.add_argument("--env")
    p.add_argument("--output", "-o", help="Output file (default: .env)")
    p.add_argument("--json", action="store_true", help="Output as JSON")
    p.add_argument("--stdout", action="store_true", help="Print to stdout only")
    p.add_argument("--force", action="store_true", help="Overwrite existing .env file")

    p = sub.add_parser("push", help="Encrypt .env to vault")
    p.add_argument("--env")
    p.add_argument("--file", "-f", default=None, help="File to encrypt (default: .env)")
    p.add_argument("--yes", "-y", action="store_true", help="Skip confirmation")

    p = sub.add_parser("run", help="Run command with injected secrets")
    p.add_argument("--env")
    p.add_argument("command", nargs=argparse.REMAINDER, metavar="-- command")

    p = sub.add_parser("set", help="Set secrets (KEY=VALUE ...)")
    p.add_argument("--env")
    p.add_argument("pairs", nargs="+", metavar="KEY=VALUE")

    p = sub.add_parser("get", help="Get a secret value")
    p.add_argument("--env")
    p.add_argument("key", metavar="KEY")

    p = sub.add_parser("delete", help="Delete secrets")
    p.add_argument("--env")
    p.add_argument("--yes", "-y", action="store_true", help="Skip confirmation")
    p.add_argument("keys", nargs="+", metavar="KEY")

    p = sub.add_parser("list", help="List secret keys")
    p.add_argument("--env")
    p.add_argument("--json", action="store_true")

    p = sub.add_parser("diff", help="Show local vs encrypted differences")
    p.add_argument("--env")

    p = sub.add_parser("edit", help="Edit encrypted secrets in $EDITOR")
    p.add_argument("--env")

    p = sub.add_parser("rotate", help="Rotate data encryption key")
    p.add_argument("--env")

    p = sub.add_parser("add-recipient", help="Add age public key to recipients")
    p.add_argument("--env")
    p.add_argument("key", metavar="AGE_PUBLIC_KEY")

    p = sub.add_parser("remove-recipient", help="Remove age public key from recipients")
    p.add_argument("--env")
    p.add_argument("key", metavar="AGE_PUBLIC_KEY")

    p = sub.add_parser("validate", help="Validate secrets against template")
    p.add_argument("--env")
    p.add_argument("--template", "-t", help=f"Template file (default: {ENV_EXAMPLE})")
    p.add_argument("--strict", action="store_true", help="Fail on extra keys too")

    p = sub.add_parser("doctor", help="Check setup and connectivity")
    p.add_argument("--env")

    p = sub.add_parser("env", help="Show or switch environment")
    p.add_argument("name", nargs="?", help="Environment to switch to")

    return parser


ALIAS_HINT = """
Tip: Create a short alias "kv" for kint-vault:

  macOS/Linux (zsh):  echo 'alias kv="kint-vault"' >> ~/.zshrc && source ~/.zshrc
  macOS/Linux (bash): echo 'alias kv="kint-vault"' >> ~/.bashrc && source ~/.bashrc
  Windows (PS):       Set-Alias -Name kv -Value kint-vault -Scope Global
"""


COMMANDS = {
    "init": cmd_init,
    "pull": cmd_pull,
    "push": cmd_push,
    "run": cmd_run,
    "set": cmd_set,
    "get": cmd_get,
    "delete": cmd_delete,
    "list": cmd_list,
    "diff": cmd_diff,
    "edit": cmd_edit,
    "rotate": cmd_rotate,
    "add-recipient": cmd_add_recipient,
    "remove-recipient": cmd_remove_recipient,
    "validate": cmd_validate,
    "doctor": cmd_doctor,
    "env": cmd_env,
}


def main():
    parser = build_parser()
    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        print(ALIAS_HINT)
        raise SystemExit(1)

    COMMANDS[args.command](args)


if __name__ == "__main__":
    main()
