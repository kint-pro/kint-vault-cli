import json
import os
import subprocess
from pathlib import Path

import pytest
import yaml

import kint_vault


def _has_sops_and_age():
    try:
        subprocess.run(["sops", "--version"], capture_output=True, check=True)
        subprocess.run(["age-keygen", "--help"], capture_output=True)
        return True
    except (FileNotFoundError, subprocess.CalledProcessError):
        return False


pytestmark = pytest.mark.skipif(not _has_sops_and_age(), reason="sops and age required")


@pytest.fixture
def vault_project(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)

    key_file = tmp_path / "age-key.txt"
    subprocess.run(["age-keygen", "-o", str(key_file)], capture_output=True, check=True)
    key_file.chmod(0o600)
    monkeypatch.setenv("SOPS_AGE_KEY_FILE", str(key_file))

    pubkey = kint_vault._read_age_pubkey(key_file)

    (tmp_path / ".kint-vault.yaml").write_text("backend: sops\nenv: dev\n")
    sops_config = {"creation_rules": [{"path_regex": "\\.env", "age": pubkey}]}
    (tmp_path / ".sops.yaml").write_text(yaml.dump(sops_config))

    return {"path": tmp_path, "pubkey": pubkey, "key_file": key_file}


@pytest.fixture
def vault_with_secrets(vault_project):
    path = vault_project["path"]
    (path / ".env").write_text("DB_HOST=localhost\nDB_PASS=secret123\nAPI_KEY=sk-abc\n")
    parser = kint_vault.build_parser()
    args = parser.parse_args(["push", "-y"])
    kint_vault.cmd_push(args)
    return vault_project


class TestFullFlow:
    def test_push_creates_encrypted_file(self, vault_project):
        path = vault_project["path"]
        (path / ".env").write_text("A=1\nB=2\n")
        parser = kint_vault.build_parser()
        args = parser.parse_args(["push", "-y"])
        kint_vault.cmd_push(args)
        enc = path / ".env.dev.enc"
        assert enc.exists()
        content = enc.read_text()
        assert "A=ENC[" in content
        assert "B=ENC[" in content

    def test_push_then_pull(self, vault_project):
        path = vault_project["path"]
        (path / ".env").write_text("SECRET=hello_world\n")
        parser = kint_vault.build_parser()
        kint_vault.cmd_push(parser.parse_args(["push", "-y"]))
        (path / ".env").unlink()
        kint_vault.cmd_pull(parser.parse_args(["pull"]))
        content = (path / ".env").read_text()
        assert "SECRET=hello_world" in content

    def test_push_pull_preserves_all_secrets(self, vault_project):
        path = vault_project["path"]
        original = "A=1\nB=two\nC_URL=postgres://h:5432/db\nD=with spaces\n"
        (path / ".env").write_text(original)
        parser = kint_vault.build_parser()
        kint_vault.cmd_push(parser.parse_args(["push", "-y"]))
        (path / ".env").unlink()
        kint_vault.cmd_pull(parser.parse_args(["pull"]))
        parsed = kint_vault._parse_env((path / ".env").read_text())
        assert parsed == {"A": "1", "B": "two", "C_URL": "postgres://h:5432/db", "D": "with spaces"}


class TestSet:
    def test_set_creates_new_secret(self, vault_with_secrets):
        parser = kint_vault.build_parser()
        kint_vault.cmd_set(parser.parse_args(["set", "NEW_KEY=new_value"]))
        content = kint_vault.sops_decrypt(kint_vault.load_config())
        secrets = kint_vault._parse_env(content)
        assert secrets["NEW_KEY"] == "new_value"
        assert secrets["DB_HOST"] == "localhost"

    def test_set_overwrites_existing(self, vault_with_secrets):
        parser = kint_vault.build_parser()
        kint_vault.cmd_set(parser.parse_args(["set", "DB_PASS=changed"]))
        content = kint_vault.sops_decrypt(kint_vault.load_config())
        assert kint_vault._parse_env(content)["DB_PASS"] == "changed"

    def test_set_multiple_at_once(self, vault_with_secrets):
        parser = kint_vault.build_parser()
        kint_vault.cmd_set(parser.parse_args(["set", "X=1", "Y=2", "Z=3"]))
        secrets = kint_vault._parse_env(kint_vault.sops_decrypt(kint_vault.load_config()))
        assert secrets["X"] == "1"
        assert secrets["Y"] == "2"
        assert secrets["Z"] == "3"


