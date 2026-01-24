package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
)

type WriteMessage struct {
	Retained bool `json:"retained"`
	QOS      byte `json:"qos"`
	Payload  any  `json:"payload"`
}

// startWrite listen websocket connection and send messages to mqtt topic
func startWrite(
	ctx context.Context,
	publishTimeout time.Duration,
	wsConn *websocket.Conn,
	mqttClient mqtt.Client,
	topic string,
) error {
	const op = "handlers.proxy.connect.startWrite"

	// buffer
	msgChan := make(chan *WriteMessage, 10)
	errChan := make(chan error, 1)

	// goroutine for reading websocket connection
	go func() {
		defer close(msgChan)
		for {
			msg := &WriteMessage{}
			err := wsConn.ReadJSON(msg)
			if err != nil {
				if errors.Is(err, websocket.ErrCloseSent) ||
					websocket.IsCloseError(err, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) ||
					strings.Contains(err.Error(), "closed network connection") {
					return
				}
				switch err.(type) {
				case *json.SyntaxError, *json.UnmarshalTypeError:
					wsConn.WriteMessage(websocket.TextMessage, []byte("invalid json"))
					continue
				}
				if errors.Is(err, io.ErrUnexpectedEOF) {
					wsConn.WriteMessage(websocket.TextMessage, []byte(err.Error()))
					continue
				}

				select {
				case errChan <- fmt.Errorf("%s: failed to read message from web socket: %w", op, err):
				default:
				}
				return
			}
			if msg.QOS > 2 {
				wsConn.WriteMessage(websocket.TextMessage, []byte("invalid qos"))
				continue
			}

			// save message in channel or exit if context done
			select {
			case msgChan <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errChan:
			return err
		case msg := <-msgChan:
			if msg == nil {
				return fmt.Errorf("%s: %w", op, ErrConnectionStopped)
			}

			p, err := json.Marshal(msg.Payload)
			if err != nil {
				return fmt.Errorf("%s: failed to marshal message: %w", op, err)
			}

			// publish message with timeout
			token := mqttClient.Publish(topic, msg.QOS, msg.Retained, p)
			select {
			case <-ctx.Done():
				return nil
			default:
				if !token.WaitTimeout(publishTimeout) {
					return fmt.Errorf("%s: mqtt publish timeout", op)
				}
				if token.Error() != nil {
					return fmt.Errorf("%s: mqtt publish error: %w", op, token.Error())
				}
			}
		}
	}
}
