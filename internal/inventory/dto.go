package inventory

type StockInRequest struct {
	ProductID   string `json:"product_id" validate:"required,uuid"`
	WarehouseID string `json:"warehouse_id" validate:"required,uuid"`
	Quantity    int    `json:"quantity" validate:"required,gt=0"`
	Description string `json:"description"`
}

// AdjustRequest.Quantity boleh negatif (spec Section 32) — delta yang
// diterapkan ke quantity, bukan nilai akhir. Validasi "hasil tidak boleh
// negatif" dilakukan di service (di bawah row lock), bukan di sini.
type AdjustRequest struct {
	ProductID   string `json:"product_id" validate:"required,uuid"`
	WarehouseID string `json:"warehouse_id" validate:"required,uuid"`
	Quantity    int    `json:"quantity" validate:"required"`
	Description string `json:"description" validate:"required"`
}

type Response struct {
	ID               string `json:"id"`
	ProductID        string `json:"product_id"`
	WarehouseID      string `json:"warehouse_id"`
	Quantity         int    `json:"quantity"`
	ReservedQuantity int    `json:"reserved_quantity"`
	AvailableStock   int    `json:"available_stock"`
}

func toResponse(inv *Inventory) Response {
	return Response{
		ID:               inv.ID,
		ProductID:        inv.ProductID,
		WarehouseID:      inv.WarehouseID,
		Quantity:         inv.Quantity,
		ReservedQuantity: inv.ReservedQuantity,
		AvailableStock:   inv.AvailableStock(),
	}
}

type TransactionResponse struct {
	ID            string  `json:"id"`
	InventoryID   string  `json:"inventory_id"`
	Type          string  `json:"type"`
	Quantity      int     `json:"quantity"`
	ReferenceType *string `json:"reference_type,omitempty"`
	ReferenceID   *string `json:"reference_id,omitempty"`
	Description   *string `json:"description,omitempty"`
}

func toTransactionResponse(t *Transaction) TransactionResponse {
	return TransactionResponse{
		ID:            t.ID,
		InventoryID:   t.InventoryID,
		Type:          string(t.Type),
		Quantity:      t.Quantity,
		ReferenceType: t.ReferenceType,
		ReferenceID:   t.ReferenceID,
		Description:   t.Description,
	}
}
