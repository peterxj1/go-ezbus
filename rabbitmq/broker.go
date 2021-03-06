package rabbitmq

import (
	"fmt"
	"log"

	"github.com/streadway/amqp"
	"github.com/zapote/go-ezbus"
	"github.com/zapote/go-ezbus/headers"
)

//Broker RabbitMQ implementation of ezbus.broker interaface.
type Broker struct {
	queueName string
	conn      *amqp.Connection
	channel   *amqp.Channel
	cfg       *config
}

//NewBroker creates a RabbitMQ broker instance
//Default url amqp://guest:guest@localhost:5672
//Default prefetchCount 100
func NewBroker(q ...string) *Broker {
	var queue string
	if len(q) > 0 {
		queue = q[0]
	}

	b := Broker{queueName: queue}
	b.cfg = &config{
		url:                "amqp://guest:guest@localhost:5672",
		prefetchCount:      100,
		queueNameDelimiter: "-",
	}
	return &b
}

//Send sends a message to given destination
func (b *Broker) Send(dst string, m ezbus.Message) error {
	err := publish(b.channel, m, dst, "")
	if err != nil {
		return fmt.Errorf("Send: %s", err)
	}
	return err
}

//Publish publishes message on exhange
func (b *Broker) Publish(m ezbus.Message) error {
	key := m.Headers[headers.MessageName]
	err := publish(b.channel, m, key, b.queueName)
	if err != nil {
		return fmt.Errorf("Publish: %s", err)
	}
	return err
}

//Start starts the RabbitMQ broker and declars queue, and exchange.
func (b *Broker) Start(handle ezbus.MessageHandler) error {
	cn, err := amqp.Dial(b.cfg.url)

	if err != nil {
		return fmt.Errorf("Dial: %s", err)
	}

	b.conn = cn
	b.channel, err = b.conn.Channel()

	if err != nil {
		return fmt.Errorf("Channel: %s", err)
	}

	if b.Endpoint() == "" {
		return nil
	}

	err = b.channel.Qos(b.cfg.prefetchCount, 0, false)
	if err != nil {
		return fmt.Errorf("Qos: %s", err)
	}

	queue, err := declareQueue(b.channel, b.queueName)
	if err != nil {
		return fmt.Errorf("Declare Queue : %s", err)
	}
	log.Printf("Queue declared. (%q %d messages, %d consumers)", queue.Name, queue.Messages, queue.Consumers)

	_, err = declareQueue(b.channel, fmt.Sprintf("%s%serror", b.queueName, b.cfg.queueNameDelimiter))
	if err != nil {
		return fmt.Errorf("Declare Error Queue : %s", err)
	}
	log.Printf("Queue declared. (%q %d messages, %d consumers)", queue.Name, queue.Messages, queue.Consumers)

	err = declareExchange(b.channel, b.queueName)
	if err != nil {
		return fmt.Errorf("Declare Exchange : %s", err)
	}
	log.Printf("Exchange declared. (%q)", b.queueName)

	msgs, err := b.channel.Consume(queue.Name, "", false, false, false, false, nil)

	if err != nil {
		return fmt.Errorf("Queue Consume: %s", err)
	}
	go func() {
		for d := range msgs {
			headers := extractHeaders(d.Headers)
			m := ezbus.Message{Headers: headers, Body: d.Body}
			handle(m)
			b.channel.Ack(d.DeliveryTag, false)
		}
	}()
	log.Print("RabbitMQ broker started")
	return nil
}

//Stop stops the RabbitMQ broker
func (b *Broker) Stop() error {
	err := b.channel.Close()
	if err != nil {
		return fmt.Errorf("Channel Close: %s", err)
	}
	err = b.conn.Close()
	if err != nil {
		return fmt.Errorf("Connection Close: %s", err)
	}

	return nil
}

//Endpoint returns name of the queue
func (b *Broker) Endpoint() string {
	return b.queueName
}

//Subscribe to messages from specific endpoint
func (b *Broker) Subscribe(endpoint string, messageName string) error {
	return queueBind(b.channel, b.Endpoint(), messageName, endpoint)
}

//Configure RabbitMQ.
func (b *Broker) Configure() Configurer {
	return b.cfg
}

func extractHeaders(h amqp.Table) map[string]string {
	headers := make(map[string]string)
	for k, v := range h {
		headers[k] = v.(string)
	}
	return headers
}
