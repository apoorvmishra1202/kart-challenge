// Package product serves the product catalogue: GET /api/product and
// GET /api/product/{productId}.
package product

import "errors"

// Product is a catalogue item. Price is in cents to avoid floating-point
// rounding; the HTTP layer converts it to the spec's float.
type Product struct {
	ID       string
	Name     string
	Price    int64
	Category string
}

var ErrNotFound = errors.New("product not found")
