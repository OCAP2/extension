// Package util provides common utility functions used across the OCAP recorder.
package util

import "strings"

// TrimQuotes removes leading and trailing double quotes from a string.
func TrimQuotes(s string) string {
	return strings.Trim(s, `"`)
}

// FixEscapeQuotes replaces escaped double quotes ("") with single double quotes (").
func FixEscapeQuotes(s string) string {
	return strings.ReplaceAll(s, `""`, `"`)
}

