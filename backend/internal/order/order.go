// Package order places orders: POST /api/order. It may import product for its
// types; product and coupon never import order.
package order

import (
	"errors"
	"fmt"

	"shop/internal/product"
)

// Quantity bounds, per request line and per product after merging duplicates.
const (
	MinQuantity = 1
	MaxQuantity = 100
)

type Item struct {
	ProductID string
	Quantity  int
}

// Order is a placed order. Items has one entry per product (duplicates in the
// request are merged); Products lists those products in the same order.
type Order struct {
	ID         string
	Items      []Item
	Products   []product.Product
	CouponCode string
}

// Client errors: the order can't be placed as requested (422).
var (
	ErrUnknownProduct = errors.New("unknown product")
	ErrInvalidCoupon  = errors.New("invalid coupon")
	ErrQuantityLimit  = fmt.Errorf("total quantity per product must not exceed %d", MaxQuantity)
)

// InputError ties a client error to the value that caused it. Err is one of
// the sentinels above; Value is the offending productId or coupon code.
// errors.Is matches Err through Unwrap.
type InputError struct {
	Err   error
	Value string
}

func (e *InputError) Error() string { return fmt.Sprintf("%v: %q", e.Err, e.Value) }
func (e *InputError) Unwrap() error { return e.Err }
