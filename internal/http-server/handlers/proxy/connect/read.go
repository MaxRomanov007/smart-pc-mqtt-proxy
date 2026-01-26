package connect

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"smart-pc-mqtt-proxy/internal/lib/logger/sl"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
)

type StartReadConfig struct {
	Topic            string
	MQQTClient       mqtt.Client
	WSConn           *websocket.Conn
	SubscribeTimeout time.Duration
	WriteTimeout     time.Duration
}

type ReadMessage struct {
	Duplicate bool   `json:"duplicate"`
	Qos       byte   `json:"qos"`
	Retained  bool   `json:"retained"`
	Topic     string `json:"Topic"`
	MessageID uint16 `json:"message_id"`
	Payload   any    `json:"payload"`
}

// startRead subscribes on Topic and send messages to websocket connection
func startRead(
	ctx context.Context,
	log *slog.Logger,
	cfg *StartReadConfig,
) error {
	const op = "handlers.proxy.connect.read.startRead"

	// buffer
	msgChan := make(chan []byte, 100)
	defer close(msgChan)

	// subscribe on Topic
	token := cfg.MQQTClient.Subscribe(cfg.Topic, 1, readMessage(log, msgChan))
	if !token.WaitTimeout(cfg.SubscribeTimeout) {
		return fmt.Errorf("%s: mqtt subscribe timeout", op)
	}
	if token.Error() != nil {
		return fmt.Errorf("%s: mqtt subscribe error: %w", op, token.Error())
	}
	defer cfg.MQQTClient.Unsubscribe(cfg.Topic)

	// send messages to websocket
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-msgChan:
			_ = cfg.WSConn.SetWriteDeadline(time.Now().Add(cfg.WriteTimeout))
			if err := cfg.WSConn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return fmt.Errorf("%s: failed to write to websocket: %w", op, err)
			}
		}
	}
}

func readMessage(log *slog.Logger, msgChan chan<- []byte) mqtt.MessageHandler {
	return func(client mqtt.Client, message mqtt.Message) {
		const op = "handlers.proxy.connect.read.readMessage"

		log := log.With(sl.Op(op))

		var payload any
		if err := json.Unmarshal(message.Payload(), &payload); err != nil {
			log.Error("failed to unmarshal received message payload")
			return
		}

		messageToSend := ReadMessage{
			Topic:     message.Topic(),
			Qos:       message.Qos(),
			Retained:  message.Retained(),
			Duplicate: message.Duplicate(),
			Payload:   payload,
			MessageID: message.MessageID(),
		}

		log.Debug("got message from mqtt", slog.Any("message", messageToSend))

		m, err := json.Marshal(messageToSend)
		if err != nil {
			log.Error("failed to marshal output message")
			return
		}

		select {
		case msgChan <- m:
		default:
			log.Warn("failed to send message to MQTT client: buffer is full")
		}
	}
}
