package product

// SeedProducts returns the sample catalogue served by the API. Each call
// returns a fresh slice.
//
// TODO: replace with real catalogue source
func SeedProducts() []Product {
	return []Product{
		{ID: "1", Name: "Waffle with Berries", Price: 650, Category: "Waffle"},
		{ID: "2", Name: "Vanilla Bean Crème Brûlée", Price: 700, Category: "Crème Brûlée"},
		{ID: "3", Name: "Macaron Mix of Five", Price: 800, Category: "Macaron"},
		{ID: "4", Name: "Classic Tiramisu", Price: 550, Category: "Tiramisu"},
		{ID: "5", Name: "Pistachio Baklava", Price: 400, Category: "Baklava"},
		{ID: "6", Name: "Lemon Meringue Pie", Price: 500, Category: "Pie"},
		{ID: "7", Name: "Red Velvet Cake", Price: 450, Category: "Cake"},
		{ID: "8", Name: "Salted Caramel Brownie", Price: 550, Category: "Brownie"},
		{ID: "9", Name: "Vanilla Panna Cotta", Price: 650, Category: "Panna Cotta"},
	}
}
