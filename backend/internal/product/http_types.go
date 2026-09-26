package product

// ProductResponse is the Product schema in api/openapi.yaml.
type ProductResponse struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Category string  `json:"category"`
}

// toProductResponse converts cents to the spec's float price (650 -> 6.5).
func toProductResponse(p Product) ProductResponse {
	return ProductResponse{
		ID:       p.ID,
		Name:     p.Name,
		Price:    float64(p.Price) / 100,
		Category: p.Category,
	}
}
