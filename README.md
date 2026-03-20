# kint-vault

Unified secrets management CLI powered by [SOPS](https://github.com/getsops/sops) + [age](https://github.com/FiloSottile/age).

Stop copying `.env` files manually. `kint-vault` encrypts secrets directly in your repo — no cloud service, no login, no infrastructure.

## Quick Start

```bash
pipx install git+https://github.com/kint-pro/kint-vault-cli.git
cd your-project
kint-vault pull                  # decrypt secrets → .env
```

That's it. If this is a new project, see [Setup a project](#setup-a-project) below.

## Prerequisites

### macOS

```bash
brew install sops age
```

### Linux (Debian/Ubuntu)

```bash
# age
sudo apt install age

# sops (not in apt — install .deb from GitHub)
curl -LO https://github.com/getsops/sops/releases/download/v3.12.2/sops_3.12.2_amd64.deb
sudo dpkg -i sops_3.12.2_amd64.deb
```

Check [SOPS releases](https://github.com/getsops/sops/releases) for the latest version.

### Windows

```powershell
winget install Mozilla.SOPS FiloSottile.age
```

Verify:

```bash
sops --version
age --version
```

## Installation

```bash
# pipx (recommended)
pipx install git+https://github.com/kint-pro/kint-vault-cli.git

# uv
uv tool install git+https://github.com/kint-pro/kint-vault-cli.git

# from source
git clone https://github.com/kint-pro/kint-vault-cli.git
cd kint-vault-cli
pip install .
```

Verify:

```bash
kint-vault --version
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
kint-vault init
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
kint-vault pull                    # decrypt → .env (fails if .env exists)
kint-vault pull --force            # overwrite existing .env
kint-vault pull -o .env.local      # custom output file
kint-vault pull --json             # JSON to stdout
kint-vault pull --env production   # decrypt .env.production.enc
```

### Push secrets

```bash
kint-vault push                    # encrypt .env (with confirmation)
kint-vault push -y                 # skip confirmation
kint-vault push -f .env.staging    # encrypt specific file
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
kint-vault delete API_KEY OLD_KEY
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
```

### Diff local vs encrypted

```bash
kint-vault diff
```

```
  + NEW_KEY (remote only)
  - OLD_KEY (local only)
  ~ API_KEY (modified)
```

### Switch environments

```bash
kint-vault env              # show current
kint-vault env production   # switch to production
```

Environments map to encrypted files: `dev` → `.env.dev.enc`, `production` → `.env.production.enc`.

### Rotate encryption key

```bash
kint-vault rotate
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
pipx install git+https://github.com/kint-pro/kint-vault-cli.git

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
