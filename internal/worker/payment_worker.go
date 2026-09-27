package worker

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// NewPaymentEventConsumer membuat StreamConsumer untuk `payment_events`
// (spec Section 45-46). Sama seperti order_worker.go — handler default
// hanya logging; konsumen NYATA untuk PAYMENT_PAID (notification, Iterasi
// 08) menggantikan handler ini lewat wiring di cmd/server/main.go.
func NewPaymentEventConsumer(client *redis.Client, logger *zap.Logger, handler EventHandler) *StreamConsumer {
	if handler == nil {
		handler = func(ctx context.Context, event Event) error {
			logger.Info("payment event received",
				zap.String("event_type", event.EventType),
				zap.String("aggregate_id", event.AggregateID))
			return nil
		}
	}
	return NewStreamConsumer(client, logger, ConsumerConfig{
		Stream:       "payment_events",
		Group:        "payment-workers",
		ConsumerName: "payment-worker",
	}, handler)
}
