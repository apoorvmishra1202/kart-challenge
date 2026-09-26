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
	tests := []struct {
		name   string
		exists bool
		want   bool
	}{
		{"not in valid_codes", false, false},
		{"in valid_codes", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{exists: tt.exists}
			got, err := NewService(store).Validate(context.Background(), "HAPPYHRS")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Validate = %v, want %v", got, tt.want)
			}
			if store.calls != 1 {
				t.Errorf("store called %d times, want 1", store.calls)
			}
		})
	}
}

func TestServiceValidateMalformedSkipsStore(t *testing.T) {
	store := &fakeStore{exists: true}
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
