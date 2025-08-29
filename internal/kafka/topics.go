package kafka

import (
	"context"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

// EnsureTopicsExist creates Kafka topics if they don't already exist
func EnsureTopicsExist(brokers []string, topics []string) error {
	// Connect to the first broker to create topics
	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	controllerConn, err := kafka.Dial("tcp", controller.Host)
	if err != nil {
		return err
	}
	defer controllerConn.Close()

	// Create each topic
	for _, topic := range topics {
		topicConfigs := []kafka.TopicConfig{
			{
				Topic:             topic,
				NumPartitions:     1,
				ReplicationFactor: 1,
			},
		}

		err = controllerConn.CreateTopics(topicConfigs...)
		if err != nil {
			// If error contains "already exists", it's not a problem
			if err.Error() == "kafka server: topic already exists" {
				log.Printf("Topic %s already exists", topic)
				continue
			}
			log.Printf("Error creating topic %s: %v", topic, err)
			// Continue trying to create other topics even if one fails
		} else {
			log.Printf("Created topic: %s", topic)
		}
	}

	// Wait a moment for topics to be fully created
	time.Sleep(1 * time.Second)
	return nil
}

// CreateTopicIfNotExists creates a single Kafka topic if it doesn't exist
func CreateTopicIfNotExists(brokers []string, topic string) error {
	return EnsureTopicsExist(brokers, []string{topic})
}

// ListTopics returns a list of all existing topics
func ListTopics(brokers []string) ([]string, error) {
	// Create a new reader config with a direct connection to get the list of topics
	ctx := context.Background()
	conn, err := kafka.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	partitions, err := conn.ReadPartitions()
	if err != nil {
		return nil, err
	}

	// Track unique topics
	topicMap := make(map[string]bool)
	for _, p := range partitions {
		topicMap[p.Topic] = true
	}

	// Convert map to slice
	var topics []string
	for topic := range topicMap {
		topics = append(topics, topic)
	}

	return topics, nil
}
