package ui

import "strings"

// FuzzyMatch returns true if every character of pattern appears in text in
// order (but not necessarily adjacent). The comparison is case-insensitive.
func FuzzyMatch(text, pattern string) bool {
	if pattern == "" {
		return true
	}
	text = strings.ToLower(text)
	pattern = strings.ToLower(pattern)

	pi := 0
	for _, r := range text {
		if rune(pattern[pi]) == r {
			pi++
			if pi == len(pattern) {
				return true
			}
		}
	}
	return false
}
