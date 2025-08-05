package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/segmentio/kafka-go"
	"ms-ticketing/internal/order/db"
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
func (p *Producer) PublishOrderCreated(order db.Order) error {
	msgBytes, err := json.Marshal(order)
	if err != nil {
		return err
	}

	fmt.Printf("Publishing to Kafka [order_created]: %s\n", string(msgBytes))

	return p.Writer.WriteMessages(context.Background(),
		kafka.Message{
			Key:   []byte(order.ID),
			Value: msgBytes,
		},
	)
}

// PublishOrderUpdated streams the order update event to Kafka
func (p *Producer) PublishOrderUpdated(order db.Order) error {
	msgBytes, err := json.Marshal(order)
	if err != nil {
		return err
	}

	fmt.Printf("Publishing to Kafka [order_updated]: %s\n", string(msgBytes))

	return p.Writer.WriteMessages(context.Background(),
		kafka.Message{
			Key:   []byte(order.ID),
			Value: msgBytes,
		},
	)
}

// PublishOrderCancelled streams the order cancellation event to Kafka
func (p *Producer) PublishOrderCancelled(order db.Order) error {
	msgBytes, err := json.Marshal(order)
	if err != nil {
		return err
	}

	fmt.Printf("Publishing to Kafka [order_cancelled]: %s\n", string(msgBytes))

	return p.Writer.WriteMessages(context.Background(),
		kafka.Message{
			Key:   []byte(order.ID),
			Value: msgBytes,
		},
	)
}
