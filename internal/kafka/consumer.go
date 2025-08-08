package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"ms-ticketing/internal/models"

	"github.com/segmentio/kafka-go"
)

type Consumer struct {
	reader *kafka.Reader
}

// NewConsumer creates a new Kafka consumer for the given topic and group
func NewConsumer(brokers []string, topic, groupID string) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
	})
	return &Consumer{reader: reader}
}

// Start begins consuming messages from Kafka
func (c *Consumer) Start(handler func(order models.Order)) {
	fmt.Println("🔄 Kafka consumer started...")

	for {
		msg, err := c.reader.ReadMessage(context.Background())
		if err != nil {
			log.Printf("❌ Error reading message: %v\n", err)
			continue
		}

		var order models.Order
		if err := json.Unmarshal(msg.Value, &order); err != nil {
			log.Printf("⚠️ Failed to unmarshal message: %v\n", err)
			continue
		}

		log.Printf("📩 Received order event: ID=%s", order.ID)
		handler(order)
	}
}

// Close gracefully shuts down the Kafka reader
func (c *Consumer) Close() error {
	return c.reader.Close()
}
