// Package coupon owns the promo code business rules: a code is valid when its
// length is within [MinLength, MaxLength] and it appears in at least MinFiles
// of the source files.
package coupon

const (
	MinLength = 8
	MaxLength = 10
	MinFiles  = 2
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
