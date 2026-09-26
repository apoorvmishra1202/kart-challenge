package coupon

import (
	"slices"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	codes := []string{
		"SUPER100", "00000000", "ZZZZZZZZ", "A1B2C3D4", // 8
		"NINECHARS", "000000000", "ZZZZZZZZZ", // 9
		"TENCHARS10", "0000000000", "ZZZZZZZZZZ", "BIRTHDAY10", // 10
	}
	seen := make(map[uint64]string)
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			v, err := Encode(code)
			if err != nil {
				t.Fatal(err)
			}
			if got := Decode(v); got != code {
				t.Errorf("Decode(Encode(%q)) = %q", code, got)
			}
			if other, dup := seen[v]; dup {
				t.Errorf("%q and %q both encode to %d", code, other, v)
			}
			seen[v] = code
		})
	}
}

func TestEncodeOrderMatchesByteOrderWithinLength(t *testing.T) {
	codes := []string{"00000000", "0000000A", "00000010", "SUPER100", "SUPERAAA", "ZZZZZZZZ"}
	for i := 1; i < len(codes); i++ {
		a, _ := Encode(codes[i-1])
		b, _ := Encode(codes[i])
		if a >= b {
			t.Errorf("Encode(%q)=%d >= Encode(%q)=%d", codes[i-1], a, codes[i], b)
		}
	}
}

func TestEncodeRejects(t *testing.T) {
	for _, code := range []string{"", "SEVEN77", "ELEVENCHARS", "super100", "SUPER-10", "SUPER 10", "SUPÉR100"} {
		if _, err := Encode(code); err == nil {
			t.Errorf("Encode(%q) succeeded, want error", code)
		}
	}
}

func enc(t *testing.T, codes ...string) []uint64 {
	t.Helper()
	out := make([]uint64, len(codes))
	for i, c := range codes {
		v, err := Encode(c)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = v
	}
	return out
}

func dec(vs []uint64) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = Decode(v)
	}
	return out
}

func TestMergeValid(t *testing.T) {
	tests := []struct {
		name  string
		files [][]string
		want  []string
	}{
		{
			name: "in 1, 2 and 3 files",
			files: [][]string{
				{"SUPER100", "HAPPYHRS", "FIFTYOFF", "ONLYFILE1"},
				{"HAPPYHRS", "FIFTYOFF", "ONLYFILE2", "BIRTHDAY10"},
				{"FIFTYOFF", "SUPER100", "BIRTHDAY10", "ONLYFILE3"},
			},
			want: []string{"FIFTYOFF", "HAPPYHRS", "SUPER100", "BIRTHDAY10"},
		},
		{
			name:  "duplicates within one file count once",
			files: [][]string{{"DUPEDUPE", "DUPEDUPE", "DUPEDUPE"}, {}, {"OTHER123"}},
			want:  nil,
		},
		{
			name:  "duplicated in one file plus once in another",
			files: [][]string{{"DUPEDUPE", "DUPEDUPE"}, {"DUPEDUPE"}, {}},
			want:  []string{"DUPEDUPE"},
		},
		{
			name:  "all empty",
			files: [][]string{{}, {}, {}},
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := make([][]uint64, len(tt.files))
			for i, f := range tt.files {
				files[i] = SortUnique(enc(t, f...))
			}
			got := dec(MergeValid(files, MinFiles))
			if !slices.Equal(got, tt.want) && !(len(got) == 0 && len(tt.want) == 0) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMergeValidWithoutPriorDedupe(t *testing.T) {
	// Duplicates must not inflate the count or repeat in the output even if
	// the caller skipped SortUnique's compaction.
	a := enc(t, "DUPEDUPE", "DUPEDUPE")
	b := enc(t, "DUPEDUPE", "DUPEDUPE")
	got := dec(MergeValid([][]uint64{a, b, nil}, MinFiles))
	if !slices.Equal(got, []string{"DUPEDUPE"}) {
		t.Errorf("got %v, want [DUPEDUPE]", got)
	}
	got = dec(MergeValid([][]uint64{a, nil, nil}, MinFiles))
	if len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func TestSortUnique(t *testing.T) {
	got := SortUnique([]uint64{5, 1, 5, 3, 1, 1})
	if !slices.Equal(got, []uint64{1, 3, 5}) {
		t.Errorf("got %v", got)
	}
}
