package ui

import (
	"regexp"
	"strings"

	"github.com/rivo/tview"
)

// colorRule pairs a compiled regex with a tview color-tag replacement string.
type colorRule struct {
	pattern *regexp.Regexp
	replace string
}

// rules are applied in order per line. Earlier rules take priority where
// matches overlap. All patterns assume literal "[" has already been escaped
// to "[[]" so tview doesn't interpret them as color tags.
var rules []colorRule

func init() {
	rules = []colorRule{
		// Log levels — ERROR / FATAL / PANIC (red bold)
		{
			pattern: regexp.MustCompile(`(?i)\b(ERROR|FATAL|PANIC)\b`),
			replace: "[red::b]${1}[-::-]",
		},
		// Log levels — WARN / WARNING (yellow)
		{
			pattern: regexp.MustCompile(`(?i)\b(WARN|WARNING)\b`),
			replace: "[yellow]${1}[-]",
		},
		// Log levels — INFO (green)
		{
			pattern: regexp.MustCompile(`(?i)\b(INFO)\b`),
			replace: "[green]${1}[-]",
		},
		// Log levels — DEBUG / TRACE (gray)
		{
			pattern: regexp.MustCompile(`(?i)\b(DEBUG|TRACE)\b`),
			replace: "[gray]${1}[-]",
		},
		// ISO 8601 timestamps: 2024-01-15T10:30:00Z or with offset
		{
			pattern: regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?`),
			replace: "[blue]${0}[-]",
		},
		// Date only: 2024-01-15
		{
			pattern: regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`),
			replace: "[blue]${0}[-]",
		},
		// Time only: 15:04:05 or 15:04:05.123
		{
			pattern: regexp.MustCompile(`\b\d{2}:\d{2}:\d{2}(?:\.\d+)?\b`),
			replace: "[blue]${0}[-]",
		},
		// IPv4 addresses
		{
			pattern: regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
			replace: "[darkcyan]${0}[-]",
		},
		// HTTP methods
		{
			pattern: regexp.MustCompile(`\b(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\b`),
			replace: "[purple]${1}[-]",
		},
		// HTTP status codes 5xx (red)
		{
			pattern: regexp.MustCompile(`\b(5\d{2})\b`),
			replace: "[red]${1}[-]",
		},
		// HTTP status codes 4xx (yellow)
		{
			pattern: regexp.MustCompile(`\b(4\d{2})\b`),
			replace: "[yellow]${1}[-]",
		},
		// Quoted strings
		{
			pattern: regexp.MustCompile(`"([^"]*?)"`),
			replace: "[teal]\"${1}\"[-]",
		},
		// Key=value pairs — colorize only the key
		{
			pattern: regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)=`),
			replace: "[darkgray]${1}[-]=",
		},
	}
}

// ColorizeLine applies color-tag rules to a single line of log output.
// Literal style tags are escaped first so tview doesn't misinterpret them.
func ColorizeLine(line string) string {
	// Escape any text that looks like a tview style tag (e.g. "[ERROR]")
	// so it's displayed literally. Our rules then inject real color tags.
	line = tview.Escape(line)

	for _, r := range rules {
		line = r.pattern.ReplaceAllString(line, r.replace)
	}
	return line
}

// colorizeBlock colorizes a multi-line block of text (e.g. initial file load).
func colorizeBlock(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = ColorizeLine(line)
	}
	return strings.Join(lines, "\n")
}
