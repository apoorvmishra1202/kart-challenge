package coupon

import (
	"fmt"
	"slices"
)

// Codes are packed into a uint64 as base-37 numbers: '0'-'9' map to 1-10 and
// 'A'-'Z' to 11-36. Zero is never a digit, so codes of different lengths can
// never collide ("0000000A" != "00000000A"), and 37^10 < 2^64 so every
// well-formed code fits. Within one length, numeric order equals byte order.
const radix = 37

const alphabet = "?0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ" // index = digit value

func digitValue(c byte) (uint64, bool) {
	switch {
	case c >= '0' && c <= '9':
		return uint64(c-'0') + 1, true
	case c >= 'A' && c <= 'Z':
		return uint64(c-'A') + 11, true
	}
	return 0, false
}

// encode is shared by Encode and the importer's []byte hot path.
func encode[T string | []byte](code T) (uint64, bool) {
	if !validLength(len(code)) {
		return 0, false
	}
	var v uint64
	for i := 0; i < len(code); i++ {
		d, ok := digitValue(code[i])
		if !ok {
			return 0, false
		}
		v = v*radix + d
	}
	return v, true
}

// Encode packs a well-formed code of [A-Z0-9] characters into a uint64.
func Encode(code string) (uint64, error) {
	if !IsWellFormed(code) {
		return 0, fmt.Errorf("code %q: length must be %d-%d", code, MinLength, MaxLength)
	}
	v, ok := encode(code)
	if !ok {
		return 0, fmt.Errorf("code %q: only A-Z and 0-9 are allowed", code)
	}
	return v, nil
}

// Decode reverses Encode. v must come from Encode.
func Decode(v uint64) string {
	var buf [MaxLength]byte
	i := len(buf)
	for v > 0 && i > 0 {
		i--
		buf[i] = alphabet[v%radix]
		v /= radix
	}
	return string(buf[i:])
}

// SortUnique sorts codes in place and removes duplicates.
func SortUnique(codes []uint64) []uint64 {
	slices.Sort(codes)
	return slices.Compact(codes)
}

// MergeValid walks the sorted per-file slices in lockstep and returns, in
// ascending order, every value present in at least minFiles of them. A value
// repeated within one file counts once.
func MergeValid(files [][]uint64, minFiles int) []uint64 {
	idx := make([]int, len(files))
	var out []uint64
	for {
		var lowest uint64
		found := false
		for f, s := range files {
			if idx[f] < len(s) && (!found || s[idx[f]] < lowest) {
				lowest, found = s[idx[f]], true
			}
		}
		if !found {
			return out
		}

		n := 0
		for f, s := range files {
			if idx[f] < len(s) && s[idx[f]] == lowest {
				n++
				for idx[f] < len(s) && s[idx[f]] == lowest {
					idx[f]++
				}
			}
		}
		if n >= minFiles {
			out = append(out, lowest)
		}
	}
}
