package payment

import "time"

type CreatePaymentRequest struct {
	PaymentMethod string `json:"payment_method" validate:"omitempty,oneof=BANK_TRANSFER VIRTUAL_ACCOUNT E_WALLET"`
}

// CallbackRequest — simulasi payload gateway eksternal (spec docs/
// api-contract.md Section 8). Endpoint ini TIDAK pakai JWT, validasi
// keabsahan sepenuhnya lewat transaction_id yang harus cocok dengan
// order_id yang terdaftar (lihat Service.Callback).
type CallbackRequest struct {
	TransactionID string `json:"transaction_id" validate:"required"`
	OrderID       string `json:"order_id" validate:"required,uuid"`
	Status        string `json:"status" validate:"required"`
}

type Response struct {
	PaymentID     string `json:"payment_id"`
	OrderID       string `json:"order_id"`
	TransactionID string `json:"transaction_id"`
	PaymentNumber string `json:"payment_number"`
	Amount        string `json:"amount"`
	Status        string `json:"status"`
	PaymentMethod string `json:"payment_method"`
	ExpiredAt     string `json:"expired_at"`
}

func toResponse(p *Payment) Response {
	return Response{
		PaymentID:     p.ID,
		OrderID:       p.OrderID,
		TransactionID: p.TransactionID,
		PaymentNumber: p.PaymentNumber,
		Amount:        p.Amount.StringFixed(2),
		Status:        string(p.Status),
		PaymentMethod: string(p.PaymentMethod),
		ExpiredAt:     p.ExpiredAt.Format(time.RFC3339),
	}
}
