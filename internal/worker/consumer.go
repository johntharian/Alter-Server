package worker

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/john/botsapp/internal/queue"
	redisclient "github.com/john/botsapp/internal/redis"
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

	log.Println("[Worker] Delivery worker started, waiting for messages...")

	for {
		select {
		case <-ctx.Done():
			log.Println("[Worker] Shutting down...")
			return nil
		case delivery, ok := <-msgs:
			if !ok {
				log.Println("[Worker] Channel closed, exiting")
				return nil
			}

			shouldAck, err := c.deliverer.Deliver(ctx, delivery.Body)
			if err != nil {
				log.Printf("[Worker] Delivery error: %v", err)
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