class TestGet:
    def test_get_value(self, vault_with_secrets, capsys):
        parser = kint_vault.build_parser()
        kint_vault.cmd_get(parser.parse_args(["get", "DB_PASS"]))
        assert capsys.readouterr().out.strip() == "secret123"

    def test_get_missing_key(self, vault_with_secrets):
        parser = kint_vault.build_parser()
        with pytest.raises(SystemExit, match="Key not found"):
            kint_vault.cmd_get(parser.parse_args(["get", "NONEXISTENT"]))


class TestDelete:
    def test_delete_removes_key(self, vault_with_secrets):
        parser = kint_vault.build_parser()
        kint_vault.cmd_delete(parser.parse_args(["delete", "-y", "API_KEY"]))
        secrets = kint_vault._parse_env(kint_vault.sops_decrypt(kint_vault.load_config()))
        assert "API_KEY" not in secrets
        assert "DB_HOST" in secrets

    def test_delete_multiple(self, vault_with_secrets):
        parser = kint_vault.build_parser()
        kint_vault.cmd_delete(parser.parse_args(["delete", "-y", "API_KEY", "DB_PASS"]))
        secrets = kint_vault._parse_env(kint_vault.sops_decrypt(kint_vault.load_config()))
        assert "API_KEY" not in secrets
        assert "DB_PASS" not in secrets
        assert "DB_HOST" in secrets


class TestList:
    def test_list_keys(self, vault_with_secrets, capsys):
        parser = kint_vault.build_parser()
        kint_vault.cmd_list(parser.parse_args(["list"]))
        output = capsys.readouterr().out.strip()
        assert output == "API_KEY\nDB_HOST\nDB_PASS"

    def test_list_json(self, vault_with_secrets, capsys):
        parser = kint_vault.build_parser()
        kint_vault.cmd_list(parser.parse_args(["list", "--json"]))
        keys = json.loads(capsys.readouterr().out)
        assert keys == ["API_KEY", "DB_HOST", "DB_PASS"]


class TestDiff:
    def test_no_diff(self, vault_with_secrets, capsys):
        parser = kint_vault.build_parser()
        kint_vault.cmd_diff(parser.parse_args(["diff"]))
        assert "No differences" in capsys.readouterr().out

    def test_local_modified(self, vault_with_secrets, capsys):
        path = vault_with_secrets["path"]
        (path / ".env").write_text("DB_HOST=changed\nDB_PASS=secret123\nAPI_KEY=sk-abc\n")
        parser = kint_vault.build_parser()
        kint_vault.cmd_diff(parser.parse_args(["diff"]))
        output = capsys.readouterr().out
        assert "~ DB_HOST (modified)" in output

    def test_local_extra_key(self, vault_with_secrets, capsys):
        path = vault_with_secrets["path"]
        (path / ".env").write_text("DB_HOST=localhost\nDB_PASS=secret123\nAPI_KEY=sk-abc\nEXTRA=x\n")
        parser = kint_vault.build_parser()
        kint_vault.cmd_diff(parser.parse_args(["diff"]))
        assert "- EXTRA (local only)" in capsys.readouterr().out


class TestValidate:
    def test_validate_pass(self, vault_with_secrets):
        path = vault_with_secrets["path"]
        (path / ".env.example").write_text("DB_HOST=\nDB_PASS=\n")
        parser = kint_vault.build_parser()
        kint_vault.cmd_validate(parser.parse_args(["validate"]))

    def test_validate_fail(self, vault_with_secrets):
        path = vault_with_secrets["path"]
        (path / ".env.example").write_text("DB_HOST=\nMISSING_KEY=\n")
        parser = kint_vault.build_parser()
        with pytest.raises(SystemExit):
            kint_vault.cmd_validate(parser.parse_args(["validate"]))


class TestRotate:
    def test_rotate_still_decryptable(self, vault_with_secrets):
        parser = kint_vault.build_parser()
        kint_vault.cmd_rotate(parser.parse_args(["rotate"]))
        secrets = kint_vault._parse_env(kint_vault.sops_decrypt(kint_vault.load_config()))
        assert secrets["DB_HOST"] == "localhost"
        assert secrets["DB_PASS"] == "secret123"


