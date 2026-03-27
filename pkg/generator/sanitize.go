package generator

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Input validation regexes for security-sensitive string interpolation.
var (
	// safeShellValue allows characters safe for shell script interpolation.
	safeShellValue = regexp.MustCompile(`^[a-zA-Z0-9._/:@-]+$`)

	// safeChartName strips all characters not matching [a-z0-9-] for chart names
	// used in Makefile targets and CI workflow commands.
	safeChartName = regexp.MustCompile(`[^a-z0-9-]`)

	// safeResourceName allows characters safe for YAML resource name entries.
	safeResourceName = regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`)
)

// validateShellSafe checks that a value is safe for interpolation into shell scripts.
// Returns an error if the value contains shell metacharacters.
func validateShellSafe(value, fieldName string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", fieldName)
	}
	if !safeShellValue.MatchString(value) {
		return fmt.Errorf("%s contains unsafe characters for shell interpolation: %q", fieldName, value)
	}
	return nil
}

// validateResourceName checks that a resource name is safe for YAML kustomization entries.
// Rejects path traversal (../), empty names, and characters outside [a-zA-Z0-9._/-].
func validateResourceName(name string) error {
	if name == "" {
		return fmt.Errorf("resource name must not be empty")
	}
	if !safeResourceName.MatchString(name) {
		return fmt.Errorf("resource name contains unsafe characters: %q", name)
	}
	// Reject path traversal components.
	cleaned := filepath.Clean(name)
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("resource name contains path traversal: %q", name)
	}
	return nil
}
