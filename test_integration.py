#!/usr/bin/env python3
"""End-to-end integration tests for kint-vault CLI.

Requires: sops, age installed and on PATH.
Runs real encryption/decryption — no mocks.

Structure:
  - Smoke tests (happy path)
  - Failure tests (expected errors)
  - Edge cases (special values, boundaries)
  - All features covered
"""

import json
import os
import subprocess
from pathlib import Path

import pytest


def kv(*args, env=None, input_text=None):
    """Run kint-vault command, return (stdout, stderr, exit_code)."""
    cmd_env = os.environ.copy()
    cmd_env["NO_COLOR"] = "1"
    if env:
        cmd_env.update(env)
    result = subprocess.run(
        ["python", "-m", "kint_vault", *args],
        capture_output=True, text=True, env=cmd_env,
        input=input_text,
    )
    return result.stdout, result.stderr, result.returncode


@pytest.fixture()
def project(tmp_path, monkeypatch):
    """Set up a fresh project with init + push for each test."""
    monkeypatch.chdir(tmp_path)

    out, err, code = kv("init", "--env", "dev", input_text="dev")
    assert code == 0, f"init failed: {err}"

    (tmp_path / ".env").write_text("API_KEY=sk-secret-123\nDB_HOST=localhost\nDB_PASS=p@ssw0rd\n")
    out, err, code = kv("push", "-y")
    assert code == 0, f"push failed: {err}"

    (tmp_path / ".env").unlink()
    return tmp_path


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  SMOKE TESTS — Happy Path
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━


