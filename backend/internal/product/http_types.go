package product

// ProductResponse is the Product schema in api/openapi.yaml.
type ProductResponse struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Category string  `json:"category"`
}

// ToResponse converts a Product to the spec shape, turning cents into the
// float price (650 -> 6.5). The order package reuses it.
func ToResponse(p Product) ProductResponse {
	return ProductResponse{
		ID:       p.ID,
		Name:     p.Name,
		Price:    float64(p.Price) / 100,
		Category: p.Category,
	}
}
