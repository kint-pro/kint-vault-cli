# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

kint-vault is a Go CLI for secrets management powered by native AES-256-GCM encryption + age key wrapping. Single static binary, zero external dependencies. Encrypts `.env` files directly in the repo.

## Development Commands

```bash
# Build
make build
# or
go build -o kint-vault ./cmd/kint-vault/

# Run unit tests
go test ./...

# Run integration tests (requires age key)
bash test_integration.sh

# Run the CLI
go run ./cmd/kint-vault/ --help
```

## Architecture

Entry point `cmd/kint-vault/main.go` parses flags and dispatches to `internal/commands/`.

**Packages:**
- **`internal/vault`** — Crypto core. Per-value AES-256-GCM with AAD (key name), 32-byte nonces, IV-stash for stable re-encryption, HMAC-SHA256 MAC (length-prefixed, encrypted)
- **`internal/sopsbackend`** — Thin adapter between commands and vault. Handles decrypt-for-update (data key reuse) vs fresh encrypt
- **`internal/commands`** — One file per subcommand. Uses `fatal()` for errors (fail fast, `os.Exit(1)`)
- **`internal/config`** — Config loading (`.kint-vault.yaml`, `.sops.yaml`), age key file discovery, atomic writes
- **`internal/envfile`** — `.env` parsing/formatting, diff computation
- **`internal/output`** — Colored terminal output respecting `NO_COLOR`

**Data key lifecycle:**
- `push` / `rotate` / `remove-recipient` → new data key (fresh `WriteEncryptedFile`)
- `set` / `delete` / `edit` → reuse existing data key + IV-stash (`DecryptForUpdate` → `WriteEncryptedFileWithKey`). Only changed values get new ciphertext

**File format (`kv_` prefix):**
```
KEY=ENC[AES256_GCM,data:<b64>,iv:<b64>,tag:<b64>,type:str]
kv_age_recipient_0=age1...
kv_age_key=<age-armored wrapped data key>
kv_mac=ENC[AES256_GCM,...]
kv_version=1
```

## Conventions

- Errors: `fatal()` calls `os.Exit(1)` — fail fast, no error returns in commands
- File writes: `config.AtomicWrite` (temp + `os.Rename`) prevents corruption
- `.env` files created with `0o600` permissions
- Monorepo `--all` uses `filepath.Walk` with exclusions (dotdirs, `node_modules`, `__pycache__`, `.venv`)
- Config lookup walks parent directories
- Reserved `kv_` prefix rejected on `set` and `push`