class TestSmokeInit:
    def test_init_creates_config(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        out, err, code = kv("init", "--env", "dev", input_text="dev")
        assert code == 0
        assert (tmp_path / ".kint-vault.yaml").exists()
        assert (tmp_path / ".sops.yaml").exists()
        assert (tmp_path / ".gitignore").exists()

    def test_init_force_overwrites(self, project):
        out, err, code = kv("init", "--force", "--env", "dev", input_text="dev")
        assert code == 0


class TestSmokePull:
    def test_pull_decrypts(self, project):
        out, err, code = kv("pull")
        assert code == 0
        env = (project / ".env").read_text()
        assert "API_KEY=sk-secret-123" in env
        assert "DB_HOST=localhost" in env
        assert "DB_PASS=p@ssw0rd" in env

    def test_pull_permissions_600(self, project):
        kv("pull")
        mode = oct((project / ".env").stat().st_mode & 0o777)
        assert mode == "0o600"

    def test_pull_force(self, project):
        (project / ".env").write_text("OLD=x")
        out, err, code = kv("pull", "--force")
        assert code == 0
        assert "API_KEY=sk-secret-123" in (project / ".env").read_text()

    def test_pull_json(self, project):
        out, err, code = kv("pull", "--json")
        assert code == 0
        data = json.loads(out)
        assert data["API_KEY"] == "sk-secret-123"

    def test_pull_stdout(self, project):
        out, err, code = kv("pull", "--stdout")
        assert code == 0
        assert "API_KEY=sk-secret-123" in out

    def test_pull_custom_output(self, project):
        out, err, code = kv("pull", "-o", ".env.local")
        assert code == 0
        assert "API_KEY=sk-secret-123" in (project / ".env.local").read_text()


class TestSmokePush:
    def test_push_encrypts(self, project):
        (project / ".env").write_text("NEW=secret\n")
        out, err, code = kv("push", "-y")
        assert code == 0
        enc = (project / ".env.dev.enc").read_text()
        assert "ENC[" in enc

    def test_push_custom_file(self, project):
        (project / "custom.env").write_text("X=1\n")
        out, err, code = kv("push", "-f", "custom.env", "-y")
        assert code == 0


class TestSmokeCRUD:
    def test_get(self, project):
        out, err, code = kv("get", "API_KEY")
        assert code == 0
        assert out.strip() == "sk-secret-123"

    def test_set_and_get(self, project):
        kv("set", "NEW=hello")
        out, _, code = kv("get", "NEW")
        assert code == 0
        assert out.strip() == "hello"

    def test_set_multiple(self, project):
        kv("set", "A=1", "B=2", "C=3")
        for key, val in [("A", "1"), ("B", "2"), ("C", "3")]:
            out, _, _ = kv("get", key)
            assert out.strip() == val

    def test_set_overwrites(self, project):
        kv("set", "API_KEY=changed")
        out, _, _ = kv("get", "API_KEY")
        assert out.strip() == "changed"

    def test_delete(self, project):
        kv("delete", "API_KEY", "-y")
        out, err, code = kv("get", "API_KEY")
        assert code == 1

    def test_list(self, project):
        out, err, code = kv("list")
        assert code == 0
        keys = out.strip().splitlines()
        assert "API_KEY" in keys
        assert "DB_HOST" in keys
        assert "DB_PASS" in keys

    def test_list_json(self, project):
        out, err, code = kv("list", "--json")
        assert code == 0
        keys = json.loads(out)
        assert set(keys) == {"API_KEY", "DB_HOST", "DB_PASS"}


class TestSmokeDiff:
    def test_diff_no_differences(self, project):
        kv("pull")
        out, err, code = kv("diff")
        assert code == 0
        assert "No differences" in out

    def test_diff_shows_changes(self, project):
        (project / ".env").write_text("API_KEY=old\nLOCAL=x\n")
        out, err, code = kv("diff")
        assert code == 0
        assert "LOCAL" in out
        assert "DB_HOST" in out

    def test_diff_json(self, project):
        (project / ".env").write_text("API_KEY=old\n")
        out, err, code = kv("diff", "--json")
        assert code == 0
        data = json.loads(out)
        assert data["modified"]["API_KEY"]["local"] == "old"
        assert data["modified"]["API_KEY"]["remote"] == "sk-secret-123"
        assert "DB_HOST" in data["added"]


class TestSmokeValidate:
    def test_validate_pass(self, project):
        (project / ".env.example").write_text("API_KEY=\nDB_HOST=\nDB_PASS=\n")
        out, err, code = kv("validate")
        assert code == 0

    def test_validate_json_pass(self, project):
        (project / ".env.example").write_text("API_KEY=\n")
        out, err, code = kv("validate", "--json")
        assert code == 0
        data = json.loads(out)
        assert data["valid"] is True


class TestSmokeRun:
    def test_run_injects_secrets(self, project):
        out, err, code = kv("run", "--", "sh", "-c", "echo $API_KEY")
        assert code == 0
        assert "sk-secret-123" in out

    def test_run_exit_code_passthrough(self, project):
        _, _, code = kv("run", "--", "sh", "-c", "exit 42")
        assert code == 42


class TestSmokeEnv:
    def test_env_show(self, project):
        out, _, code = kv("env")
        assert code == 0
        assert out.strip() == "dev"

    def test_env_switch(self, project):
        kv("env", "staging")
        out, _, _ = kv("env")
        assert out.strip() == "staging"


class TestSmokeRotate:
    def test_rotate_preserves_secrets(self, project):
        out, err, code = kv("rotate")
        assert code == 0
        out2, _, code2 = kv("get", "API_KEY")
        assert code2 == 0
        assert out2.strip() == "sk-secret-123"


class TestSmokeDoctor:
    def test_doctor_pass(self, project):
        out, err, code = kv("doctor")
        assert code == 0


class TestSmokeRecipient:
    def test_add_recipient_duplicate(self, project):
        key_file = Path.home() / ".config" / "sops" / "age" / "keys.txt"
        if not key_file.exists():
            pytest.skip("no age key file")
        prefix = "# public key: "
        pubkey = next(
            line[len(prefix):].strip()
            for line in key_file.read_text().splitlines()
            if line.startswith(prefix)
        )
        out, err, code = kv("add-recipient", pubkey)
        assert code == 1
        assert "already exists" in (out + err)


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  FAILURE TESTS — Expected Errors
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━


class TestFailInit:
    def test_init_double_without_force(self, project):
        out, err, code = kv("init", "--env", "dev", input_text="dev")
        assert code == 1
        assert "already exists" in (out + err)


class TestFailPull:
    def test_pull_nonexistent_env(self, project):
        out, err, code = kv("pull", "--env", "fantasy")
        assert code == 1
        assert "No encrypted secrets" in (out + err)

    def test_pull_output_dir_missing(self, project):
        out, err, code = kv("pull", "-o", "sub/dir/.env")
        assert code == 1
        assert "Directory does not exist" in (out + err)

    def test_pull_abort_on_no(self, project):
        (project / ".env").write_text("X=1")
        out, err, code = kv("pull", input_text="n")
        assert code == 1
        assert (project / ".env").read_text() == "X=1"


class TestFailPush:
    def test_push_nonexistent_file(self, project):
        out, err, code = kv("push", "-f", "nope.env", "-y")
        assert code == 1

    def test_push_aborts_without_yes(self, project):
        (project / ".env").write_text("X=1\n")
        out, err, code = kv("push", input_text="n")
        assert code == 1


class TestFailCRUD:
    def test_get_nonexistent(self, project):
        _, _, code = kv("get", "NOPE")
        assert code == 1

    def test_delete_nonexistent(self, project):
        _, _, code = kv("delete", "NOPE", "-y")
        assert code == 1

    def test_set_no_equals(self, project):
        _, _, code = kv("set", "INVALID")
        assert code == 1


class TestFailDiff:
    def test_diff_no_local_env(self, project):
        out, err, code = kv("diff")
        assert code == 1
        assert "No local .env" in (out + err)


class TestFailValidate:
    def test_validate_missing_template(self, project):
        _, _, code = kv("validate")
        assert code == 1

    def test_validate_missing_keys(self, project):
        (project / ".env.example").write_text("API_KEY=\nMISSING=\n")
        _, _, code = kv("validate")
        assert code == 1

    def test_validate_strict_extra_keys(self, project):
        (project / ".env.example").write_text("API_KEY=\n")
        _, _, code = kv("validate", "--strict")
        assert code == 1

    def test_validate_json_missing(self, project):
        (project / ".env.example").write_text("MISSING=\n")
        out, _, code = kv("validate", "--json")
        assert code == 1
        data = json.loads(out)
        assert data["valid"] is False
        assert "MISSING" in data["missing"]


class TestFailEnv:
    def test_env_invalid_name(self, project):
        _, _, code = kv("env", "../../etc")
        assert code == 1

    def test_env_injection(self, project):
        _, _, code = kv("env", "dev;rm -rf /")
        assert code == 1


class TestFailRecipient:
    def test_add_invalid_key(self, project):
        _, _, code = kv("add-recipient", "not-age-key")
        assert code == 1

    def test_remove_last_recipient(self, project):
        key_file = Path.home() / ".config" / "sops" / "age" / "keys.txt"
        if not key_file.exists():
            pytest.skip("no age key file")
        prefix = "# public key: "
        pubkey = next(
            line[len(prefix):].strip()
            for line in key_file.read_text().splitlines()
            if line.startswith(prefix)
        )
        out, err, code = kv("remove-recipient", pubkey)
        assert code == 1
        assert "last recipient" in (out + err).lower()


class TestFailRun:
    def test_run_no_command(self, project):
        _, _, code = kv("run")
        assert code == 1


class TestFailNoProject:
    def test_commands_without_init(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        for cmd in ["pull", "push -y", "list", "diff", "doctor"]:
            args = cmd.split()
            _, _, code = kv(*args)
            assert code == 1


# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#  EDGE CASES
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━


class TestEdgeValues:
    def test_long_value_roundtrip(self, project):
        long_val = "x" * 1000
        kv("set", f"LONG={long_val}")
        out, _, code = kv("get", "LONG")
        assert code == 0
        assert out.strip() == long_val

    def test_value_with_equals(self, project):
        kv("set", "URL=postgres://u:p@h:5432/db?ssl=true")
        out, _, _ = kv("get", "URL")
        assert out.strip() == "postgres://u:p@h:5432/db?ssl=true"

    def test_value_with_special_chars(self, project):
        kv("set", 'S=hello world & "quotes"')
        out, _, _ = kv("get", "S")
        assert 'hello world & "quotes"' in out

    def test_empty_value(self, project):
        kv("set", "EMPTY=")
        out, _, code = kv("get", "EMPTY")
        assert code == 0
        assert out.strip() == ""

    def test_jwt_like_value(self, project):
        jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123"
        kv("set", f"TOKEN={jwt}")
        out, _, _ = kv("get", "TOKEN")
        assert out.strip() == jwt


class TestEdgePull:
    def test_pull_existing_shows_previous_values(self, project):
        (project / ".env").write_text("OLD=secret123\nLOCAL=keep")
        out, err, code = kv("pull", input_text="y")
        assert code == 0
        assert "OLD=secret123" in out
        assert "LOCAL=keep" in out

    def test_pull_force_shows_diff(self, project):
        (project / ".env").write_text("API_KEY=old")
        out, err, code = kv("pull", "--force")
        assert code == 0
        assert "Overwriting" in out
        assert "API_KEY" in out

    def test_pull_no_diff_skips(self, project):
        kv("pull")
        out, _, code = kv("pull")
        assert code == 0
        assert "No differences" in out


class TestEdgeDiff:
    def test_diff_truncates_long_values(self, project):
        kv("set", f"LONG={'A' * 100}")
        (project / ".env").write_text("LONG=short")
        out, _, _ = kv("diff")
        assert "..." in out

    def test_diff_json_full_values(self, project):
        long_val = "A" * 100
        kv("set", f"LONG={long_val}")
        (project / ".env").write_text("LONG=short")
        out, _, _ = kv("diff", "--json")
        data = json.loads(out)
        assert data["modified"]["LONG"]["remote"] == long_val
        assert data["modified"]["LONG"]["local"] == "short"


class TestEdgePushPull:
    def test_push_pull_roundtrip(self, project):
        secrets = "A=1\nB=two\nC=three=3\n"
        (project / ".env").write_text(secrets)
        kv("push", "-y")
        (project / ".env").unlink()
        kv("pull")
        for line in ["A=1", "B=two", "C=three=3"]:
            assert line in (project / ".env").read_text()

    def test_pull_force_then_diff_clean(self, project):
        kv("pull", "--force")
        out, _, code = kv("diff")
        assert code == 0
        assert "No differences" in out


class TestMonorepo:
    """Tests for --all flag across a monorepo with multiple services."""

    @pytest.fixture()
    def monorepo(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)

        # init at root
        out, err, code = kv("init", "--env", "dev", input_text="dev")
        assert code == 0, f"init failed: {err}"

        # create two services with their own .env
        for svc in ["services/api", "services/worker"]:
            svc_dir = tmp_path / svc
            svc_dir.mkdir(parents=True)

        (tmp_path / "services/api/.env").write_text("API_KEY=api-secret\nPORT=3000\n")
        (tmp_path / "services/worker/.env").write_text("WORKER_KEY=worker-secret\nQUEUE=redis\n")

        # push all
        out, err, code = kv("push", "--all", "-y")
        assert code == 0, f"push --all failed: {err}"

        # remove .env files
        (tmp_path / "services/api/.env").unlink()
        (tmp_path / "services/worker/.env").unlink()

        return tmp_path

    def test_pull_all(self, monorepo):
        out, err, code = kv("pull", "--all", "--force")
        assert code == 0
        api_env = (monorepo / "services/api/.env").read_text()
        assert "API_KEY=api-secret" in api_env
        worker_env = (monorepo / "services/worker/.env").read_text()
        assert "WORKER_KEY=worker-secret" in worker_env

    def test_pull_all_shows_file_list(self, monorepo):
        out, err, code = kv("pull", "--all", "--force")
        assert code == 0
        assert "services/api" in out
        assert "services/worker" in out

    def test_pull_all_confirms_per_service(self, monorepo):
        """Without --force, pull --all asks per service."""
        # create existing .env with different content
        (monorepo / "services/api").mkdir(parents=True, exist_ok=True)
        (monorepo / "services/api/.env").write_text("OLD=x")
        (monorepo / "services/worker").mkdir(parents=True, exist_ok=True)
        (monorepo / "services/worker/.env").write_text("OLD=y")
        # answer y for both
        out, err, code = kv("pull", "--all", input_text="y\ny\n")
        assert code == 0

    def test_list_all(self, monorepo):
        out, err, code = kv("list", "--all")
        assert code == 0
        assert "API_KEY" in out
        assert "WORKER_KEY" in out

    def test_list_all_json(self, monorepo):
        out, err, code = kv("list", "--all", "--json")
        assert code == 0
        data = json.loads(out)
        assert any("API_KEY" in keys for keys in data.values())
        assert any("WORKER_KEY" in keys for keys in data.values())

    def test_diff_all(self, monorepo):
        kv("pull", "--all", "--force")
        out, err, code = kv("diff", "--all")
        assert code == 0
        # both services should report no differences
        assert out.count("no differences") == 2

    def test_diff_all_json(self, monorepo):
        kv("pull", "--all", "--force")
        out, err, code = kv("diff", "--all", "--json")
        assert code == 0
        data = json.loads(out)
        assert len(data) == 2
        for label, diff in data.items():
            assert diff["added"] == {}
            assert diff["removed"] == {}
            assert diff["modified"] == {}

    def test_validate_all(self, monorepo):
        # create templates
        (monorepo / "services/api/.env.example").write_text("API_KEY=\nPORT=\n")
        (monorepo / "services/worker/.env.example").write_text("WORKER_KEY=\nQUEUE=\n")
        out, err, code = kv("validate", "--all")
        assert code == 0

    def test_validate_all_json(self, monorepo):
        (monorepo / "services/api/.env.example").write_text("API_KEY=\nMISSING=\n")
        (monorepo / "services/worker/.env.example").write_text("WORKER_KEY=\n")
        out, err, code = kv("validate", "--all", "--json")
        assert code == 1
        data = json.loads(out)
        assert len(data) == 2
        # one should fail, one should pass
        results = list(data.values())
        assert any(not r["valid"] for r in results)
        assert any(r["valid"] for r in results)

    def test_rotate_all(self, monorepo):
        out, err, code = kv("rotate", "--all")
        assert code == 0
        # secrets still accessible
        kv("pull", "--all", "--force")
        assert "API_KEY=api-secret" in (monorepo / "services/api/.env").read_text()


class TestEdgeCI:
    def test_all_commands_with_sops_age_key(self, project):
        """Simulate CI: use SOPS_AGE_KEY env var instead of key file."""
        key_file = Path.home() / ".config" / "sops" / "age" / "keys.txt"
        if not key_file.exists():
            pytest.skip("no age key file")
        ci_env = {"SOPS_AGE_KEY": key_file.read_text(), "HOME": "/tmp/ci-fake"}

        out, err, code = kv("pull", "--force", env=ci_env)
        assert code == 0

        out, err, code = kv("list", "--json", env=ci_env)
        assert code == 0
        assert "API_KEY" in json.loads(out)

        out, err, code = kv("get", "API_KEY", env=ci_env)
        assert code == 0
        assert out.strip() == "sk-secret-123"

        out, err, code = kv("run", "--", "sh", "-c", "echo $DB_HOST", env=ci_env)
        assert code == 0
        assert "localhost" in out

        out, err, code = kv("doctor", env=ci_env)
        assert code == 0
        assert "SOPS_AGE_KEY" in out
