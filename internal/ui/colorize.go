package ui

import (
	"regexp"
	"strings"
)

// ANSI escape helpers
const (
	ansiReset     = "\033[0m"
	ansiRed       = "\033[31m"
	ansiRedBold   = "\033[1;31m"
	ansiGreen     = "\033[32m"
	ansiYellow    = "\033[33m"
	ansiBlue      = "\033[34m"
	ansiPurple    = "\033[35m"
	ansiCyan      = "\033[36m"
	ansiDarkCyan  = "\033[36m"
	ansiGray      = "\033[90m"
	ansiDarkGray  = "\033[90m"
	ansiTeal      = "\033[36m"
)

type colorRule struct {
	pattern *regexp.Regexp
	replace string
}

var rules []colorRule

func init() {
	rules = []colorRule{
		// Log levels - ERROR / FATAL / PANIC (red bold)
		{
			pattern: regexp.MustCompile(`(?i)\b(ERROR|FATAL|PANIC)\b`),
			replace: ansiRedBold + "${1}" + ansiReset,
		},
		// Log levels - WARN / WARNING (yellow)
		{
			pattern: regexp.MustCompile(`(?i)\b(WARN|WARNING)\b`),
			replace: ansiYellow + "${1}" + ansiReset,
		},
		// Log levels - INFO (green)
		{
			pattern: regexp.MustCompile(`(?i)\b(INFO)\b`),
			replace: ansiGreen + "${1}" + ansiReset,
		},
		// Log levels - DEBUG / TRACE (gray)
		{
			pattern: regexp.MustCompile(`(?i)\b(DEBUG|TRACE)\b`),
			replace: ansiGray + "${1}" + ansiReset,
		},
		// ISO 8601 timestamps
		{
			pattern: regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?`),
			replace: ansiBlue + "${0}" + ansiReset,
		},
		// Date only
		{
			pattern: regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`),
			replace: ansiBlue + "${0}" + ansiReset,
		},
		// Time only
		{
			pattern: regexp.MustCompile(`\b\d{2}:\d{2}:\d{2}(?:\.\d+)?\b`),
			replace: ansiBlue + "${0}" + ansiReset,
		},
		// IPv4 addresses
		{
			pattern: regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
			replace: ansiDarkCyan + "${0}" + ansiReset,
		},
		// HTTP methods
		{
			pattern: regexp.MustCompile(`\b(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\b`),
			replace: ansiPurple + "${1}" + ansiReset,
		},
		// HTTP status codes 5xx (red)
		{
			pattern: regexp.MustCompile(`\b(5\d{2})\b`),
			replace: ansiRed + "${1}" + ansiReset,
		},
		// HTTP status codes 4xx (yellow)
		{
			pattern: regexp.MustCompile(`\b(4\d{2})\b`),
			replace: ansiYellow + "${1}" + ansiReset,
		},
		// Quoted strings
		{
			pattern: regexp.MustCompile(`"([^"]*?)"`),
			replace: ansiTeal + `"${1}"` + ansiReset,
		},
		// Key=value pairs - colorize only the key
		{
			pattern: regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)=`),
			replace: ansiDarkGray + "${1}" + ansiReset + "=",
		},
	}
}

// ColorizeLine applies ANSI color rules to a single line of log output.
func ColorizeLine(line string) string {
	for _, r := range rules {
		line = r.pattern.ReplaceAllString(line, r.replace)
	}
	return line
}

// colorizeBlock colorizes a multi-line block of text.
func colorizeBlock(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = ColorizeLine(line)
	}
	return strings.Join(lines, "\n")
}
