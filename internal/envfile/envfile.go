// Package envfile handles parsing and formatting .env files.
package envfile

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func Parse(content string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if len(value) >= 2 && value[0] == value[len(value)-1] && (value[0] == '"' || value[0] == '\'') {
			value = value[1 : len(value)-1]
		}
		result[key] = value
	}
	return result
}

func Format(secrets map[string]string) string {
	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var lines []string
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", k, secrets[k]))
	}
	return strings.Join(lines, "\n")
}

func SortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type DiffData struct {
	Added    map[string]string         `json:"added"`
	Removed  map[string]string         `json:"removed"`
	Modified map[string]DiffModified   `json:"modified"`
}

type DiffModified struct {
	Local  string `json:"local"`
	Remote string `json:"remote"`
}

func ComputeDiff(local, remote map[string]string) DiffData {
	added := make(map[string]string)
	removed := make(map[string]string)
	modified := make(map[string]DiffModified)

	for k, v := range remote {
		if _, ok := local[k]; !ok {
			added[k] = v
		}
	}
	for k, v := range local {
		if _, ok := remote[k]; !ok {
			removed[k] = v
		}
	}
	for k, lv := range local {
		if rv, ok := remote[k]; ok && lv != rv {
			modified[k] = DiffModified{Local: lv, Remote: rv}
		}
	}
	return DiffData{Added: added, Removed: removed, Modified: modified}
}

// IsExcludedPath checks if a path relative to root contains excluded dirs.
func IsExcludedPath(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	parts := strings.Split(filepath.Dir(rel), string(filepath.Separator))
	for _, part := range parts {
		if strings.HasPrefix(part, ".") {
			return true
		}
		switch part {
		case "node_modules", "__pycache__", ".venv", "venv":
			return true
		}
	}
	return false
}
