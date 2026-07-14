package main

import (
	"os"
	"strings"
)

// resolveFormat returns the effective output format.
// If format is explicitly "table" it returns "table".
// Otherwise it returns "json" — either because the caller passed "json"
// or because no format was specified (json is the default for agent pipelines).
// When stdout is a terminal and no format was specified, "table" is returned
// so human operators get a readable view without having to pass --format.
func resolveFormat(format string) string {
	if format == "table" {
		return "table"
	}
	if format == "json" {
		return "json"
	}
	// No format specified — use TTY to decide.
	if isTerminal(os.Stdout) {
		return "table"
	}
	return "json"
}

// isTerminal reports whether f is connected to an interactive terminal.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func joinOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ", ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
