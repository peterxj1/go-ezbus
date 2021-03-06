package ezbus

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/zapote/go-ezbus/logging"

	"github.com/zapote/go-ezbus/headers"
)

type subscription struct {
	endpoint    string
	messageName string
}

type subscriptions []subscription

//Bus for publishing, sending and receiving messages
type Bus interface {
	StarterStopper
	Sender
	Publisher
	Subscriber
	EnableLogger
}

// EnableLogger interface
type EnableLogger interface {
	EnableLog()
}

// Sender interface
type Sender interface {
	Send(dst string, msg interface{}) error
}

// Publisher interface
type Publisher interface {
	Publish(msg interface{}) error
}

//Subscriber interface
type Subscriber interface {
	Subscribe(endpoint string)
	SubscribeMessage(endpoint string, messageName string)
}

//StarterStopper interface
type StarterStopper interface {
	Go() error
	Stop() error
}

type bus struct {
	broker      Broker
	router      Router
	subscribers []subscription
	logger      *logging.Service
}

// NewBus creates a bus instance for sending and receiving messages.
func NewBus(b Broker, r Router) Bus {
	bus := bus{
		broker:      b,
		router:      r,
		subscribers: make([]subscription, 0),
		logger:      &logging.Service{},
	}

	return &bus
}

func (b *bus) EnableLog() {
	b.logger.Enable()
}

//Go starts the bus and listens to incoming messages.
func (b *bus) Go() error {
	err := b.broker.Start(b.handle)
	if err != nil {
		return err
	}
	for _, s := range b.subscribers {
		err = b.broker.Subscribe(s.endpoint, s.messageName)
		if err != nil {
			return err
		}
	}

	b.logger.Log("Bus is on the Go!")

	return nil
}

//Stop the bus and any incoming messages.
func (b *bus) Stop() error {
	b.logger.Log("Bus stopped.")

	return b.broker.Stop()
}

// Send message to destination.
func (b *bus) Send(dst string, msg interface{}) error {
	json, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	t := reflect.TypeOf(msg)
	return b.broker.Send(dst, NewMessage(b.getHeaders(t, dst), json))
}

//Publish message to subscribers
func (b *bus) Publish(msg interface{}) error {
	json, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	t := reflect.TypeOf(msg)
	h := b.getHeaders(t, "")

	return b.broker.Publish(NewMessage(h, json))
}

//SubscribeMessage to a specific message from a publisher. Provide endpoint (queue) and name of the message to subscribe to.
func (b *bus) SubscribeMessage(endpoint string, messageName string) {
	b.logger.Logf("Subscribing to message '%s' from endpoint '%s'", messageName, endpoint)

	b.subscribers = append(b.subscribers, subscription{endpoint, messageName})
}

//Subscribe to all messages from a publisher. Provide endpoint (queue).
func (b *bus) Subscribe(endpoint string) {
	b.logger.Logf("Subscribing to all messages from endpoint '%s'", endpoint)

	b.subscribers = append(b.subscribers, subscription{endpoint, ""})
}

func (b *bus) handle(m Message) (err error) {
	n := m.Headers[headers.MessageName]
	err = receive(func() error {
		return b.router.Receive(n, m)
	}, 5)

	if err == nil {
		return nil
	}

	if _, ok := err.(HandlerNotFoundErr); ok {
		b.logger.Logf("Message will be discarded: %s", err.Error())

		return nil
	}

	eq := fmt.Sprintf("%s-error", b.broker.Endpoint())
	m.Headers[headers.Error] = err.Error()

	b.logger.Logf("Failed to handle message. Putting on error queue: %s\n", eq)

	return b.broker.Send(eq, m)
}

func (b *bus) getHeaders(msgType reflect.Type, dst string) map[string]string {
	h := make(map[string]string)
	h[headers.MessageName] = msgType.Name()
	h[headers.MessageFullname] = msgType.String()
	h[headers.TimeSent] = time.Now().Format("2006-01-02 15:04:05.000000")

	if dst != "" {
		h[headers.Destination] = dst
	}

	n, err := os.Hostname()
	if err == nil {
		h[headers.SendingHost] = n
	}

	b.logger.Logf("Failed to get hostname: %v", err)

	return h
}
