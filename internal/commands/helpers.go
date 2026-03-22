package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kint-pro/kint-vault-cli/internal/envfile"
	"github.com/kint-pro/kint-vault-cli/internal/output"
)

var stdinScanner = bufio.NewScanner(os.Stdin)

func fatal(msg string) {
	output.Err(msg)
	os.Exit(1)
}

func prompt(label, defaultVal string) string {
	suffix := ""
	if defaultVal != "" {
		suffix = fmt.Sprintf(" [%s]", defaultVal)
	}
	fmt.Printf("%s%s: ", label, suffix)
	stdinScanner.Scan()
	value := strings.TrimSpace(stdinScanner.Text())
	if value == "" && defaultVal != "" {
		return defaultVal
	}
	if value == "" {
		fatal(fmt.Sprintf("%s is required", label))
	}
	return value
}

func confirm(question string) bool {
	fmt.Printf("%s ", question)
	stdinScanner.Scan()
	return strings.ToLower(strings.TrimSpace(stdinScanner.Text())) == "y"
}

func formatDiff(local, remote map[string]string) string {
	allKeys := make(map[string]bool)
	for k := range local {
		allKeys[k] = true
	}
	for k := range remote {
		allKeys[k] = true
	}

	sorted := envfile.SortedKeys(mergeKeyMaps(local, remote))
	var lines []string
	for _, key := range sorted {
		lv, inLocal := local[key]
		rv, inRemote := remote[key]
		if inLocal && !inRemote {
			lines = append(lines, output.DiffLine("-", output.Red,
				fmt.Sprintf("%s=%s (local only)", key, envfile.Truncate(lv, 40))))
		} else if !inLocal && inRemote {
			lines = append(lines, output.DiffLine("+", output.Green,
				fmt.Sprintf("%s=%s (remote only)", key, envfile.Truncate(rv, 40))))
		} else if lv != rv {
			lines = append(lines, output.DiffLine("~", output.Yellow,
				fmt.Sprintf("%s: %s → %s", key, envfile.Truncate(lv, 40), envfile.Truncate(rv, 40))))
		}
	}
	if len(lines) == 0 {
		return "No differences"
	}
	return strings.Join(lines, "\n")
}

func mergeKeyMaps(a, b map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range a {
		merged[k] = v
	}
	for k, v := range b {
		merged[k] = v
	}
	return merged
}

// findAllEncFilesRecursive finds all .env.{env}.enc files under root.
func findAllEncFilesRecursive(root, env string) []string {
	pattern := fmt.Sprintf(".env.%s.enc", env)
	var results []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) == pattern && !envfile.IsExcludedPath(path, root) {
			results = append(results, path)
		}
		return nil
	})
	return results
}

// findAllEnvFiles finds all .env files under root (not .env.*).
func findAllEnvFiles(root string) []string {
	var results []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) == ".env" && !envfile.IsExcludedPath(path, root) {
			results = append(results, path)
		}
		return nil
	})
	return results
}

// findAllEncFilesAnyEnv finds all .env.*.enc files under root.
func findAllEncFilesAnyEnv(root string) []string {
	var results []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".env.") && strings.HasSuffix(base, ".enc") && !envfile.IsExcludedPath(path, root) {
			results = append(results, path)
		}
		return nil
	})
	return results
}
