package order

import (
	"errors"
	"fmt"

	"shop/internal/product"
)

// PlaceOrderRequest is the OrderReq schema in api/openapi.yaml.
type PlaceOrderRequest struct {
	CouponCode string        `json:"couponCode"`
	Items      []ItemRequest `json:"items"`
}

type ItemRequest struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

// Validate checks the required fields and quantity bounds, reporting every
// problem at once. A missing quantity decodes as 0 and fails the bound check.
func (r PlaceOrderRequest) Validate() error {
	if len(r.Items) == 0 {
		return errors.New("items must not be empty")
	}
	var errs []error
	for i, it := range r.Items {
		if it.ProductID == "" {
			errs = append(errs, fmt.Errorf("items[%d].productId is required", i))
		}
		if it.Quantity < MinQuantity || it.Quantity > MaxQuantity {
			errs = append(errs, fmt.Errorf("items[%d].quantity must be between %d and %d", i, MinQuantity, MaxQuantity))
		}
	}
	return errors.Join(errs...)
}

func (r PlaceOrderRequest) toItems() []Item {
	items := make([]Item, len(r.Items))
	for i, it := range r.Items {
		items[i] = Item(it)
	}
	return items
}

// OrderResponse is the Order schema in api/openapi.yaml.
type OrderResponse struct {
	ID       string                    `json:"id"`
	Items    []ItemRequest             `json:"items"`
	Products []product.ProductResponse `json:"products"`
}

func toOrderResponse(o Order) OrderResponse {
	resp := OrderResponse{
		ID:       o.ID,
		Items:    make([]ItemRequest, len(o.Items)),
		Products: make([]product.ProductResponse, len(o.Products)),
	}
	for i, it := range o.Items {
		resp.Items[i] = ItemRequest(it)
	}
	for i, p := range o.Products {
		resp.Products[i] = product.ToResponse(p)
	}
	return resp
}
