// Package coupon owns the promo code business rules: a code is valid when its
// length is within [MinLength, MaxLength] and it appears in at least
// min-files of the source files. The importer applies the second rule once and
// stores only valid codes; lookups check the length rule, then membership.
package coupon

import (
	"errors"
	"fmt"
	"strconv"
)

// ErrInvalid means a coupon code was rejected: malformed or not a known
// valid code.
var ErrInvalid = errors.New("invalid coupon")

const (
	MinLength = 8
	MaxLength = 10

	// DefaultMinFiles is how many source files a code must appear in, unless
	// overridden with IMPORT_MIN_FILES.
	DefaultMinFiles = 2
	// SourceFiles is the number of coupon files, the upper bound for min-files.
	SourceFiles = 3
)

// IsWellFormed reports whether code satisfies the length rule.
func IsWellFormed(code string) bool {
	return validLength(len(code))
}

// validLength lets the importer filter raw []byte lines without converting
// each one to a string first.
func validLength(n int) bool {
	return n >= MinLength && n <= MaxLength
}

// ParseMinFiles reads IMPORT_MIN_FILES; empty selects DefaultMinFiles.
func ParseMinFiles(s string) (int, error) {
	if s == "" {
		return DefaultMinFiles, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > SourceFiles {
		return 0, fmt.Errorf("IMPORT_MIN_FILES must be an integer from 1 to %d, got %q", SourceFiles, s)
	}
	return n, nil
}
