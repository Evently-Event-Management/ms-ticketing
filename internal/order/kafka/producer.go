package kafka

import (
	"context"
	"encoding/json"
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

// PublishOrderCreated streams the order event to Kafka
func (p *Producer) PublishOrderCreated(order db.Order) error {
	msgBytes, err := json.Marshal(order)
	if err != nil {
		return err
	}
	return p.Writer.WriteMessages(context.Background(),
		kafka.Message{
			Key:   []byte(order.ID),
			Value: msgBytes,
		},
	)
}
