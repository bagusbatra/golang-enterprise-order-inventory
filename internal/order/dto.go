package order

type ItemInput struct {
	ProductID string `json:"product_id" validate:"required,uuid"`
	Quantity  int    `json:"quantity" validate:"required,gt=0"`
}

type CreateOrderRequest struct {
	WarehouseID string      `json:"warehouse_id" validate:"required,uuid"`
	Items       []ItemInput `json:"items" validate:"required,min=1,dive"`
}

type Response struct {
	ID           string `json:"id"`
	OrderNumber  string `json:"order_number"`
	CustomerID   string `json:"customer_id"`
	WarehouseID  string `json:"warehouse_id"`
	Status       string `json:"status"`
	Subtotal     string `json:"subtotal"`
	Discount     string `json:"discount"`
	Tax          string `json:"tax"`
	ShippingCost string `json:"shipping_cost"`
	GrandTotal   string `json:"grand_total"`
}

type ItemResponse struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	UnitPrice string `json:"unit_price"`
	Subtotal  string `json:"subtotal"`
}

type DetailResponse struct {
	Response
	Items []ItemResponse `json:"items"`
}

// ToResponse adalah wrapper exported dari toResponse — dipakai domain lain
// milik Agent 3 (Shipment, Iterasi 09) yang mengubah status order lewat
// method passthrough (LockForUpdate/UpdateStatus) dan perlu mengembalikan
// representasi order yang konsisten dengan endpoint /orders lainnya.
func ToResponse(o *Order) Response {
	return toResponse(o)
}

func toResponse(o *Order) Response {
	return Response{
		ID:           o.ID,
		OrderNumber:  o.OrderNumber,
		CustomerID:   o.CustomerID,
		WarehouseID:  o.WarehouseID,
		Status:       string(o.Status),
		Subtotal:     o.Subtotal.StringFixed(2),
		Discount:     o.Discount.StringFixed(2),
		Tax:          o.Tax.StringFixed(2),
		ShippingCost: o.ShippingCost.StringFixed(2),
		GrandTotal:   o.GrandTotal.StringFixed(2),
	}
}

func toDetailResponse(o *Order) DetailResponse {
	items := make([]ItemResponse, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, ItemResponse{
			ProductID: it.ProductID,
			Quantity:  it.Quantity,
			UnitPrice: it.UnitPrice.StringFixed(2),
			Subtotal:  it.Subtotal.StringFixed(2),
		})
	}
	return DetailResponse{Response: toResponse(o), Items: items}
}
