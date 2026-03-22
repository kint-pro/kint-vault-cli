// Package config handles .kint-vault.yaml and .sops.yaml loading/saving.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	ConfigFile = ".kint-vault.yaml"
	SopsConfig = ".sops.yaml"
	EnvExample = ".env.example"
)

type Config struct {
	Backend string `yaml:"backend"`
	Env     string `yaml:"env"`
}

type SopsConfigFile struct {
	CreationRules []CreationRule `yaml:"creation_rules"`
}

type CreationRule struct {
	PathRegex string `yaml:"path_regex"`
	Age       string `yaml:"age"`
}

var envNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func ValidateEnvName(env string) error {
	if !envNameRe.MatchString(env) {
		return fmt.Errorf("invalid environment name: %q", env)
	}
	return nil
}

func EncFile(cfg *Config) string {
	env := cfg.Env
	if env == "" {
		env = "dev"
	}
	if err := ValidateEnvName(env); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return fmt.Sprintf(".env.%s.enc", env)
}

// FindConfigPath searches upward for .kint-vault.yaml and returns its path.
func FindConfigPath() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, ConfigFile)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no %s found. Run: kint-vault init", ConfigFile)
}

func LoadConfig(envOverride string) (*Config, error) {
	configPath, err := FindConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if envOverride != "" {
		cfg.Env = envOverride
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	configPath, err := FindConfigPath()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return AtomicWrite(configPath, data)
}

// AgeKeyFile returns the path to the age key file.
func AgeKeyFile() string {
	if v := os.Getenv("SOPS_AGE_KEY_FILE"); v != "" {
		return v
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "sops", "age", "keys.txt")
	}
	if runtime.GOOS == "windows" {
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			home, _ := os.UserHomeDir()
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appdata, "sops", "age", "keys.txt")
	}
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "sops", "age", "keys.txt")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "sops", "age", "keys.txt")
}

func ReadAgePubkey(keyFile string) (string, error) {
	data, err := os.ReadFile(keyFile)
	if err != nil {
		return "", err
	}
	prefix := "# public key: "
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(line[len(prefix):]), nil
		}
	}
	return "", fmt.Errorf("no public key found in %s", keyFile)
}

// FindSopsConfigPath searches upward for .sops.yaml.
func FindSopsConfigPath() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, SopsConfig)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no %s found. Run: kint-vault init", SopsConfig)
}

func LoadSopsConfig() (*SopsConfigFile, error) {
	path, err := FindSopsConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg SopsConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveSopsConfig(cfg *SopsConfigFile) error {
	path, err := FindSopsConfigPath()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return AtomicWrite(path, data)
}

func GetRecipients() ([]string, error) {
	sops, err := LoadSopsConfig()
	if err != nil {
		return nil, err
	}
	for _, rule := range sops.CreationRules {
		if rule.Age != "" {
			var recipients []string
			for _, r := range strings.Split(rule.Age, ",") {
				r = strings.TrimSpace(r)
				if r != "" {
					recipients = append(recipients, r)
				}
			}
			return recipients, nil
		}
	}
	return nil, nil
}

func AtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp.*")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	tmpPath = "" // prevent cleanup
	return nil
}

func RestrictFile(path string) {
	if runtime.GOOS == "windows" {
		os.Chmod(path, 0o444)
	} else {
		os.Chmod(path, 0o600)
	}
}


func ResolveEnc(cfg *Config, directory string) string {
	enc := EncFile(cfg)
	if directory != "" {
		return filepath.Join(directory, enc)
	}
	return enc
}
