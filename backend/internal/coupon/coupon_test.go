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
	has   bool
	err   error
	calls int
}

func (f *fakeStore) Has(context.Context, string) (bool, error) {
	f.calls++
	return f.has, f.err
}

func TestServiceValidate(t *testing.T) {
	dbDown := errors.New("db down")
	tests := []struct {
		name        string
		code        string
		store       *fakeStore
		wantInvalid bool  // error matches ErrInvalid and reports InvalidCoupon()
		wantErr     error // other error expected (store failure)
		wantCalls   int
	}{
		{"known code", "HAPPYHRS", &fakeStore{has: true}, false, nil, 1},
		{"unknown code", "HAPPYHRS", &fakeStore{has: false}, true, nil, 1},
		{"too short skips store", "ABC", &fakeStore{has: true}, true, nil, 0},
		{"too long skips store", "WAYTOOLONGCODE", &fakeStore{has: true}, true, nil, 0},
		{"empty skips store", "", &fakeStore{has: true}, true, nil, 0},
		{"store failure is not a verdict", "HAPPYHRS", &fakeStore{err: dbDown}, false, dbDown, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewService(tt.store).Validate(context.Background(), tt.code)

			var ic interface{ InvalidCoupon() bool }
			isInvalid := errors.Is(err, ErrInvalid)
			reportsInvalid := errors.As(err, &ic) && ic.InvalidCoupon()
			if isInvalid != tt.wantInvalid || reportsInvalid != tt.wantInvalid {
				t.Errorf("err = %v: Is(ErrInvalid) = %v, InvalidCoupon() = %v, want %v",
					err, isInvalid, reportsInvalid, tt.wantInvalid)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
			if !tt.wantInvalid && tt.wantErr == nil && err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if tt.store.calls != tt.wantCalls {
				t.Errorf("store called %d times, want %d", tt.store.calls, tt.wantCalls)
			}
		})
	}
}

func TestMemoryStore(t *testing.T) {
	ctx := context.Background()
	if ok, err := NewMemoryStore().Has(ctx, "HAPPYHRS"); ok || err != nil {
		t.Errorf("empty store Has = %v, %v; want false, nil", ok, err)
	}
	s := NewMemoryStore("HAPPYHRS", "FIFTYOFF")
	for code, want := range map[string]bool{"HAPPYHRS": true, "FIFTYOFF": true, "happyhrs": false, "SUPER100": false} {
		if ok, err := s.Has(ctx, code); ok != want || err != nil {
			t.Errorf("Has(%q) = %v, %v; want %v", code, ok, err, want)
		}
	}
}
