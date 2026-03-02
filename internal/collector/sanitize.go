package collector

import (
	"regexp"
	"strings"
)

var unsafeCharsRe = regexp.MustCompile(`[^a-zA-Z0-9_:.@\-]`)
var multiUnderscoreRe = regexp.MustCompile(`_{2,}`)

// SanitizeName replaces special characters in field values
// to ensure safe downstream processing (ES insertion).
// Parentheses are removed (common in hardware names like "Intel(R)").
// Other unsafe chars → '_', then collapse consecutive '_'.
// Keeps: [a-zA-Z0-9_:.@-]
func SanitizeName(s string) string {
	s = strings.NewReplacer("(", "", ")", "").Replace(s)
	s = unsafeCharsRe.ReplaceAllString(s, "_")
	s = multiUnderscoreRe.ReplaceAllString(s, "_")
	return s
}
