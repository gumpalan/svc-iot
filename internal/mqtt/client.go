package mqtt

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

type Config struct {
	Broker   string
	ClientID string
	Username string
	Password string
}

func LoadConfig() Config {
	broker := os.Getenv("MQTT_BROKER")
	if broker == "" {
		broker = "tcp://127.0.0.1:1883"
	}
	if !strings.HasPrefix(broker, "tcp://") && !strings.HasPrefix(broker, "ssl://") && !strings.HasPrefix(broker, "ws://") {
		broker = "tcp://" + broker
	}

	clientID := os.Getenv("MQTT_CLIENT_ID")
	if clientID == "" {
		clientID = fmt.Sprintf("gumpalan_svc_iot_%d", time.Now().Unix())
	}

	return Config{
		Broker:   broker,
		ClientID: clientID,
		Username: os.Getenv("MQTT_USERNAME"),
		Password: os.Getenv("MQTT_PASSWORD"),
	}
}

type InboundMessageHandler func(topic string, payload []byte)

type Client struct {
	pahoClient paho.Client
	handler    InboundMessageHandler
}

func NewClient(cfg Config, handler InboundMessageHandler) (*Client, error) {
	opts := paho.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID)
	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}

	opts.SetAutoReconnect(true)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetConnectTimeout(3 * time.Second)

	c := &Client{
		handler: handler,
	}

	opts.SetDefaultPublishHandler(func(client paho.Client, msg paho.Message) {
		if c.handler != nil {
			c.handler(msg.Topic(), msg.Payload())
		}
	})

	opts.SetOnConnectHandler(func(client paho.Client) {
		log.Printf("[MQTT] Terhubung ke broker %s", cfg.Broker)
	})

	opts.SetConnectionLostHandler(func(client paho.Client, err error) {
		log.Printf("[MQTT] Koneksi terputus: %v, mencoba menyambung kembali...", err)
	})

	client := paho.NewClient(opts)
	token := client.Connect()
	if token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("connect to mqtt broker %s: %w", cfg.Broker, token.Error())
	}

	c.pahoClient = client
	return c, nil
}

func (c *Client) SubscribeTopics(topics []string) error {
	if len(topics) == 0 {
		return nil
	}

	filters := make(map[string]byte)
	for _, t := range topics {
		if strings.TrimSpace(t) != "" {
			filters[t] = 0 // QoS 0
		}
	}

	token := c.pahoClient.SubscribeMultiple(filters, func(client paho.Client, msg paho.Message) {
		if c.handler != nil {
			c.handler(msg.Topic(), msg.Payload())
		}
	})

	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("subscribe mqtt topics: %w", token.Error())
	}

	log.Printf("[MQTT] Berhasil subscribe ke %d topik", len(filters))
	return nil
}

func (c *Client) PublishCommand(topic string, payload []byte) error {
	token := c.pahoClient.Publish(topic, 0, false, payload)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("publish command to %s: %w", topic, token.Error())
	}
	return nil
}

func (c *Client) Close() {
	if c.pahoClient != nil && c.pahoClient.IsConnected() {
		c.pahoClient.Disconnect(250)
	}
}
