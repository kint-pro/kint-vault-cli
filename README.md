# kint-vault

Unified secrets management CLI for Azure Key Vault.

Stop copying `.env` files manually. `kint-vault` wraps [Azure Key Vault](https://azure.microsoft.com/en-us/products/key-vault) behind a simple CLI so your team can pull, push, and manage secrets with one command.

## Quick Start

```bash
pipx install git+https://github.com/kint-org/kint-vault-cli.git

cd your-project
kint-vault init              # set vault name and environment
kint-vault pull              # secrets -> .env
kint-vault run -- npm start  # inject secrets without .env file
```

## Installation

```bash
# pipx (recommended)
pipx install git+https://github.com/kint-org/kint-vault-cli.git

# uv
uv tool install git+https://github.com/kint-org/kint-vault-cli.git

# from source
git clone https://github.com/kint-org/kint-vault-cli.git
cd kint-vault-cli
pip install .
```

Requires Azure CLI:

```bash
brew install azure-cli
az login
```

## Commands

| Command | Description |
|---------|-------------|
| `kint-vault init` | Initialize project config, create `.kint-vault.yaml` |
| `kint-vault pull` | Pull secrets from Key Vault to `.env` |
| `kint-vault push` | Push local `.env` to Key Vault |
| `kint-vault run -- <cmd>` | Run a command with secrets injected as env vars |
| `kint-vault set KEY=VALUE` | Set one or more secrets |
| `kint-vault get KEY` | Get a single secret value |
| `kint-vault list` | List all secret keys |
| `kint-vault diff` | Show differences between local `.env` and remote |
| `kint-vault validate` | Check secrets against `.env.example` template |
| `kint-vault doctor` | Verify CLI installation, auth, and vault access |
| `kint-vault env [name]` | Show or switch the active environment |

## Usage

### Setup a project

```bash
kint-vault init --vault kv-myapp --env dev
```

This creates `.kint-vault.yaml` (commit this to git) and adds `.env` to `.gitignore`.

The vault name convention is `{prefix}-{env}`, so `--vault kv-myapp --env dev` targets the vault `kv-myapp-dev`.

### Pull secrets

```bash
kint-vault pull                    # -> .env
kint-vault pull -o .env.local      # custom output file
kint-vault pull --json             # JSON to stdout
kint-vault pull --env production   # pull from kv-myapp-production
```

### Push secrets

```bash
kint-vault push                    # push .env
kint-vault push -f .env.staging    # push specific file
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
kint-vault list
kint-vault list --json
```

### Validate secrets

Create a `.env.example` with required keys (values can be empty):

```
API_KEY=
DATABASE_URL=
REDIS_URL=
```

Then validate:

```bash
kint-vault validate                   # check all keys exist in remote
kint-vault validate --strict          # also flag extra keys not in template
kint-vault validate -t .env.required  # custom template file
```

### Diff local vs remote

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

### Diagnose issues

```bash
kint-vault doctor
```

```
  ✓ Config file (.kint-vault.yaml) (azure)
  ✓ Azure CLI installed
  ✓ Authenticated
  ✓ Vault accessible (kv-myapp-dev)
```

## Configuration

`.kint-vault.yaml` lives in your project root and should be committed to git:

```yaml
backend: azure
vault: kv-myapp
env: dev
```

| Field | Description |
|-------|-------------|
| `backend` | `azure` |
| `vault` | Key Vault name prefix (env is appended: `kv-myapp-dev`) |
| `env` | Default environment (dev, staging, production) |

Every command accepts `--env` to override the default environment for that invocation.

## Key Name Conversion

Azure Key Vault does not allow underscores in secret names. `kint-vault` automatically converts between `.env` format and Azure format:

| `.env` (local) | Azure Key Vault |
|-----------------|-----------------|
| `DATABASE_URL` | `database-url` |
| `API_KEY` | `api-key` |
| `REDIS_HOST` | `redis-host` |

This conversion is transparent — you always use `UPPER_SNAKE_CASE` in your `.env` files and CLI commands.

## Team Onboarding

```bash
# New developer joins:
git clone your-project && cd your-project
pipx install git+https://github.com/kint-org/kint-vault-cli.git  # one-time
az login                                                           # one-time
kint-vault pull                                                    # secrets ready
```

## License

Copyright (c) 2026 Kint. All rights reserved.
