package warehouse

type CreateRequest struct {
	Code    string `json:"code" validate:"required"`
	Name    string `json:"name" validate:"required"`
	Address string `json:"address" validate:"required"`
	City    string `json:"city" validate:"required"`
}

type UpdateRequest struct {
	Name    *string `json:"name"`
	Address *string `json:"address"`
	City    *string `json:"city"`
	Status  *string `json:"status" validate:"omitempty,oneof=ACTIVE INACTIVE"`
}

type Response struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	Address string `json:"address"`
	City    string `json:"city"`
	Status  string `json:"status"`
}

func toResponse(w *Warehouse) Response {
	return Response{ID: w.ID, Code: w.Code, Name: w.Name, Address: w.Address, City: w.City, Status: string(w.Status)}
}
