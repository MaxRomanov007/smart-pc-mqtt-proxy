package connect

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
)

type ReadMessage struct {
	Duplicate bool   `json:"duplicate"`
	Qos       byte   `json:"qos"`
	Retained  bool   `json:"retained"`
	Topic     string `json:"topic"`
	MessageID uint16 `json:"message_id"`
	Payload   any    `json:"payload"`
}

// startRead subscribes on topic and send messages to websocket connection
func startRead(
	ctx context.Context,
	subscribeTimeout time.Duration,
	writeTimeout time.Duration,
	wsConn *websocket.Conn,
	mqttClient mqtt.Client,
	topic string,
) error {
	const op = "handlers.proxy.connect.startRead"

	// buffer
	msgChan := make(chan []byte, 100)
	defer close(msgChan)

	// mqtt messages handler
	handler := func(client mqtt.Client, message mqtt.Message) {
		var payload any
		if err := json.Unmarshal(message.Payload(), &payload); err != nil {
			return
		}

		m, _ := json.Marshal(ReadMessage{
			Topic:     message.Topic(),
			Qos:       message.Qos(),
			Retained:  message.Retained(),
			Duplicate: message.Duplicate(),
			Payload:   payload,
			MessageID: message.MessageID(),
		})

		select {
		case msgChan <- m:
		default:
			// buffer is full
		}
	}

	// subscribe on topic
	token := mqttClient.Subscribe(topic, 1, handler)
	if !token.WaitTimeout(subscribeTimeout) {
		return fmt.Errorf("%s: mqtt subscribe timeout", op)
	}
	if token.Error() != nil {
		return fmt.Errorf("%s: mqtt subscribe error: %w", op, token.Error())
	}
	defer mqttClient.Unsubscribe(topic)

	// send messages to websocket
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-msgChan:
			wsConn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := wsConn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return fmt.Errorf("%s: failed to write to websocket: %w", op, err)
			}
		}
	}
}
