package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/kint-pro/kint-vault-cli/internal/config"
	"gopkg.in/yaml.v3"
)

func TestInitJoinExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	identityA, _ := age.GenerateX25519Identity()
	pubkeyA := identityA.Recipient().String()

	cfg := config.Config{Env: "dev", Recipients: []string{pubkeyA}}
	data, _ := yaml.Marshal(&cfg)
	os.WriteFile(config.ConfigFile, data, 0o644)

	identityB, _ := age.GenerateX25519Identity()
	pubkeyB := identityB.Recipient().String()
	keyFileB := filepath.Join(tmpDir, "age_key_b.txt")
	keyContent := fmt.Sprintf("# created: by kint-vault\n# public key: %s\n%s\n",
		pubkeyB, identityB.String())
	os.WriteFile(keyFileB, []byte(keyContent), 0o600)

	t.Setenv("SOPS_AGE_KEY_FILE", keyFileB)

	err := doInit("", false)
	if err != nil {
		t.Fatalf("init should join existing config without --force: %v", err)
	}

	cfgData, _ := os.ReadFile(config.ConfigFile)
	var result config.Config
	yaml.Unmarshal(cfgData, &result)

	if len(result.Recipients) != 2 {
		t.Fatalf("expected 2 recipients, got %d: %v", len(result.Recipients), result.Recipients)
	}

	hasA, hasB := false, false
	for _, r := range result.Recipients {
		if r == pubkeyA {
			hasA = true
		}
		if r == pubkeyB {
			hasB = true
		}
	}
	if !hasA {
		t.Fatal("User A's key missing from recipients")
	}
	if !hasB {
		t.Fatal("User B's key missing from recipients")
	}
	if result.Env != "dev" {
		t.Fatalf("env should be preserved as 'dev', got %q", result.Env)
	}
}

func TestInitAlreadyInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	identity, _ := age.GenerateX25519Identity()
	pubkey := identity.Recipient().String()

	cfg := config.Config{Env: "dev", Recipients: []string{pubkey}}
	data, _ := yaml.Marshal(&cfg)
	os.WriteFile(config.ConfigFile, data, 0o644)

	keyFile := filepath.Join(tmpDir, "age_key.txt")
	keyContent := fmt.Sprintf("# created: by kint-vault\n# public key: %s\n%s\n",
		pubkey, identity.String())
	os.WriteFile(keyFile, []byte(keyContent), 0o600)

	t.Setenv("SOPS_AGE_KEY_FILE", keyFile)

	err := doInit("", false)
	if err != nil {
		t.Fatalf("init should succeed when already initialized: %v", err)
	}

	cfgData, _ := os.ReadFile(config.ConfigFile)
	var result config.Config
	yaml.Unmarshal(cfgData, &result)

	if len(result.Recipients) != 1 {
		t.Fatalf("expected 1 recipient (no duplicate), got %d: %v", len(result.Recipients), result.Recipients)
	}
}
