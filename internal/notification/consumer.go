package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"order-management/internal/worker"
)

type paymentPaidPayload struct {
	OrderID     string `json:"order_id"`
	OrderNumber string `json:"order_number"`
	CustomerID  string `json:"customer_id"`
}

type orderShippedPayload struct {
	OrderID    string `json:"order_id"`
	CustomerID string `json:"customer_id"`
}

// HandlePaymentEvent adalah worker.EventHandler untuk stream payment_events
// (spec Section 49) — hanya bereaksi ke PAYMENT_PAID; event lain di-ACK
// tanpa efek (return nil, BUKAN error) supaya tidak keliru dianggap gagal
// dan masuk dead letter.
func (s *Service) HandlePaymentEvent(ctx context.Context, event worker.Event) error {
	if event.EventType != "PAYMENT_PAID" {
		return nil
	}

	var p paymentPaidPayload
	if err := json.Unmarshal(event.Payload, &p); err != nil {
		return err
	}

	message := fmt.Sprintf("Your payment for order %s has been confirmed.", p.OrderNumber)
	return s.CreateIfNotExists(ctx, p.CustomerID, "PAYMENT_PAID", "Payment Successful", message)
}

// HandleOrderEvent adalah worker.EventHandler untuk stream order_events —
// hanya bereaksi ke ORDER_SHIPPED (spec Section 49); ORDER_CREATED/
// ORDER_CANCELLED lewat stream yang sama TIDAK memicu notification.
func (s *Service) HandleOrderEvent(ctx context.Context, event worker.Event) error {
	if event.EventType != "ORDER_SHIPPED" {
		return nil
	}

	var p orderShippedPayload
	if err := json.Unmarshal(event.Payload, &p); err != nil {
		return err
	}

	return s.CreateIfNotExists(ctx, p.CustomerID, "ORDER_SHIPPED", "Order Shipped", "Your order has been shipped.")
}