class TestEnvironments:
    def test_multiple_environments(self, vault_project):
        path = vault_project["path"]
        parser = kint_vault.build_parser()

        (path / ".env").write_text("A=dev_value\n")
        kint_vault.cmd_push(parser.parse_args(["push", "-y"]))
        assert (path / ".env.dev.enc").exists()

        (path / ".env").write_text("A=prod_value\n")
        kint_vault.cmd_push(parser.parse_args(["push", "-y", "--env", "prod"]))
        assert (path / ".env.prod.enc").exists()

        dev_secrets = kint_vault._parse_env(kint_vault.sops_decrypt({"env": "dev"}))
        prod_secrets = kint_vault._parse_env(kint_vault.sops_decrypt({"env": "prod"}))
        assert dev_secrets["A"] == "dev_value"
        assert prod_secrets["A"] == "prod_value"

    def test_env_switch(self, vault_project):
        parser = kint_vault.build_parser()
        kint_vault.cmd_env(parser.parse_args(["env", "staging"]))
        config = kint_vault.load_config()
        assert config["env"] == "staging"


class TestRecipients:
    def test_add_second_recipient(self, vault_with_secrets, tmp_path):
        second_key = tmp_path / "second-key.txt"
        subprocess.run(["age-keygen", "-o", str(second_key)], capture_output=True, check=True)
        second_pubkey = kint_vault._read_age_pubkey(second_key)

        parser = kint_vault.build_parser()
        kint_vault.cmd_add_recipient(parser.parse_args(["add-recipient", second_pubkey]))

        sops = yaml.safe_load((tmp_path / ".sops.yaml").read_text())
        recipients = sops["creation_rules"][0]["age"]
        assert second_pubkey in recipients
        assert vault_with_secrets["pubkey"] in recipients

        secrets = kint_vault._parse_env(kint_vault.sops_decrypt(kint_vault.load_config()))
        assert secrets["DB_HOST"] == "localhost"


class TestInit:
    def test_init_full(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        key_file = tmp_path / "age-key.txt"
        subprocess.run(["age-keygen", "-o", str(key_file)], capture_output=True, check=True)
        monkeypatch.setenv("SOPS_AGE_KEY_FILE", str(key_file))

        parser = kint_vault.build_parser()
        kint_vault.cmd_init(parser.parse_args(["init", "--env", "dev"]))

        assert (tmp_path / ".kint-vault.yaml").exists()
        assert (tmp_path / ".sops.yaml").exists()
        assert (tmp_path / ".gitignore").exists()
        assert ".env" in (tmp_path / ".gitignore").read_text()

        pubkey = kint_vault._read_age_pubkey(key_file)
        sops = yaml.safe_load((tmp_path / ".sops.yaml").read_text())
        assert pubkey in sops["creation_rules"][0]["age"]

    def test_init_then_push_pull(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        key_file = tmp_path / "age-key.txt"
        subprocess.run(["age-keygen", "-o", str(key_file)], capture_output=True, check=True)
        monkeypatch.setenv("SOPS_AGE_KEY_FILE", str(key_file))

        parser = kint_vault.build_parser()
        kint_vault.cmd_init(parser.parse_args(["init", "--env", "dev"]))

        (tmp_path / ".env").write_text("TOKEN=xyz\n")
        kint_vault.cmd_push(parser.parse_args(["push", "-y"]))
        (tmp_path / ".env").unlink()

        kint_vault.cmd_pull(parser.parse_args(["pull"]))
        assert "TOKEN=xyz" in (tmp_path / ".env").read_text()


class TestPullOutputModes:
    def test_pull_to_custom_file(self, vault_with_secrets):
        path = vault_with_secrets["path"]
        (path / ".env").unlink()
        parser = kint_vault.build_parser()
        kint_vault.cmd_pull(parser.parse_args(["pull", "-o", ".env.local"]))
        content = (path / ".env.local").read_text()
        assert "DB_HOST=localhost" in content

    def test_pull_json(self, vault_with_secrets, capsys):
        parser = kint_vault.build_parser()
        kint_vault.cmd_pull(parser.parse_args(["pull", "--json"]))
        data = json.loads(capsys.readouterr().out)
        assert data["DB_HOST"] == "localhost"

    def test_pull_stdout(self, vault_with_secrets, capsys):
        parser = kint_vault.build_parser()
        kint_vault.cmd_pull(parser.parse_args(["pull", "--stdout"]))
        output = capsys.readouterr().out
        assert "DB_HOST=localhost" in output

    def test_pull_file_permissions(self, vault_with_secrets):
        path = vault_with_secrets["path"]
        (path / ".env").unlink()
        parser = kint_vault.build_parser()
        kint_vault.cmd_pull(parser.parse_args(["pull"]))
        assert oct((path / ".env").stat().st_mode & 0o777) == "0o600"
