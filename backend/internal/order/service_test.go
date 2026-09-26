package order

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"testing"

	"shop/internal/product"
)

// fakeProducts serves a fixed catalogue, or err for every lookup.
type fakeProducts struct {
	byID map[string]product.Product
	err  error
}

func (f *fakeProducts) Get(_ context.Context, id string) (product.Product, error) {
	if f.err != nil {
		return product.Product{}, f.err
	}
	p, ok := f.byID[id]
	if !ok {
		return product.Product{}, fmt.Errorf("get product %s: %w", id, product.ErrNotFound)
	}
	return p, nil
}

type fakeCoupons struct {
	valid map[string]bool
	err   error // returned instead of a verdict (validator failure)
	calls []string
}

func (f *fakeCoupons) Validate(_ context.Context, code string) (bool, error) {
	f.calls = append(f.calls, code)
	if f.err != nil {
		return false, f.err
	}
	return f.valid[code], nil
}

type fakeStore struct {
	saved []Order
	err   error
}

func (f *fakeStore) Save(_ context.Context, o Order) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, o)
	return nil
}

var (
	waffle   = product.Product{ID: "1", Name: "Waffle", Price: 650, Category: "Waffle"}
	tiramisu = product.Product{ID: "2", Name: "Tiramisu", Price: 550, Category: "Tiramisu"}
	catalog  = map[string]product.Product{"1": waffle, "2": tiramisu}
	errInfra = errors.New("connection refused")
	uuidV4   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

func TestServicePlace(t *testing.T) {
	tests := []struct {
		name         string
		items        []Item
		coupon       string
		products     *fakeProducts
		coupons      *fakeCoupons
		store        *fakeStore
		wantItems    []Item
		wantProducts []product.Product
		wantErrIs    error // sentinel the error must match
		wantErrNot   []error
		wantCoupons  []string // codes passed to the validator
	}{
		{
			name:         "single item, no coupon",
			items:        []Item{{"1", 2}},
			products:     &fakeProducts{byID: catalog},
			coupons:      &fakeCoupons{},
			store:        &fakeStore{},
			wantItems:    []Item{{"1", 2}},
			wantProducts: []product.Product{waffle},
		},
		{
			name:         "duplicates merged in first-seen order",
			items:        []Item{{"2", 1}, {"1", 2}, {"2", 3}},
			products:     &fakeProducts{byID: catalog},
			coupons:      &fakeCoupons{},
			store:        &fakeStore{},
			wantItems:    []Item{{"2", 4}, {"1", 2}},
			wantProducts: []product.Product{tiramisu, waffle},
		},
		{
			name:         "valid coupon",
			items:        []Item{{"1", 1}},
			coupon:       "HAPPYHRS",
			products:     &fakeProducts{byID: catalog},
			coupons:      &fakeCoupons{valid: map[string]bool{"HAPPYHRS": true}},
			store:        &fakeStore{},
			wantItems:    []Item{{"1", 1}},
			wantProducts: []product.Product{waffle},
			wantCoupons:  []string{"HAPPYHRS"},
		},
		{
			name:        "invalid coupon",
			items:       []Item{{"1", 1}},
			coupon:      "NOPE1234",
			products:    &fakeProducts{byID: catalog},
			coupons:     &fakeCoupons{},
			store:       &fakeStore{},
			wantErrIs:   ErrInvalidCoupon,
			wantErrNot:  []error{ErrUnknownProduct},
			wantCoupons: []string{"NOPE1234"},
		},
		{
			name:        "coupon validator failure is not an invalid coupon",
			items:       []Item{{"1", 1}},
			coupon:      "HAPPYHRS",
			products:    &fakeProducts{byID: catalog},
			coupons:     &fakeCoupons{err: errInfra},
			store:       &fakeStore{},
			wantErrIs:   errInfra,
			wantErrNot:  []error{ErrInvalidCoupon, ErrUnknownProduct},
			wantCoupons: []string{"HAPPYHRS"},
		},
		{
			name:       "unknown product",
			items:      []Item{{"1", 1}, {"99", 1}},
			products:   &fakeProducts{byID: catalog},
			coupons:    &fakeCoupons{},
			store:      &fakeStore{},
			wantErrIs:  ErrUnknownProduct,
			wantErrNot: []error{ErrInvalidCoupon},
		},
		{
			name:        "unknown product is reported before the coupon is checked",
			items:       []Item{{"99", 1}},
			coupon:      "NOPE1234",
			products:    &fakeProducts{byID: catalog},
			coupons:     &fakeCoupons{},
			store:       &fakeStore{},
			wantErrIs:   ErrUnknownProduct,
			wantCoupons: nil,
		},
		{
			name:       "product lookup failure is not an unknown product",
			items:      []Item{{"1", 1}},
			products:   &fakeProducts{err: errInfra},
			coupons:    &fakeCoupons{},
			store:      &fakeStore{},
			wantErrIs:  errInfra,
			wantErrNot: []error{ErrUnknownProduct},
		},
		{
			name:      "save failure",
			items:     []Item{{"1", 1}},
			products:  &fakeProducts{byID: catalog},
			coupons:   &fakeCoupons{},
			store:     &fakeStore{err: errInfra},
			wantErrIs: errInfra,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(tt.store, tt.products, tt.coupons)
			got, err := svc.Place(context.Background(), tt.items, tt.coupon)

			if !slices.Equal(tt.coupons.calls, tt.wantCoupons) {
				t.Errorf("validator called with %v, want %v", tt.coupons.calls, tt.wantCoupons)
			}
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("err = %v, want %v", err, tt.wantErrIs)
				}
				for _, not := range tt.wantErrNot {
					if errors.Is(err, not) {
						t.Errorf("err = %v must not match %v", err, not)
					}
				}
				if len(tt.store.saved) != 0 {
					t.Error("order saved despite error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !uuidV4.MatchString(got.ID) {
				t.Errorf("ID %q is not a UUIDv4", got.ID)
			}
			if !slices.Equal(got.Items, tt.wantItems) {
				t.Errorf("items = %v, want %v", got.Items, tt.wantItems)
			}
			if !slices.Equal(got.Products, tt.wantProducts) {
				t.Errorf("products = %v, want %v", got.Products, tt.wantProducts)
			}
			if got.CouponCode != tt.coupon {
				t.Errorf("coupon = %q, want %q", got.CouponCode, tt.coupon)
			}
			if len(tt.store.saved) != 1 || tt.store.saved[0].ID != got.ID {
				t.Errorf("saved %v, want exactly the returned order", tt.store.saved)
			}
		})
	}
}

func TestNewUUIDv4Unique(t *testing.T) {
	seen := make(map[string]bool)
	for range 1000 {
		id, err := newUUIDv4()
		if err != nil {
			t.Fatal(err)
		}
		if !uuidV4.MatchString(id) {
			t.Fatalf("%q is not a UUIDv4", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestMemoryStore(t *testing.T) {
	s := NewMemoryStore()
	o := Order{ID: "a", Items: []Item{{"1", 1}}}
	if err := s.Save(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	o.Items[0].Quantity = 99 // caller mutates after saving
	if got := s.orders["a"].Items[0].Quantity; got != 1 {
		t.Errorf("stored order changed to quantity %d; Save must copy", got)
	}
	if err := s.Save(context.Background(), Order{ID: "a"}); err == nil {
		t.Error("saving a duplicate ID succeeded")
	}
}
