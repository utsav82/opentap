package queue

import (
	"github.com/nats-io/nats.go"
)

type NATSClient struct {
	Conn      *nats.Conn
	JetStream nats.JetStreamContext
}

func NewNATSClient(url, streamName string, maxBytes int64) (*NATSClient, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, err
	}
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	// Create or update stream
	_, err = js.AddStream(&nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{"events.ingest.>", "events.dlq"},
		Storage:   nats.FileStorage,
		MaxBytes:  maxBytes,
		Retention: nats.WorkQueuePolicy,
		Discard:   nats.DiscardOld,
	})
	if err != nil {
		return nil, err
	}
	return &NATSClient{Conn: nc, JetStream: js}, nil
}

func (c *NATSClient) Publish(subject string, data []byte) error {
	_, err := c.JetStream.Publish(subject, data)
	return err
}

func (c *NATSClient) Close() {
	c.Conn.Close()
}
