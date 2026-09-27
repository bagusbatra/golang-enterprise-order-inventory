package worker

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisPublisher mengimplementasikan order.EventPublisher &
// payment.EventPublisher (keduanya punya signature Publish yang identik)
// di atas PublishEvent — satu implementasi konkret dipakai lintas domain
// Agent 3 tanpa domain tersebut perlu tahu detail Redis Streams.
type RedisPublisher struct {
	client *redis.Client
	logger *zap.Logger
}

func NewRedisPublisher(client *redis.Client, logger *zap.Logger) *RedisPublisher {
	return &RedisPublisher{client: client, logger: logger}
}

func (p *RedisPublisher) Publish(ctx context.Context, stream, eventType, aggregateID string, payload any) error {
	_, err := PublishEvent(ctx, p.client, stream, eventType, aggregateID, payload)
	if err != nil {
		p.logger.Error("failed to publish event",
			zap.String("stream", stream), zap.String("event_type", eventType), zap.Error(err))
	}
	return err
}
