# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

kint-vault is a single-file Python CLI for secrets management powered by SOPS + age. It encrypts `.env` files directly in the repo — no cloud service or infrastructure needed. The entire implementation lives in `kint_vault.py`.

## Development Commands

```bash
# Install in dev mode (with test dependencies)
pip install -e ".[dev]"

# Run all tests
python3 -m pytest

# Run only unit tests (mocked, no sops/age needed)
python3 -m pytest test_kint_vault.py

# Run only integration tests (requires sops + age on PATH)
python3 -m pytest test_integration.py

# Run a single test
python3 -m pytest test_kint_vault.py::TestParseEnvParametrized::test_parse_env

# Run the CLI directly
python3 -m kint_vault --help
```

## Architecture

Single-module CLI (`kint_vault.py`) with no internal packages. Entry point is `main()` which dispatches to `cmd_*` functions via `COMMANDS` dict and `argparse`.

**Key layers in `kint_vault.py`:**
- **Output helpers** (`ok`, `err`, `warn`, `info`): Colored terminal output respecting `NO_COLOR`
- **Config** (`find_config`, `load_config`): Walks up directory tree to find `.kint-vault.yaml`
- **Helpers**: `.env` parsing (`_parse_env`), diff formatting, path exclusion for monorepo traversal
- **SOPS backend** (`sops_decrypt`, `sops_encrypt_file`, `sops_encrypt_content`): Shells out to `sops` CLI
- **Commands** (`cmd_init`, `cmd_pull`, `cmd_push`, etc.): Each subcommand is a standalone function taking parsed args

**External dependencies:** Only `pyyaml`. All crypto is delegated to `sops` and `age` binaries.

**Test structure:**
- `test_kint_vault.py` — Unit tests with mocks and Hypothesis property-based tests. No external tools needed.
- `test_integration.py` — End-to-end tests using real `sops`/`age` encryption. Uses a `project` fixture that creates a fresh encrypted project in `tmp_path`.

## Conventions

- All errors use `raise SystemExit(...)` — fail fast, no exception hierarchies
- File writes use `_atomic_write` (write to temp + `os.replace`) to prevent corruption
- `.env` files are created with `0o600` permissions via `os.open` flags
- Monorepo commands (`--all`) find files via `rglob` with `_is_excluded_path` filtering out dotdirs, `node_modules`, `__pycache__`, `.venv`
- Config lookup walks parent directories (like git does)
