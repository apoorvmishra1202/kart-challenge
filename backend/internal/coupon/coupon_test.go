package coupon

import (
	"context"
	"errors"
	"testing"
)

func TestIsWellFormed(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{"", false},
		{"ABC", false},
		{"SEVEN77", false},
		{"SUPER100", true},
		{"NINECHARS", true},
		{"TENCHARS10", true},
		{"ELEVENCHARS", false},
		{"WAYTOOLONGCODE", false},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := IsWellFormed(tt.code); got != tt.want {
				t.Errorf("IsWellFormed(%q) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

type fakeStore struct {
	exists bool
	err    error
	calls  int
}

func (f *fakeStore) Exists(context.Context, string) (bool, error) {
	f.calls++
	return f.exists, f.err
}

func TestServiceValidate(t *testing.T) {
	dbDown := errors.New("db down")
	tests := []struct {
		name      string
		code      string
		store     *fakeStore
		want      bool
		wantErr   error
		wantCalls int
	}{
		{"known code", "HAPPYHRS", &fakeStore{exists: true}, true, nil, 1},
		{"unknown code", "HAPPYHRS", &fakeStore{exists: false}, false, nil, 1},
		{"too short skips store", "ABC", &fakeStore{exists: true}, false, nil, 0},
		{"too long skips store", "WAYTOOLONGCODE", &fakeStore{exists: true}, false, nil, 0},
		{"empty skips store", "", &fakeStore{exists: true}, false, nil, 0},
		{"store failure is an error, not a verdict", "HAPPYHRS", &fakeStore{err: dbDown}, false, dbDown, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewService(tt.store).Validate(context.Background(), tt.code)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Validate = %v, want %v", got, tt.want)
			}
			if tt.store.calls != tt.wantCalls {
				t.Errorf("store called %d times, want %d", tt.store.calls, tt.wantCalls)
			}
		})
	}
}

func TestSourceFileNames(t *testing.T) {
	if len(SourceFileNames) != SourceFiles {
		t.Fatalf("%d names for %d files", len(SourceFileNames), SourceFiles)
	}
	seen := map[string]bool{}
	for _, n := range SourceFileNames {
		if n == "" || seen[n] {
			t.Errorf("bad or duplicate file name %q", n)
		}
		seen[n] = true
	}
}
