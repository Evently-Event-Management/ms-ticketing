package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/models"

	"github.com/segmentio/kafka-go"
)

type Producer struct {
	Writer *kafka.Writer
}

func NewProducer(brokers []string, topic string) *Producer {
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers: brokers,
		Topic:   topic,
	})
	return &Producer{Writer: writer}
}

// PublishOrderCreated streams the order creation event to Kafka
func (p *Producer) PublishOrderCreated(order models.Order) error {
	msgBytes, err := json.Marshal(order)
	if err != nil {
		return err
	}

	fmt.Printf("Publishing to Kafka [order_created]: %s\n", string(msgBytes))

	return p.Writer.WriteMessages(context.Background(),
		kafka.Message{
			Key:   []byte(order.OrderID),
			Value: msgBytes,
		},
	)
}

// PublishOrderUpdated streams the order update event to Kafka
func (p *Producer) PublishOrderUpdated(order models.Order) error {
	msgBytes, err := json.Marshal(order)
	if err != nil {
		return err
	}

	fmt.Printf("Publishing to Kafka [order_updated]: %s\n", string(msgBytes))

	return p.Writer.WriteMessages(context.Background(),
		kafka.Message{
			Key:   []byte(order.OrderID),
			Value: msgBytes,
		},
	)
}

// PublishOrderCancelled streams the order cancellation event to Kafka
func (p *Producer) PublishOrderCancelled(order models.Order) error {
	msgBytes, err := json.Marshal(order)
	if err != nil {
		return err
	}

	fmt.Printf("Publishing to Kafka [order_cancelled]: %s\n", string(msgBytes))

	return p.Writer.WriteMessages(context.Background(),
		kafka.Message{
			Key:   []byte(order.OrderID),
			Value: msgBytes,
		},
	)
}
