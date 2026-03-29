# kint-vault-cli

@SOUL.md

GitHub: kint-pro/kint-vault-cli

## Build & Test

```bash
make build                    # build binary with version injection
make test                     # go test ./...
go test ./internal/vault/...  # run single package tests
go vet ./...                  # lint
./test_integration.sh         # full integration test suite (needs built binary)
```

## Architecture

Single-binary Go CLI for encrypted secrets management using age encryption. No external tools required.

### Layer flow

```
CLI (cmd/kint-vault/main.go)  — flag parsing, command dispatch via switch
  → commands/ (internal/commands/)  — one CmdX() function per command, loads config, calls backend
    → backend/ (internal/backend/)  — orchestrates vault operations, no config knowledge
      → vault/ (internal/vault/)  — per-value AES-256-GCM encryption, age key wrapping, MAC integrity
```

Supporting packages:
- `config/` — loads `.kint-vault.yaml`, resolves age key paths, atomic file writes
- `envfile/` — parses/formats `.env` files, computes diffs
- `output/` — colored terminal output (`Ok`, `Err`, `Info`, `Warn`), respects `NO_COLOR`

### Encryption model

- Each secret value encrypted independently with AES-256-GCM (32-byte nonce)
- Random data key (AES-256) wrapped per-recipient using age X25519
- Format: `ENC[AES256_GCM,data:<b64>,iv:<b64>,tag:<b64>,type:str]`
- Metadata lines use `kv_` prefix (`kv_age_key`, `kv_age_recipient_N`, `kv_mac`, `kv_version`)
- HMAC-SHA256 MAC (encrypted) for tamper detection
- Nonce stashing: re-encryption reuses nonces for unchanged values (deterministic output)

### Config

`.kint-vault.yaml` stores `env` (default environment) and `recipients` (age public keys). Encrypted files are `.env.{env}.enc`. Config is searched upward from cwd.

### Key conventions

- Age key file paths follow SOPS conventions (`SOPS_AGE_KEY`, `SOPS_AGE_KEY_FILE`, `~/.config/sops/age/keys.txt`) for compatibility
- Reserved key prefix `kv_` — user secrets cannot use it
- Env names validated: `[a-zA-Z0-9_-]+`

### Monorepo

`--all` flag recursively discovers `.env.{env}.enc` files. Parallel execution via goroutine semaphore bounded by CPU cores (`parallel.go`).

### Error handling

Commands use `fatal()` (print + exit 1). All errors are explicit and immediately fatal.

## Release

Tag push triggers GoReleaser → multi-platform binaries + Homebrew tap update:

```bash
git tag v0.x.x && git push --tags
```
