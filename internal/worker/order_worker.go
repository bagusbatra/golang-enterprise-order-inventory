package worker

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// NewOrderEventConsumer membuat StreamConsumer untuk `order_events`
// (spec Section 45-46). Handler saat ini mencatat event secara terstruktur
// (observability dasar) — konsumen NYATA untuk ORDER_SHIPPED (notification,
// Iterasi 08) akan menggantikan/menambah handler ini lewat wiring di
// cmd/server/main.go, bukan mengubah file ini.
func NewOrderEventConsumer(client *redis.Client, logger *zap.Logger, handler EventHandler) *StreamConsumer {
	if handler == nil {
		handler = func(ctx context.Context, event Event) error {
			logger.Info("order event received",
				zap.String("event_type", event.EventType),
				zap.String("aggregate_id", event.AggregateID))
			return nil
		}
	}
	return NewStreamConsumer(client, logger, ConsumerConfig{
		Stream:       "order_events",
		Group:        "order-workers",
		ConsumerName: "order-worker",
	}, handler)
}
