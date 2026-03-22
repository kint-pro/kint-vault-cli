# kint-vault

Unified secrets management CLI powered by [SOPS](https://github.com/getsops/sops) + [age](https://github.com/FiloSottile/age).

Stop copying `.env` files manually. `kint-vault` encrypts secrets directly in your repo — no cloud service, no login, no infrastructure.

## Quick Start

```bash
# download binary (see Installation below)
cd your-project
kint-vault pull                  # decrypt secrets → .env
```

That's it. If this is a new project, see [Setup a project](#setup-a-project) below.

## Installation

Single binary, no dependencies (sops and age are still required on PATH):

```bash
# from source (requires Go 1.21+)
git clone https://github.com/kint-pro/kint-vault-cli.git
cd kint-vault-cli
go build -o kint-vault ./cmd/kint-vault/
sudo mv kint-vault /usr/local/bin/

# or install directly
go install github.com/kint-pro/kint-vault-cli/cmd/kint-vault@latest
```

### Prerequisites

sops and age must be installed separately:

**macOS:**
```bash
brew install sops age
```

**Linux (Debian/Ubuntu):**
```bash
sudo apt install age
curl -LO https://github.com/getsops/sops/releases/download/v3.12.2/sops_3.12.2_amd64.deb
sudo dpkg -i sops_3.12.2_amd64.deb
```

Check [SOPS releases](https://github.com/getsops/sops/releases) for the latest version.

**Windows:**
```powershell
winget install Mozilla.SOPS FiloSottile.age
```

Verify:

```bash
kint-vault --version
sops --version
age --version
kint-vault doctor
```

## Commands

| Command | Description |
|---------|-------------|
| `kint-vault init` | Initialize project config, generate age key |
| `kint-vault pull` | Decrypt secrets to `.env` |
| `kint-vault push` | Encrypt `.env` to vault |
| `kint-vault run -- <cmd>` | Run a command with secrets injected as env vars |
| `kint-vault set KEY=VALUE` | Set one or more secrets |
| `kint-vault get KEY` | Get a single secret value |
| `kint-vault delete KEY` | Delete one or more secrets |
| `kint-vault list` | List all secret keys |
| `kint-vault diff` | Show differences between local `.env` and encrypted vault |
| `kint-vault edit` | Edit encrypted secrets in `$EDITOR` |
| `kint-vault rotate` | Rotate data encryption key |
| `kint-vault add-recipient` | Add a team member's age public key |
| `kint-vault remove-recipient` | Remove a team member's age public key |
| `kint-vault validate` | Check secrets against `.env.example` template |
| `kint-vault doctor` | Verify installation and setup |
| `kint-vault env [name]` | Show or switch the active environment |

## Usage

### Setup a project

```bash
kint-vault init                    # interactive setup
kint-vault init --env production   # set default environment
kint-vault init --force            # overwrite existing config
```

This creates:
- `.kint-vault.yaml` — project config (commit to git)
- `.sops.yaml` — SOPS encryption config with your public key (commit to git)
- Age key pair (never commit):
  - macOS: `~/Library/Application Support/sops/age/keys.txt`
  - Linux: `~/.config/sops/age/keys.txt`
  - Windows: `%AppData%\sops\age\keys.txt`

### Pull secrets

```bash
kint-vault pull                    # decrypt → .env (shows diff + confirms if .env exists)
kint-vault pull --force            # overwrite without confirmation
kint-vault pull --all              # decrypt all services (monorepo)
kint-vault pull -o .env.local      # custom output file
kint-vault pull --json             # JSON to stdout
kint-vault pull --stdout           # plain text to stdout
kint-vault pull --env production   # decrypt .env.production.enc
```

If `.env` already exists, `pull` shows a colored diff and asks before overwriting. On overwrite, the previous values are printed so you can copy them if needed.

### Push secrets

```bash
kint-vault push                    # encrypt .env (with confirmation)
kint-vault push -y                 # skip confirmation
kint-vault push -f .env.staging    # encrypt specific file
kint-vault push --all              # encrypt all .env files (monorepo)
```

### Run with secrets

```bash
kint-vault run -- python train.py
kint-vault run --env production -- npm start
```

### Manage secrets

```bash
kint-vault set API_KEY=sk-123 DB_HOST=localhost
kint-vault get API_KEY
kint-vault delete API_KEY OLD_KEY     # confirms before deleting
kint-vault delete API_KEY -y          # skip confirmation
kint-vault list
kint-vault list --json
```

### Edit encrypted secrets directly

```bash
kint-vault edit
```

SOPS decrypts → opens `$EDITOR` → re-encrypts on save. The decrypted content never touches disk.

### Validate secrets

Create a `.env.example` with required keys (values can be empty):

```
API_KEY=
DATABASE_URL=
REDIS_URL=
```

Then validate:

```bash
kint-vault validate                   # check all keys exist
kint-vault validate --strict          # also flag extra keys not in template
kint-vault validate -t .env.required  # custom template file
kint-vault validate --json            # machine-readable output
kint-vault validate --all             # all services (monorepo)
```

### Diff local vs encrypted

```bash
kint-vault diff                    # single project
kint-vault diff --json             # machine-readable output
kint-vault diff --all              # all services (monorepo)
```

```
  + NEW_KEY=value (remote only)
  - OLD_KEY=value (local only)
  ~ API_KEY: old_value → new_value
```

Output is colored in terminals (`+` green, `-` red, `~` yellow). Long values are truncated.

### Switch environments

```bash
kint-vault env              # show current
kint-vault env production   # switch to production
```

Environments map to encrypted files: `dev` → `.env.dev.enc`, `production` → `.env.production.enc`.

### Rotate encryption key

```bash
kint-vault rotate                  # single project
kint-vault rotate --all            # all services (monorepo)
```

Generates a new data encryption key and re-encrypts all values. Use after removing a team member.

### Diagnose issues

```bash
kint-vault doctor
```

```
  ✓ Config file (.kint-vault.yaml) (sops)
  ✓ sops installed (sops 3.12.2)
  ✓ age installed
  ✓ age key exists (age1abc123...)
  ✓ .sops.yaml valid (3 recipients)
  ✓ Your key in recipients
  ✓ Encrypted file (.env.dev.enc)
  ✓ Decryption works (5 secrets)
```

## Team Onboarding

### New developer joins

```bash
# 1. Install prerequisites (see above) and kint-vault
go install github.com/kint-pro/kint-vault-cli/cmd/kint-vault@latest

# 2. Clone project and initialize
git clone <your-project> && cd <your-project>
kint-vault init

# 3. Share public key with team lead
# Output from init: "Your public key: age1abc123..."
```

### Team lead adds the new developer

```bash
kint-vault add-recipient age1abc123...
git add .sops.yaml .env.*.enc
git commit -m "add developer to vault"
git push
```

### New developer can now decrypt

```bash
git pull
kint-vault pull
```

### Remove a developer

```bash
kint-vault remove-recipient age1abc123...
git add .sops.yaml .env.*.enc
git commit -m "remove developer from vault"
```

This updates the encryption keys AND rotates the data key. The removed developer can still decrypt old versions from git history — rotate actual secret values if needed.

## Monorepo

Commands with `--all` find all `.env.{env}.enc` files recursively and operate on each service independently:

```bash
kint-vault pull --all              # decrypt all services
kint-vault push --all              # encrypt all services
kint-vault diff --all              # diff all services
kint-vault validate --all          # validate all services
kint-vault rotate --all            # rotate keys for all services
kint-vault list --all              # list keys for all services
```

## CI/CD

### Setup

```bash
# generate a CI-only age key
age-keygen -o ci-key.txt
# note the public key from output: age1abc...

# add as recipient
kint-vault add-recipient age1abc...
git add .sops.yaml .env.*.enc
git commit -m "add CI key" && git push
```

Store the private key from `ci-key.txt` as a GitHub Secret named `SOPS_AGE_KEY`.

### GitHub Actions

```yaml
- name: Decrypt secrets
  env:
    SOPS_AGE_KEY: ${{ secrets.SOPS_AGE_KEY }}
  run: kint-vault pull --force
```

Or inject secrets without writing to disk:

```yaml
- name: Run tests with secrets
  env:
    SOPS_AGE_KEY: ${{ secrets.SOPS_AGE_KEY }}
  run: kint-vault run -- npm test
```

All commands work with `SOPS_AGE_KEY` — no key file needed. Use `--force`, `-y`, and `--json` flags to avoid interactive prompts.

## Configuration

### `.kint-vault.yaml`

Lives in your project root. Commit to git.

```yaml
backend: sops
env: dev
```

| Field | Description |
|-------|-------------|
| `backend` | `sops` |
| `env` | Default environment (dev, staging, production) |

Every command accepts `--env` to override the default environment.

### Environment variables

| Variable | Description |
|----------|-------------|
| `SOPS_AGE_KEY` | Age private key content (for CI/CD, instead of key file) |
| `SOPS_AGE_KEY_FILE` | Override age key file location (default: `~/.config/sops/age/keys.txt`) |
| `NO_COLOR` | Disable colored output |

### Alias

After `kint-vault init` a shell alias is suggested:

```bash
alias kv="kint-vault"
```

### `.sops.yaml`

Managed by `kint-vault`. Commit to git.

```yaml
creation_rules:
  - path_regex: \.env
    age: >-
      age1abc...,
      age1def...,
      age1ghi...
```

### Encrypted files

Encrypted files (`.env.dev.enc`, `.env.production.enc`) are committed to git. They contain keys in cleartext and values encrypted with AES-256-GCM:

```
API_KEY=ENC[AES256_GCM,data:abc...,iv:...,tag:...,type:str]
DB_HOST=ENC[AES256_GCM,data:xyz...,iv:...,tag:...,type:str]
```

## How It Works

1. Each developer has an **age key pair** (private key stays local, public key shared)
2. SOPS generates a random **data key** (AES-256) to encrypt secret values
3. The data key is encrypted with **every recipient's public key** and stored in the file
4. Any recipient can decrypt the data key with their private key → decrypt the values
5. Adding/removing recipients only changes who can decrypt the data key, not the values themselves

## License

Copyright (c) 2026 Kint. All rights reserved.
