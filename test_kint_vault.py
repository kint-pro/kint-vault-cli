import json
import os
import subprocess
from pathlib import Path
from unittest.mock import patch, MagicMock

import pytest
import yaml
from hypothesis import given, assume, settings
from hypothesis import strategies as st

import kint_vault


# --- Strategies ---

env_key_st = st.from_regex(r"[A-Z][A-Z0-9_]{0,20}", fullmatch=True)
env_val_st = st.text(
    alphabet=st.characters(whitelist_categories=("L", "N", "P", "S"), blacklist_characters="\n\r\"'#"),
    min_size=1,
    max_size=50,
)
env_dict_st = st.dictionaries(keys=env_key_st, values=env_val_st, min_size=1, max_size=10)


# --- Parametrized: _parse_env ---


class TestParseEnvParametrized:
    @pytest.mark.parametrize("input,expected", [
        ("A=1", {"A": "1"}),
        ("A=1\nB=2", {"A": "1", "B": "2"}),
        ("", {}),
        ("# comment", {}),
        ("# comment\nA=1", {"A": "1"}),
        ("\n\n  \n", {}),
        ('A="hello"', {"A": "hello"}),
        ("A='hello'", {"A": "hello"}),
        ("URL=postgres://h:5432/db?opt=1", {"URL": "postgres://h:5432/db?opt=1"}),
        ("A=x=y=z", {"A": "x=y=z"}),
        ("INVALID_LINE", {}),
        ("INVALID_LINE\nA=1", {"A": "1"}),
        ("A=", {"A": ""}),
        ("  A  =  hello  ", {"A": "hello"}),
        ("A=hello world", {"A": "hello world"}),
        ("A=hello # not a comment", {"A": "hello # not a comment"}),
        ('#A=1\nB=2', {"B": "2"}),
        ("A=1\n\n\nB=2\n\n", {"A": "1", "B": "2"}),
    ])
    def test_parse_env(self, input, expected):
        assert kint_vault._parse_env(input) == expected


# --- Property-based: _parse_env / _format_env roundtrip ---


class TestParseEnvProperty:
    @given(env_dict_st)
    def test_format_then_parse_roundtrip(self, secrets):
        formatted = kint_vault._format_env(secrets)
        parsed = kint_vault._parse_env(formatted)
        assert parsed == secrets

    @given(env_dict_st)
    def test_format_is_sorted(self, secrets):
        formatted = kint_vault._format_env(secrets)
        lines = formatted.strip().splitlines()
        keys = [l.split("=", 1)[0] for l in lines]
        assert keys == sorted(keys)

    @given(env_dict_st)
    def test_parse_preserves_count(self, secrets):
        formatted = kint_vault._format_env(secrets)
        parsed = kint_vault._parse_env(formatted)
        assert len(parsed) == len(secrets)

    @given(st.text(max_size=200))
    def test_parse_never_crashes(self, text):
        result = kint_vault._parse_env(text)
        assert isinstance(result, dict)


# --- Parametrized: _format_env ---


class TestFormatEnvParametrized:
    @pytest.mark.parametrize("input,expected", [
        ({}, ""),
        ({"A": "1"}, "A=1"),
        ({"B": "2", "A": "1"}, "A=1\nB=2"),
        ({"Z": "last", "A": "first", "M": "mid"}, "A=first\nM=mid\nZ=last"),
    ])
    def test_format_env(self, input, expected):
        assert kint_vault._format_env(input) == expected


# --- Parametrized: _format_diff ---


class TestFormatDiffParametrized:
    @pytest.mark.parametrize("local,remote,expected_fragments", [
        ({"A": "1"}, {"A": "1"}, ["No differences"]),
        ({"A": "1"}, {}, ["- A (local only)"]),
        ({}, {"A": "1"}, ["+ A (remote only)"]),
        ({"A": "1"}, {"A": "2"}, ["~ A (modified)"]),
        ({"A": "1", "B": "2"}, {"B": "3", "C": "4"}, ["- A (local only)", "~ B (modified)", "+ C (remote only)"]),
        ({}, {}, ["No differences"]),
        ({"A": "same", "B": "same"}, {"A": "same", "B": "same"}, ["No differences"]),
    ])
    def test_format_diff(self, local, remote, expected_fragments):
        result = kint_vault._format_diff(local, remote)
        for fragment in expected_fragments:
            assert fragment in result


