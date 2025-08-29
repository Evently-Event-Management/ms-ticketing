package kafka

import (
	"context"
	"fmt"
	"sync"

	"github.com/segmentio/kafka-go"
)

type Producer struct {
	Writers map[string]*kafka.Writer
	Brokers []string
	mu      sync.Mutex
}

func NewProducer(brokers []string) *Producer {
	// Convert kafka:9092 to kafka:29092 for containerized environments
	adjustedBrokers := make([]string, len(brokers))
	for i, broker := range brokers {
		if broker == "kafka:9092" {
			fmt.Printf("Converting Kafka broker address from kafka:9092 to kafka:29092 for internal container communication\n")
			adjustedBrokers[i] = "kafka:29092"
		} else {
			adjustedBrokers[i] = broker
		}
	}

	return &Producer{
		Writers: map[string]*kafka.Writer{
			"ticketly.order.created": kafka.NewWriter(kafka.WriterConfig{
				Brokers: adjustedBrokers,
				Topic:   "ticketly.order.created",
			}),
			"ticketly.order.updated": kafka.NewWriter(kafka.WriterConfig{
				Brokers: adjustedBrokers,
				Topic:   "ticketly.order.updated",
			}),
			"ticketly.order.canceled": kafka.NewWriter(kafka.WriterConfig{
				Brokers: adjustedBrokers,
				Topic:   "ticketly.order.canceled",
			}),
			"ticketly.seats.locked": kafka.NewWriter(kafka.WriterConfig{
				Brokers: adjustedBrokers,
				Topic:   "ticketly.seats.locked",
			}),
			"ticketly.seats.released": kafka.NewWriter(kafka.WriterConfig{
				Brokers: adjustedBrokers,
				Topic:   "ticketly.seats.released",
			}),
		},
		Brokers: adjustedBrokers,
	}
}

func (p *Producer) getOrCreateWriter(topic string) (*kafka.Writer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if writer exists
	if writer, exists := p.Writers[topic]; exists {
		return writer, nil
	}

	// Create topic if it doesn't exist
	if err := CreateTopicIfNotExists(p.Brokers, topic); err != nil {
		return nil, fmt.Errorf("failed to create topic %s: %w", topic, err)
	}

	// Create new writer
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers: p.Brokers,
		Topic:   topic,
	})

	// Store for future use
	p.Writers[topic] = writer
	return writer, nil
}

func (p *Producer) Publish(topic string, key string, value []byte) error {
	writer, err := p.getOrCreateWriter(topic)
	if err != nil {
		return fmt.Errorf("failed to get writer for topic %s: %w", topic, err)
	}

	// Add debug logging for the Kafka message
	fmt.Printf("Publishing to Kafka topic: %s, key: %s, value length: %d bytes\n",
		topic, key, len(value))

	return writer.WriteMessages(context.Background(),
		kafka.Message{Key: []byte(key), Value: value},
	)
}

func (p *Producer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for topic, w := range p.Writers {
		if err := w.Close(); err != nil {
			return fmt.Errorf("failed to close writer for topic %s: %w", topic, err)
		}
	}
	return nil
}
