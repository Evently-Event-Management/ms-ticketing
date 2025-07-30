package kafka

import (
	"ms-ticketing/internal/order/db"
)

type Producer struct {
	// you’ll need Kafka client here
}

func (p *Producer) PublishOrderCreated(o db.Order) error {
	// serialize and send to Kafka topic
	return nil
}