# --- Property-based: _format_diff ---


class TestFormatDiffProperty:
    @given(env_dict_st)
    def test_identical_dicts_no_differences(self, secrets):
        assert kint_vault._format_diff(secrets, secrets) == "No differences"

    @given(env_dict_st, env_dict_st)
    def test_diff_mentions_all_differing_keys(self, local, remote):
        result = kint_vault._format_diff(local, remote)
        all_keys = set(local) | set(remote)
        for key in all_keys:
            if key in local and key not in remote:
                assert key in result
            elif key not in local and key in remote:
                assert key in result
            elif key in local and key in remote and local[key] != remote[key]:
                assert key in result

    @given(env_dict_st)
    def test_empty_vs_dict_all_remote_only(self, secrets):
        result = kint_vault._format_diff({}, secrets)
        for key in secrets:
            assert f"+ {key} (remote only)" in result


# --- Parametrized: _enc_file ---


class TestEncFileParametrized:
    @pytest.mark.parametrize("config,expected", [
        ({"env": "dev"}, ".env.dev.enc"),
        ({"env": "production"}, ".env.production.enc"),
        ({"env": "staging"}, ".env.staging.enc"),
        ({}, ".env.dev.enc"),
        ({"env": "a-b-c"}, ".env.a-b-c.enc"),
    ])
    def test_enc_file(self, config, expected):
        assert kint_vault._enc_file(config) == expected


# --- Parametrized: _read_age_pubkey ---


class TestReadAgePubkeyParametrized:
    @pytest.mark.parametrize("content,expected", [
        ("# created: 2026-03-20\n# public key: age1abc\nAGE-SECRET-KEY-1XYZ\n", "age1abc"),
        ("# public key: age1longkey123456789\nAGE-SECRET-KEY-1XYZ\n", "age1longkey123456789"),
        ("# other comment\n# public key: age1x\n", "age1x"),
    ])
    def test_reads_pubkey(self, tmp_path, content, expected):
        key_file = tmp_path / "keys.txt"
        key_file.write_text(content)
        assert kint_vault._read_age_pubkey(key_file) == expected

    @pytest.mark.parametrize("content", [
        "AGE-SECRET-KEY-1XYZ\n",
        "# created: 2026\n",
        "",
    ])
    def test_no_pubkey(self, tmp_path, content):
        key_file = tmp_path / "keys.txt"
        key_file.write_text(content)
        with pytest.raises(SystemExit):
            kint_vault._read_age_pubkey(key_file)


# --- Parametrized: _age_key_file ---


class TestAgeKeyFileParametrized:
    @pytest.mark.parametrize("env_vars,platform,expected_suffix", [
        ({"SOPS_AGE_KEY_FILE": "/custom/keys.txt"}, "darwin", "/custom/keys.txt"),
        ({"XDG_CONFIG_HOME": "/xdg"}, "darwin", "/xdg/sops/age/keys.txt"),
        ({"XDG_CONFIG_HOME": "/xdg"}, "linux", "/xdg/sops/age/keys.txt"),
    ])
    def test_key_file_paths(self, monkeypatch, env_vars, platform, expected_suffix):
        monkeypatch.delenv("SOPS_AGE_KEY_FILE", raising=False)
        monkeypatch.delenv("XDG_CONFIG_HOME", raising=False)
        for k, v in env_vars.items():
            monkeypatch.setenv(k, v)
        monkeypatch.setattr("sys.platform", platform)
        assert str(kint_vault._age_key_file()) == expected_suffix

    def test_darwin_default(self, monkeypatch):
        monkeypatch.delenv("SOPS_AGE_KEY_FILE", raising=False)
        monkeypatch.delenv("XDG_CONFIG_HOME", raising=False)
        monkeypatch.setattr("sys.platform", "darwin")
        result = kint_vault._age_key_file()
        assert "sops/age/keys.txt" in str(result)
        assert "Library" in str(result)

    def test_linux_default(self, monkeypatch):
        monkeypatch.delenv("SOPS_AGE_KEY_FILE", raising=False)
        monkeypatch.delenv("XDG_CONFIG_HOME", raising=False)
        monkeypatch.setattr("sys.platform", "linux")
        result = kint_vault._age_key_file()
        assert str(result).endswith(".config/sops/age/keys.txt")


