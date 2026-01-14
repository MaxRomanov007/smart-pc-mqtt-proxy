package connect

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
)

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
	msgChan := make(chan []byte, 10)
	errChan := make(chan error, 1)

	// goroutine for reading websocket connection
	go func() {
		defer close(msgChan)
		for {
			_, p, err := wsConn.ReadMessage()
			if err != nil {
				if errors.Is(err, websocket.ErrCloseSent) ||
					websocket.IsCloseError(err, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) ||
					strings.Contains(err.Error(), "closed network connection") {
					return
				}
				select {
				case errChan <- fmt.Errorf("%s: failed to read message from web socket: %w", op, err):
				default:
				}
				return
			}

			// save message in channel or exit if context done
			select {
			case msgChan <- p:
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
		case p := <-msgChan:
			if p == nil {
				return fmt.Errorf("%s: %w", op, ErrConnectionStopped)
			}

			// publish message with timeout
			token := mqttClient.Publish(topic, 0, false, string(p))
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
