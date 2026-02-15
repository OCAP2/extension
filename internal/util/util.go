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

// ParseSQFStringArray parses a stringified SQF array of 3 quoted strings.
// Input format: ["str1","str2","str3"]
// Returns the three strings. If parsing fails, returns the input as the second
// element (weapon) with empty vehicle and magazine, preserving backward compatibility.
func ParseSQFStringArray(s string) (vehicle, weapon, magazine string) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return "", s, ""
	}

	inner := s[1 : len(s)-1]
	parts := strings.SplitN(inner, `","`, 3)
	if len(parts) != 3 {
		return "", s, ""
	}

	vehicle = FixEscapeQuotes(strings.Trim(parts[0], `"`))
	weapon = FixEscapeQuotes(strings.Trim(parts[1], `"`))
	magazine = FixEscapeQuotes(strings.Trim(parts[2], `"`))
	return vehicle, weapon, magazine
}

// FormatWeaponText builds a display string from the weapon array components.
// Format: "Vehicle: Weapon [Magazine]" with empty parts omitted.
func FormatWeaponText(vehicle, weapon, magazine string) string {
	var b strings.Builder
	if vehicle != "" {
		b.WriteString(vehicle)
		if weapon != "" {
			b.WriteString(": ")
		}
	}
	b.WriteString(weapon)
	if magazine != "" {
		b.WriteString(" [")
		b.WriteString(magazine)
		b.WriteByte(']')
	}
	return b.String()
}

