// Package output provides colored terminal output helpers.
package output

import (
	"fmt"
	"os"
)

const (
	Red    = "\033[91m"
	Green  = "\033[92m"
	Yellow = "\033[93m"
	Blue   = "\033[94m"
	Reset  = "\033[0m"
)

func ColorEnabled() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	_, noColor := os.LookupEnv("NO_COLOR")
	return !noColor
}

func msg(icon, color, fallback, message string, isErr bool) {
	w := os.Stdout
	if isErr {
		w = os.Stderr
	}
	if ColorEnabled() {
		fmt.Fprintf(w, "%s%s%s %s\n", color, icon, Reset, message)
	} else {
		fmt.Fprintf(w, "%s%s\n", fallback, message)
	}
}

func Ok(message string) {
	msg("✓", Green, "OK: ", message, false)
}

func Err(message string) {
	msg("✗", Red, "ERROR: ", message, true)
}

func Warn(message string) {
	msg("⚠", Yellow, "WARNING: ", message, false)
}

func Info(message string) {
	msg("→", Blue, "", message, false)
}

func DiffLine(symbol, color, text string) string {
	if ColorEnabled() {
		return fmt.Sprintf("  %s%s%s %s", color, symbol, Reset, text)
	}
	return fmt.Sprintf("  %s %s", symbol, text)
}

func PrintChecks(checks []Check) {
	for _, c := range checks {
		var status string
		if ColorEnabled() {
			if c.Passed {
				status = Green + "✓" + Reset
			} else {
				status = Red + "✗" + Reset
			}
		} else {
			if c.Passed {
				status = "OK"
			} else {
				status = "FAIL"
			}
		}
		suffix := ""
		if c.Detail != "" {
			suffix = " (" + c.Detail + ")"
		}
		fmt.Printf("  %s %s%s\n", status, c.Name, suffix)
	}
}

type Check struct {
	Name   string
	Passed bool
	Detail string
}