# --- _find_all_enc_files ---


class TestFindAllEncFiles:
    def test_finds_enc_files(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        (tmp_path / ".env.dev.enc").write_text("x")
        (tmp_path / ".env.prod.enc").write_text("x")
        (tmp_path / ".env").write_text("x")
        (tmp_path / "other.txt").write_text("x")
        result = kint_vault._find_all_enc_files()
        names = [p.name for p in result]
        assert names == [".env.dev.enc", ".env.prod.enc"]

    def test_empty_dir(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        assert kint_vault._find_all_enc_files() == []


# --- run_cmd ---


class TestRunCmd:
    def test_success(self):
        assert kint_vault.run_cmd(["echo", "hello"]) == "hello"

    def test_command_not_found(self):
        with pytest.raises(SystemExit, match="Command not found"):
            kint_vault.run_cmd(["nonexistent_binary_xyz"])

    def test_command_failure(self):
        with pytest.raises(SystemExit, match="Command failed"):
            kint_vault.run_cmd(["false"])


# --- Config ---


class TestFindConfig:
    def test_finds_in_cwd(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        config_path = tmp_path / ".kint-vault.yaml"
        config_path.write_text("backend: sops\nenv: dev\n")
        assert kint_vault.find_config() == config_path

    def test_finds_in_parent(self, tmp_path, monkeypatch):
        config_path = tmp_path / ".kint-vault.yaml"
        config_path.write_text("backend: sops\nenv: dev\n")
        subdir = tmp_path / "sub" / "deep"
        subdir.mkdir(parents=True)
        monkeypatch.chdir(subdir)
        assert kint_vault.find_config() == config_path

    def test_not_found(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        with pytest.raises(SystemExit, match="No .kint-vault.yaml found"):
            kint_vault.find_config()


class TestLoadConfig:
    @pytest.mark.parametrize("env_override,expected_env", [
        (None, "dev"),
        ("production", "production"),
        ("staging", "staging"),
    ])
    def test_loads_with_override(self, tmp_path, monkeypatch, env_override, expected_env):
        monkeypatch.chdir(tmp_path)
        (tmp_path / ".kint-vault.yaml").write_text("backend: sops\nenv: dev\n")
        config = kint_vault.load_config(env_override)
        assert config["env"] == expected_env


# --- SOPS config ---


class TestSopsConfig:
    def test_load(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        sops = {"creation_rules": [{"path_regex": "\\.env", "age": "age1abc"}]}
        (tmp_path / ".sops.yaml").write_text(yaml.dump(sops))
        assert kint_vault._load_sops_config() == sops

    def test_not_found(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        with pytest.raises(SystemExit, match="No .sops.yaml found"):
            kint_vault._load_sops_config()

    def test_save(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        sops = {"creation_rules": [{"path_regex": "\\.env", "age": "age1abc"}]}
        (tmp_path / ".sops.yaml").write_text(yaml.dump(sops))
        sops["creation_rules"][0]["age"] = "age1abc,age1def"
        kint_vault._save_sops_config(sops)
        loaded = yaml.safe_load((tmp_path / ".sops.yaml").read_text())
        assert loaded["creation_rules"][0]["age"] == "age1abc,age1def"

    @pytest.mark.parametrize("age_str,expected", [
        ("age1abc,age1def", ["age1abc", "age1def"]),
        ("age1abc, age1def, age1ghi", ["age1abc", "age1def", "age1ghi"]),
        ("age1single", ["age1single"]),
    ])
    def test_get_recipients(self, tmp_path, monkeypatch, age_str, expected):
        monkeypatch.chdir(tmp_path)
        sops = {"creation_rules": [{"path_regex": "\\.env", "age": age_str}]}
        (tmp_path / ".sops.yaml").write_text(yaml.dump(sops))
        assert kint_vault._get_recipients() == expected

    def test_get_recipients_empty(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        sops = {"creation_rules": [{"path_regex": "\\.env"}]}
        (tmp_path / ".sops.yaml").write_text(yaml.dump(sops))
        assert kint_vault._get_recipients() == []


# --- Command tests (with mocked SOPS) ---


def _setup_project(tmp_path, monkeypatch, env="dev"):
    monkeypatch.chdir(tmp_path)
    (tmp_path / ".kint-vault.yaml").write_text(f"backend: sops\nenv: {env}\n")
    sops = {"creation_rules": [{"path_regex": "\\.env", "age": "age1testkey123"}]}
    (tmp_path / ".sops.yaml").write_text(yaml.dump(sops))
    return tmp_path


class TestCmdRun:
    @pytest.mark.parametrize("argv,expected_cmd", [
        (["run", "--", "echo", "hello"], ["echo", "hello"]),
        (["run", "--", "python", "-c", "pass"], ["python", "-c", "pass"]),
        (["run", "echo", "hello"], ["echo", "hello"]),
    ])
    def test_strips_double_dash(self, argv, expected_cmd):
        parser = kint_vault.build_parser()
        args = parser.parse_args(argv)
        command = args.command
        if command and command[0] == "--":
            command = command[1:]
        assert command == expected_cmd

    def test_run_executes(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_decrypt", return_value="A=1"):
            with pytest.raises(SystemExit) as exc:
                parser = kint_vault.build_parser()
                args = parser.parse_args(["run", "--", "true"])
                kint_vault.cmd_run(args)
            assert exc.value.code == 0

    def test_run_no_command(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        parser = kint_vault.build_parser()
        args = parser.parse_args(["run", "--"])
        with pytest.raises(SystemExit, match="No command specified"):
            kint_vault.cmd_run(args)

    def test_run_injects_env(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_decrypt", return_value="TEST_VAR=injected"):
            with patch("subprocess.run") as mock_run:
                mock_run.return_value = MagicMock(returncode=0)
                parser = kint_vault.build_parser()
                args = parser.parse_args(["run", "--", "echo"])
                with pytest.raises(SystemExit):
                    kint_vault.cmd_run(args)
                call_env = mock_run.call_args[1]["env"]
                assert call_env["TEST_VAR"] == "injected"


class TestCmdPull:
    def test_pull_to_file(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_decrypt", return_value="A=1\nB=2"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["pull", "--force"])
            kint_vault.cmd_pull(args)
        env_path = tmp_path / ".env"
        assert env_path.exists()
        assert oct(env_path.stat().st_mode & 0o777) == "0o600"
        assert "A=1" in env_path.read_text()

    def test_pull_refuses_overwrite(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        (tmp_path / ".env").write_text("existing")
        with patch("kint_vault.sops_decrypt", return_value="A=1"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["pull"])
            with pytest.raises(SystemExit, match="already exists"):
                kint_vault.cmd_pull(args)

    def test_pull_force_overwrites(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        (tmp_path / ".env").write_text("old")
        with patch("kint_vault.sops_decrypt", return_value="NEW=val"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["pull", "--force"])
            kint_vault.cmd_pull(args)
        assert "NEW=val" in (tmp_path / ".env").read_text()

    def test_pull_json(self, tmp_path, monkeypatch, capsys):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_decrypt", return_value="A=1\nB=2"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["pull", "--json"])
            kint_vault.cmd_pull(args)
        output = json.loads(capsys.readouterr().out)
        assert output == {"A": "1", "B": "2"}

    def test_pull_stdout(self, tmp_path, monkeypatch, capsys):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_decrypt", return_value="A=1"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["pull", "--stdout"])
            kint_vault.cmd_pull(args)
        assert "A=1" in capsys.readouterr().out

    def test_pull_custom_output(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_decrypt", return_value="A=1"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["pull", "-o", ".env.local"])
            kint_vault.cmd_pull(args)
        assert (tmp_path / ".env.local").exists()


class TestCmdPush:
    def test_push_encrypts(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        (tmp_path / ".env").write_text("A=1\nB=2\n")
        with patch("kint_vault.sops_encrypt_file") as mock_enc:
            parser = kint_vault.build_parser()
            args = parser.parse_args(["push", "-y"])
            kint_vault.cmd_push(args)
            mock_enc.assert_called_once()

    def test_push_file_not_found(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        parser = kint_vault.build_parser()
        args = parser.parse_args(["push", "-y"])
        with pytest.raises(SystemExit, match="File not found"):
            kint_vault.cmd_push(args)

    def test_push_new_env_message(self, tmp_path, monkeypatch, capsys):
        _setup_project(tmp_path, monkeypatch)
        (tmp_path / ".env").write_text("A=1\n")
        with patch("kint_vault.sops_encrypt_file"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["push", "-y"])
            kint_vault.cmd_push(args)
        assert "Created" in capsys.readouterr().out

    def test_push_existing_env_message(self, tmp_path, monkeypatch, capsys):
        _setup_project(tmp_path, monkeypatch)
        (tmp_path / ".env").write_text("A=1\n")
        (tmp_path / ".env.dev.enc").write_text("x")
        with patch("kint_vault.sops_encrypt_file"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["push", "-y"])
            kint_vault.cmd_push(args)
        assert "Encrypted" in capsys.readouterr().out

    def test_push_custom_file(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        (tmp_path / ".env.staging").write_text("A=1\n")
        with patch("kint_vault.sops_encrypt_file") as mock_enc:
            parser = kint_vault.build_parser()
            args = parser.parse_args(["push", "-y", "-f", ".env.staging"])
            kint_vault.cmd_push(args)
            mock_enc.assert_called_once()


class TestCmdSet:
    def test_set_new_key(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        (tmp_path / ".env.dev.enc").write_text("x")
        with patch("kint_vault.sops_decrypt", return_value="A=1"):
            with patch("kint_vault.sops_encrypt_content") as mock_enc:
                parser = kint_vault.build_parser()
                args = parser.parse_args(["set", "B=2"])
                kint_vault.cmd_set(args)
                content = mock_enc.call_args[0][1]
                assert "A=1" in content
                assert "B=2" in content

    def test_set_overwrite_key(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        (tmp_path / ".env.dev.enc").write_text("x")
        with patch("kint_vault.sops_decrypt", return_value="A=old"):
            with patch("kint_vault.sops_encrypt_content") as mock_enc:
                parser = kint_vault.build_parser()
                args = parser.parse_args(["set", "A=new"])
                kint_vault.cmd_set(args)
                assert "A=new" in mock_enc.call_args[0][1]

    def test_set_multiple(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_encrypt_content") as mock_enc:
            parser = kint_vault.build_parser()
            args = parser.parse_args(["set", "A=1", "B=2", "C=3"])
            kint_vault.cmd_set(args)
            content = mock_enc.call_args[0][1]
            assert "A=1" in content
            assert "B=2" in content
            assert "C=3" in content

    def test_set_invalid_format(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        parser = kint_vault.build_parser()
        args = parser.parse_args(["set", "INVALID"])
        with pytest.raises(SystemExit, match="Invalid format"):
            kint_vault.cmd_set(args)

    def test_set_no_existing_file(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_encrypt_content") as mock_enc:
            parser = kint_vault.build_parser()
            args = parser.parse_args(["set", "A=1"])
            kint_vault.cmd_set(args)
            assert "A=1" in mock_enc.call_args[0][1]


class TestCmdGet:
    def test_get_existing(self, tmp_path, monkeypatch, capsys):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_decrypt", return_value="A=secret"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["get", "A"])
            kint_vault.cmd_get(args)
        assert capsys.readouterr().out.strip() == "secret"

    def test_get_missing(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_decrypt", return_value="A=1"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["get", "MISSING"])
            with pytest.raises(SystemExit, match="Key not found"):
                kint_vault.cmd_get(args)


class TestCmdDelete:
    def test_delete_single(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_decrypt", return_value="A=1\nB=2"):
            with patch("kint_vault.sops_encrypt_content") as mock_enc:
                parser = kint_vault.build_parser()
                args = parser.parse_args(["delete", "-y", "A"])
                kint_vault.cmd_delete(args)
                content = mock_enc.call_args[0][1]
                assert "A=" not in content
                assert "B=2" in content

    def test_delete_multiple(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_decrypt", return_value="A=1\nB=2\nC=3"):
            with patch("kint_vault.sops_encrypt_content") as mock_enc:
                parser = kint_vault.build_parser()
                args = parser.parse_args(["delete", "-y", "A", "C"])
                kint_vault.cmd_delete(args)
                content = mock_enc.call_args[0][1]
                assert "A=" not in content
                assert "B=2" in content
                assert "C=" not in content

    def test_delete_missing_key(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_decrypt", return_value="A=1"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["delete", "-y", "MISSING"])
            with pytest.raises(SystemExit, match="Key not found"):
                kint_vault.cmd_delete(args)


class TestCmdList:
    def test_list(self, tmp_path, monkeypatch, capsys):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_decrypt", return_value="B=2\nA=1"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["list"])
            kint_vault.cmd_list(args)
        assert capsys.readouterr().out.strip() == "A\nB"

    def test_list_json(self, tmp_path, monkeypatch, capsys):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.sops_decrypt", return_value="A=1\nB=2"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["list", "--json"])
            kint_vault.cmd_list(args)
        assert json.loads(capsys.readouterr().out) == ["A", "B"]


class TestCmdDiff:
    def test_no_local_env(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        parser = kint_vault.build_parser()
        args = parser.parse_args(["diff"])
        with pytest.raises(SystemExit, match="No local .env"):
            kint_vault.cmd_diff(args)

    def test_diff_output(self, tmp_path, monkeypatch, capsys):
        _setup_project(tmp_path, monkeypatch)
        (tmp_path / ".env").write_text("A=1\nB=local\n")
        with patch("kint_vault.sops_decrypt", return_value="B=remote\nC=3"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["diff"])
            kint_vault.cmd_diff(args)
        output = capsys.readouterr().out
        assert "- A (local only)" in output
        assert "~ B (modified)" in output
        assert "+ C (remote only)" in output


class TestCmdEnv:
    @pytest.mark.parametrize("initial,switch_to", [
        ("dev", "production"),
        ("staging", "dev"),
        ("production", "staging"),
    ])
    def test_switch_env(self, tmp_path, monkeypatch, initial, switch_to):
        _setup_project(tmp_path, monkeypatch, env=initial)
        parser = kint_vault.build_parser()
        args = parser.parse_args(["env", switch_to])
        kint_vault.cmd_env(args)
        config = yaml.safe_load((tmp_path / ".kint-vault.yaml").read_text())
        assert config["env"] == switch_to

    def test_show_env(self, tmp_path, monkeypatch, capsys):
        _setup_project(tmp_path, monkeypatch, env="staging")
        parser = kint_vault.build_parser()
        args = parser.parse_args(["env"])
        kint_vault.cmd_env(args)
        assert capsys.readouterr().out.strip() == "staging"


class TestCmdAddRecipient:
    def test_add(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        with patch("kint_vault.run_cmd"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["add-recipient", "age1newkey456"])
            kint_vault.cmd_add_recipient(args)
        sops = yaml.safe_load((tmp_path / ".sops.yaml").read_text())
        assert "age1newkey456" in sops["creation_rules"][0]["age"]
        assert "age1testkey123" in sops["creation_rules"][0]["age"]

    def test_add_duplicate(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        parser = kint_vault.build_parser()
        args = parser.parse_args(["add-recipient", "age1testkey123"])
        with pytest.raises(SystemExit, match="already exists"):
            kint_vault.cmd_add_recipient(args)

    @pytest.mark.parametrize("key", ["invalid_key", "pgp_fingerprint", "ssh-ed25519 AAAA"])
    def test_add_invalid_key(self, tmp_path, monkeypatch, key):
        _setup_project(tmp_path, monkeypatch)
        parser = kint_vault.build_parser()
        args = parser.parse_args(["add-recipient", key])
        with pytest.raises(SystemExit, match="Invalid age public key"):
            kint_vault.cmd_add_recipient(args)

    def test_add_updates_all_enc_files(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        (tmp_path / ".env.dev.enc").write_text("x")
        (tmp_path / ".env.prod.enc").write_text("x")
        with patch("kint_vault.run_cmd") as mock_run:
            parser = kint_vault.build_parser()
            args = parser.parse_args(["add-recipient", "age1newkey456"])
            kint_vault.cmd_add_recipient(args)
        updatekeys_calls = [
            c for c in mock_run.call_args_list
            if "updatekeys" in c[0][0]
        ]
        assert len(updatekeys_calls) == 2


class TestCmdRemoveRecipient:
    def test_remove(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        (tmp_path / ".kint-vault.yaml").write_text("backend: sops\nenv: dev\n")
        sops = {"creation_rules": [{"path_regex": "\\.env", "age": "age1keep,age1remove"}]}
        (tmp_path / ".sops.yaml").write_text(yaml.dump(sops))
        with patch("kint_vault.run_cmd"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["remove-recipient", "age1remove"])
            kint_vault.cmd_remove_recipient(args)
        loaded = yaml.safe_load((tmp_path / ".sops.yaml").read_text())
        assert "age1remove" not in loaded["creation_rules"][0]["age"]
        assert "age1keep" in loaded["creation_rules"][0]["age"]

    def test_remove_last_fails(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        parser = kint_vault.build_parser()
        args = parser.parse_args(["remove-recipient", "age1testkey123"])
        with pytest.raises(SystemExit, match="Cannot remove last"):
            kint_vault.cmd_remove_recipient(args)

    def test_remove_not_found(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        parser = kint_vault.build_parser()
        args = parser.parse_args(["remove-recipient", "age1nonexistent"])
        with pytest.raises(SystemExit, match="Recipient not found"):
            kint_vault.cmd_remove_recipient(args)

    def test_remove_rotates_all_enc_files(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        (tmp_path / ".kint-vault.yaml").write_text("backend: sops\nenv: dev\n")
        sops = {"creation_rules": [{"path_regex": "\\.env", "age": "age1keep,age1remove"}]}
        (tmp_path / ".sops.yaml").write_text(yaml.dump(sops))
        (tmp_path / ".env.dev.enc").write_text("x")
        (tmp_path / ".env.prod.enc").write_text("x")
        with patch("kint_vault.run_cmd") as mock_run:
            parser = kint_vault.build_parser()
            args = parser.parse_args(["remove-recipient", "age1remove"])
            kint_vault.cmd_remove_recipient(args)
        rotate_calls = [c for c in mock_run.call_args_list if "rotate" in c[0][0]]
        assert len(rotate_calls) == 2


class TestCmdValidate:
    def test_all_present(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        (tmp_path / ".env.example").write_text("A=\nB=\n")
        with patch("kint_vault.sops_decrypt", return_value="A=1\nB=2\nC=3"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["validate"])
            kint_vault.cmd_validate(args)

    def test_missing_keys(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        (tmp_path / ".env.example").write_text("A=\nB=\nC=\n")
        with patch("kint_vault.sops_decrypt", return_value="A=1"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["validate"])
            with pytest.raises(SystemExit):
                kint_vault.cmd_validate(args)

    def test_strict_extra_keys_fails(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        (tmp_path / ".env.example").write_text("A=\n")
        with patch("kint_vault.sops_decrypt", return_value="A=1\nEXTRA=x"):
            parser = kint_vault.build_parser()
            args = parser.parse_args(["validate", "--strict"])
            with pytest.raises(SystemExit):
                kint_vault.cmd_validate(args)

    def test_no_template(self, tmp_path, monkeypatch):
        _setup_project(tmp_path, monkeypatch)
        parser = kint_vault.build_parser()
        args = parser.parse_args(["validate"])
        with pytest.raises(SystemExit, match="Template not found"):
            kint_vault.cmd_validate(args)


class TestCmdInit:
    def test_init_creates_files(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        key_file = tmp_path / "age_keys.txt"
        key_file.write_text("# created: 2026-03-20\n# public key: age1testpub\nAGE-SECRET-KEY-1XYZ\n")
        monkeypatch.setenv("SOPS_AGE_KEY_FILE", str(key_file))
        parser = kint_vault.build_parser()
        args = parser.parse_args(["init", "--env", "dev"])
        kint_vault.cmd_init(args)
        assert (tmp_path / ".kint-vault.yaml").exists()
        assert (tmp_path / ".sops.yaml").exists()
        assert (tmp_path / ".gitignore").exists()
        config = yaml.safe_load((tmp_path / ".kint-vault.yaml").read_text())
        assert config == {"backend": "sops", "env": "dev"}
        sops = yaml.safe_load((tmp_path / ".sops.yaml").read_text())
        assert "age1testpub" in sops["creation_rules"][0]["age"]

    def test_init_preserves_existing_sops(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        key_file = tmp_path / "age_keys.txt"
        key_file.write_text("# created: 2026-03-20\n# public key: age1newdev\nAGE-SECRET-KEY-1XYZ\n")
        monkeypatch.setenv("SOPS_AGE_KEY_FILE", str(key_file))
        existing_sops = {"creation_rules": [{"path_regex": "\\.env", "age": "age1existing"}]}
        (tmp_path / ".sops.yaml").write_text(yaml.dump(existing_sops))
        parser = kint_vault.build_parser()
        args = parser.parse_args(["init", "--env", "dev", "--force"])
        kint_vault.cmd_init(args)
        sops = yaml.safe_load((tmp_path / ".sops.yaml").read_text())
        age_keys = sops["creation_rules"][0]["age"]
        assert "age1existing" in age_keys
        assert "age1newdev" in age_keys

    def test_init_skips_if_already_in_recipients(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        key_file = tmp_path / "age_keys.txt"
        key_file.write_text("# created: 2026-03-20\n# public key: age1existing\nAGE-SECRET-KEY-1XYZ\n")
        monkeypatch.setenv("SOPS_AGE_KEY_FILE", str(key_file))
        existing_sops = {"creation_rules": [{"path_regex": "\\.env", "age": "age1existing"}]}
        (tmp_path / ".sops.yaml").write_text(yaml.dump(existing_sops))
        parser = kint_vault.build_parser()
        args = parser.parse_args(["init", "--env", "dev", "--force"])
        kint_vault.cmd_init(args)
        sops = yaml.safe_load((tmp_path / ".sops.yaml").read_text())
        assert sops["creation_rules"][0]["age"] == "age1existing"

    def test_init_already_exists(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        (tmp_path / ".kint-vault.yaml").write_text("backend: sops\n")
        parser = kint_vault.build_parser()
        args = parser.parse_args(["init", "--env", "dev"])
        with pytest.raises(SystemExit, match="already exists"):
            kint_vault.cmd_init(args)

    def test_init_gitignore_existing(self, tmp_path, monkeypatch):
        monkeypatch.chdir(tmp_path)
        key_file = tmp_path / "age_keys.txt"
        key_file.write_text("# created: 2026-03-20\n# public key: age1x\nAGE-SECRET-KEY-1XYZ\n")
        monkeypatch.setenv("SOPS_AGE_KEY_FILE", str(key_file))
        (tmp_path / ".gitignore").write_text("node_modules\n")
        parser = kint_vault.build_parser()
        args = parser.parse_args(["init", "--env", "dev"])
        kint_vault.cmd_init(args)
        content = (tmp_path / ".gitignore").read_text()
        assert ".env" in content
        assert "node_modules" in content

    def test_init_gitignore_already_has_env(self, tmp_path, monkeypatch, capsys):
        monkeypatch.chdir(tmp_path)
        key_file = tmp_path / "age_keys.txt"
        key_file.write_text("# created: 2026-03-20\n# public key: age1x\nAGE-SECRET-KEY-1XYZ\n")
        monkeypatch.setenv("SOPS_AGE_KEY_FILE", str(key_file))
        (tmp_path / ".gitignore").write_text(".env\n")
        parser = kint_vault.build_parser()
        args = parser.parse_args(["init", "--env", "dev"])
        kint_vault.cmd_init(args)
        assert (tmp_path / ".gitignore").read_text().count(".env") == 1


# --- Parser ---


class TestParser:
    def test_all_commands_registered(self):
        expected = {
            "init", "pull", "push", "run", "set", "get", "delete",
            "list", "diff", "edit", "rotate", "add-recipient",
            "remove-recipient", "validate", "doctor", "env",
        }
        assert set(kint_vault.COMMANDS.keys()) == expected

    def test_no_command_shows_help(self):
        parser = kint_vault.build_parser()
        args = parser.parse_args([])
        assert args.command is None
