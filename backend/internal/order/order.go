// Package order places orders: POST /api/order. It may import product for its
// types; product and coupon never import order.
package order

import (
	"errors"

	"shop/internal/product"
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

var (
	ErrUnknownProduct = errors.New("unknown product")
	ErrInvalidCoupon  = errors.New("invalid coupon")
)
