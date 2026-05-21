package events

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/segmentio/kafka-go"
)

type kafkaWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

type KafkaProducer struct {
	topic  string
	writer kafkaWriter
}

type KafkaProducerConfig struct {
	Brokers []string
	Topic   string
}

func NewKafkaProducer(cfg KafkaProducerConfig) (*KafkaProducer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, errors.New("kafka brokers are required")
	}
	if cfg.Topic == "" {
		return nil, errors.New("kafka topic is required")
	}
	writer := &kafka.Writer{
		Addr:     kafka.TCP(cfg.Brokers...),
		Topic:    cfg.Topic,
		Balancer: &kafka.Hash{},
	}
	return &KafkaProducer{topic: cfg.Topic, writer: writer}, nil
}

func (p *KafkaProducer) Publish(ctx context.Context, event UsageEvent) error {
	if p == nil || p.writer == nil {
		return nil
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(event.RequestID.String()),
		Value: body,
	})
}

func (p *KafkaProducer) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
