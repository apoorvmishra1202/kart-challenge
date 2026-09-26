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
	count int
	err   error
	calls int
}

func (f *fakeStore) CountFiles(context.Context, string) (int, error) {
	f.calls++
	return f.count, f.err
}

func TestServiceValidate(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  bool
	}{
		{"in no files", 0, false},
		{"in one file", 1, false},
		{"in two files", 2, true},
		{"in three files", 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(&fakeStore{count: tt.count})
			got, err := svc.Validate(context.Background(), "HAPPYHRS")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Validate with count %d = %v, want %v", tt.count, got, tt.want)
			}
		})
	}
}

func TestServiceValidateMalformedSkipsStore(t *testing.T) {
	store := &fakeStore{count: 3}
	got, err := NewService(store).Validate(context.Background(), "ABC")
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("malformed code reported valid")
	}
	if store.calls != 0 {
		t.Errorf("store called %d times, want 0", store.calls)
	}
}

func TestServiceValidateStoreError(t *testing.T) {
	wantErr := errors.New("db down")
	_, err := NewService(&fakeStore{err: wantErr}).Validate(context.Background(), "HAPPYHRS")
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}
