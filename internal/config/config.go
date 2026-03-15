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
	EnvExample = ".env.example"
)

type Config struct {
	Env        string   `yaml:"env"`
	Recipients []string `yaml:"recipients"`
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

func GetRecipients() ([]string, error) {
	cfg, err := LoadConfig("")
	if err != nil {
		return nil, err
	}
	return cfg.Recipients, nil
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
	tmpPath = ""
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
