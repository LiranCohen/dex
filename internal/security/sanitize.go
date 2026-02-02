// Package security provides security utilities for Dex, including
// sanitization of untrusted input to prevent prompt injection attacks.
package security

import (
	"fmt"
	"regexp"
	"strings"
)

// Unicode ranges to remove - compiled once at package init
var (
	// Unicode Tags block (U+E0000-U+E007F) - invisible tag characters
	// These can be used to hide instructions in text
	unicodeTagsRegex = regexp.MustCompile(`[\x{E0000}-\x{E007F}]`)

	// Variation selectors that could hide content
	// FE00-FE0F: Variation Selectors
	// E0100-E01EF: Variation Selectors Supplement
	variationSelectorsRegex = regexp.MustCompile(`[\x{FE00}-\x{FE0F}\x{E0100}-\x{E01EF}]`)
)

// Bidirectional control characters - can reverse text display direction
var bidiControlChars = []rune{
	'\u200E', // LEFT-TO-RIGHT MARK
	'\u200F', // RIGHT-TO-LEFT MARK
	'\u202A', // LEFT-TO-RIGHT EMBEDDING
	'\u202B', // RIGHT-TO-LEFT EMBEDDING
	'\u202C', // POP DIRECTIONAL FORMATTING
	'\u202D', // LEFT-TO-RIGHT OVERRIDE
	'\u202E', // RIGHT-TO-LEFT OVERRIDE
	'\u2066', // LEFT-TO-RIGHT ISOLATE
	'\u2067', // RIGHT-TO-LEFT ISOLATE
	'\u2068', // FIRST STRONG ISOLATE
	'\u2069', // POP DIRECTIONAL ISOLATE
}

// Zero-width and invisible characters - can encode hidden data
var zeroWidthChars = []rune{
	'\u200B', // ZERO WIDTH SPACE
	'\u200C', // ZERO WIDTH NON-JOINER
	'\u200D', // ZERO WIDTH JOINER
	'\u2060', // WORD JOINER
	'\uFEFF', // ZERO WIDTH NO-BREAK SPACE (BOM)
}

// SanitizeForPrompt removes potentially dangerous unicode from text
// that will be included in LLM prompts. This helps prevent prompt
// injection attacks using invisible characters.
//
// Removed characters:
//   - Unicode Tags (U+E0000-U+E007F): invisible tag characters
//   - Variation Selectors: can modify character appearance
//   - Bidirectional controls: can reverse text direction
//   - Zero-width characters: invisible spacers
func SanitizeForPrompt(input string) string {
	if input == "" {
		return ""
	}

	result := input

	// Remove unicode tags
	result = unicodeTagsRegex.ReplaceAllString(result, "")

	// Remove variation selectors
	result = variationSelectorsRegex.ReplaceAllString(result, "")

	// Remove bidi control characters
	for _, char := range bidiControlChars {
		result = strings.ReplaceAll(result, string(char), "")
	}

	// Remove zero-width characters
	for _, char := range zeroWidthChars {
		result = strings.ReplaceAll(result, string(char), "")
	}

	return result
}

// HasDangerousUnicode checks if input contains suspicious unicode characters
// that could be used for prompt injection attacks.
// Returns true and a description if dangerous characters are found.
func HasDangerousUnicode(input string) (bool, string) {
	if input == "" {
		return false, ""
	}

	// Check for unicode tags
	if unicodeTagsRegex.MatchString(input) {
		return true, "contains unicode tag characters (U+E0000-U+E007F)"
	}

	// Check for variation selectors
	if variationSelectorsRegex.MatchString(input) {
		return true, "contains variation selector characters"
	}

	// Check for bidi control characters
	for _, char := range bidiControlChars {
		if strings.ContainsRune(input, char) {
			return true, fmt.Sprintf("contains bidirectional control character (U+%04X)", char)
		}
	}

	// Check for zero-width characters
	for _, char := range zeroWidthChars {
		if strings.ContainsRune(input, char) {
			return true, fmt.Sprintf("contains zero-width character (U+%04X)", char)
		}
	}

	return false, ""
}

// WasSanitized returns true if sanitization changed the input.
// Useful for logging when dangerous content was removed.
func WasSanitized(original, sanitized string) bool {
	return original != sanitized
}

// SanitizeAndReport sanitizes input and returns both the result
// and whether any changes were made. Useful for logging.
func SanitizeAndReport(input string) (sanitized string, changed bool, reason string) {
	sanitized = SanitizeForPrompt(input)
	changed = input != sanitized

	if changed {
		_, reason = HasDangerousUnicode(input)
		if reason == "" {
			reason = "contained dangerous unicode characters"
		}
	}

	return sanitized, changed, reason
}
