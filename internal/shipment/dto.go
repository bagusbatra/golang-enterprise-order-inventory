package shipment

import "time"

// ShipRequest.Courier opsional, default "Simulated Courier" (Locked
// Decision 03-BACKEND-CORE.md Section 3).
type ShipRequest struct {
	Courier string `json:"courier"`
}

type Response struct {
	ID             string  `json:"id"`
	OrderID        string  `json:"order_id"`
	Courier        string  `json:"courier"`
	TrackingNumber string  `json:"tracking_number"`
	Status         string  `json:"status"`
	ShippedAt      *string `json:"shipped_at,omitempty"`
	DeliveredAt    *string `json:"delivered_at,omitempty"`
}

func toResponse(s *Shipment) Response {
	resp := Response{
		ID:             s.ID,
		OrderID:        s.OrderID,
		Courier:        s.Courier,
		TrackingNumber: s.TrackingNumber,
		Status:         string(s.Status),
	}
	if s.ShippedAt != nil {
		t := s.ShippedAt.Format(time.RFC3339)
		resp.ShippedAt = &t
	}
	if s.DeliveredAt != nil {
		t := s.DeliveredAt.Format(time.RFC3339)
		resp.DeliveredAt = &t
	}
	return resp
}
