package order

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"shop/internal/product"
)

// ProductLookup finds products; *product.Service implements it.
type ProductLookup interface {
	Get(ctx context.Context, id string) (product.Product, error)
}

// CouponValidator checks coupon codes; *coupon.Service implements it.
// Validate reports whether code is valid; a non-nil error means the check
// itself failed.
type CouponValidator interface {
	Validate(ctx context.Context, code string) (bool, error)
}

// Store persists placed orders.
type Store interface {
	Save(ctx context.Context, o Order) error
}

type Service struct {
	store    Store
	products ProductLookup
	coupons  CouponValidator
}

func NewService(store Store, products ProductLookup, coupons CouponValidator) *Service {
	return &Service{store: store, products: products, coupons: coupons}
}

// Place merges duplicate product IDs, checks each product's total quantity,
// resolves every product, validates the coupon if one is given, and saves the
// order under a new UUIDv4 ID. When the input is at fault the error is an
// *InputError wrapping ErrQuantityLimit, ErrUnknownProduct or
// ErrInvalidCoupon. Request lines are assumed to be validated by the caller
// (see PlaceOrderRequest).
func (s *Service) Place(ctx context.Context, items []Item, couponCode string) (Order, error) {
	merged := mergeItems(items)
	for _, it := range merged {
		if it.Quantity > MaxQuantity {
			return Order{}, &InputError{Err: ErrQuantityLimit, Value: it.ProductID}
		}
	}

	products := make([]product.Product, 0, len(merged))
	for _, it := range merged {
		p, err := s.products.Get(ctx, it.ProductID)
		if errors.Is(err, product.ErrNotFound) {
			return Order{}, &InputError{Err: ErrUnknownProduct, Value: it.ProductID}
		}
		if err != nil {
			return Order{}, fmt.Errorf("look up product %q: %w", it.ProductID, err)
		}
		products = append(products, p)
	}

	if couponCode != "" {
		ok, err := s.coupons.Validate(ctx, couponCode)
		if err != nil {
			return Order{}, fmt.Errorf("validate coupon: %w", err)
		}
		if !ok {
			return Order{}, &InputError{Err: ErrInvalidCoupon, Value: couponCode}
		}
	}

	id, err := newUUIDv4()
	if err != nil {
		return Order{}, fmt.Errorf("generate order id: %w", err)
	}
	o := Order{ID: id, Items: merged, Products: products, CouponCode: couponCode}
	if err := s.store.Save(ctx, o); err != nil {
		return Order{}, fmt.Errorf("save order: %w", err)
	}
	return o, nil
}

// mergeItems sums quantities of repeated product IDs, keeping the order in
// which each ID first appears.
func mergeItems(items []Item) []Item {
	index := make(map[string]int, len(items))
	merged := make([]Item, 0, len(items))
	for _, it := range items {
		if i, ok := index[it.ProductID]; ok {
			merged[i].Quantity += it.Quantity
			continue
		}
		index[it.ProductID] = len(merged)
		merged = append(merged, it)
	}
	return merged
}

// newUUIDv4 returns a random (version 4, RFC 9562 variant) UUID.
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
