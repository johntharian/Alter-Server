package worker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/john/alter/internal/logger"
	"github.com/john/alter/internal/queue"
	redisclient "github.com/john/alter/internal/redis"
)

type Consumer struct {
	rmq       *queue.RabbitMQ
	deliverer *Deliverer
}

func NewConsumer(db *pgxpool.Pool, redis *redisclient.Client, rmq *queue.RabbitMQ) *Consumer {
	return &Consumer{
		rmq:       rmq,
		deliverer: NewDeliverer(db, redis),
	}
}

// Start begins consuming messages from RabbitMQ and delivering them.
func (c *Consumer) Start(ctx context.Context) error {
	msgs, err := c.rmq.Consume()
	if err != nil {
		return err
	}

	logger.Info("Delivery worker started, waiting for messages...", nil)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Shutting down...", nil)
			return nil
		case delivery, ok := <-msgs:
			if !ok {
				logger.Info("Channel closed, exiting", nil)
				return nil
			}

			shouldAck, err := c.deliverer.Deliver(ctx, delivery.Body)
			if err != nil {
				logger.Error("Delivery error", map[string]interface{}{"error": err.Error()})
			}

			if shouldAck {
				delivery.Ack(false)
			} else {
				// Nack without requeue — let DLX handle if max retries exceeded
				// For retries, we nack with requeue=true
				if err != nil {
					// Check retry count to decide
					delivery.Nack(false, true) // requeue for retry
				} else {
					delivery.Ack(false)
				}
			}
		}
	}
}
