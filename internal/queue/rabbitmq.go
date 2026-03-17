package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/john/alter/internal/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeName    = "botsapp.messages"
	DLXExchangeName = "botsapp.messages.dlx"
	DeliverQueue    = "messages.deliver"
	DeadLetterQueue = "messages.dead"
	DeliverKey      = "deliver"
	DeadKey         = "dead"
)

type RabbitMQ struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

func Connect(url string) (*RabbitMQ, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("connect to rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}

	rmq := &RabbitMQ{conn: conn, channel: ch}

	if err := rmq.setupTopology(); err != nil {
		rmq.Close()
		return nil, fmt.Errorf("setup topology: %w", err)
	}

	logger.Info("Connected to RabbitMQ", nil)
	return rmq, nil
}

func (r *RabbitMQ) setupTopology() error {
	// Declare dead-letter exchange
	if err := r.channel.ExchangeDeclare(
		DLXExchangeName, "direct", true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("declare DLX exchange: %w", err)
	}

	// Declare main exchange
	if err := r.channel.ExchangeDeclare(
		ExchangeName, "direct", true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}

	// Declare delivery queue with DLX
	_, err := r.channel.QueueDeclare(
		DeliverQueue, true, false, false, false,
		amqp.Table{
			"x-dead-letter-exchange":    DLXExchangeName,
			"x-dead-letter-routing-key": DeadKey,
		},
	)
	if err != nil {
		return fmt.Errorf("declare deliver queue: %w", err)
	}

	// Bind delivery queue to main exchange
	if err := r.channel.QueueBind(
		DeliverQueue, DeliverKey, ExchangeName, false, nil,
	); err != nil {
		return fmt.Errorf("bind deliver queue: %w", err)
	}

	// Declare dead-letter queue
	_, err = r.channel.QueueDeclare(
		DeadLetterQueue, true, false, false, false, nil,
	)
	if err != nil {
		return fmt.Errorf("declare DLQ: %w", err)
	}

	// Bind DLQ to DLX exchange
	if err := r.channel.QueueBind(
		DeadLetterQueue, DeadKey, DLXExchangeName, false, nil,
	); err != nil {
		return fmt.Errorf("bind DLQ: %w", err)
	}

	// Set prefetch count for fair dispatch
	if err := r.channel.Qos(10, 0, false); err != nil {
		return fmt.Errorf("set qos: %w", err)
	}

	logger.Info("RabbitMQ topology configured", nil)
	return nil
}

func (r *RabbitMQ) Publish(ctx context.Context, body []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r.channel.PublishWithContext(ctx,
		ExchangeName, DeliverKey, false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
		},
	)
}

func (r *RabbitMQ) Consume() (<-chan amqp.Delivery, error) {
	return r.channel.Consume(
		DeliverQueue, "", false, false, false, false, nil,
	)
}

func (r *RabbitMQ) Close() {
	if r.channel != nil {
		r.channel.Close()
	}
	if r.conn != nil {
		r.conn.Close()
	}
}
