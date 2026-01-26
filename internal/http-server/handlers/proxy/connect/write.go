package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"smart-pc-mqtt-proxy/internal/lib/logger/sl"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
)

type StartWriteConfig struct {
	Topic          string
	PublishTimeout time.Duration
	WSConn         *websocket.Conn
	MQTTClient     mqtt.Client
}

type WriteMessage struct {
	Retained bool `json:"retained"`
	QOS      byte `json:"qos"`
	Payload  any  `json:"payload"`
}

// startWrite listen websocket connection and send messages to mqtt Topic
func startWrite(
	ctx context.Context,
	log *slog.Logger,
	cfg *StartWriteConfig,
) error {
	const op = "handlers.proxy.connect.startWrite"

	// buffer
	msgChan := make(chan *WriteMessage, 10)
	errChan := make(chan error, 1)

	// goroutine for reading websocket connection
	go func() {
		defer close(msgChan)
		readFromWS(ctx, log, cfg.WSConn, msgChan, errChan)
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
			token := cfg.MQTTClient.Publish(cfg.Topic, msg.QOS, msg.Retained, p)
			select {
			case <-ctx.Done():
				return nil
			default:
				if !token.WaitTimeout(cfg.PublishTimeout) {
					return fmt.Errorf("%s: mqtt publish timeout", op)
				}
				if token.Error() != nil {
					return fmt.Errorf("%s: mqtt publish error: %w", op, token.Error())
				}
			}
		}
	}
}

func readFromWS(
	ctx context.Context,
	l *slog.Logger,
	ws *websocket.Conn,
	msgChan chan<- *WriteMessage,
	errChan chan<- error,
) {
	const op = "handlers.proxy.connect.write.readFromWS"

	log := l.With(sl.Op(op))

	for {
		msg := &WriteMessage{}
		err := ws.ReadJSON(msg)
		if err != nil {
			if errors.Is(err, websocket.ErrCloseSent) ||
				websocket.IsCloseError(err, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) ||
				strings.Contains(err.Error(), "closed network connection") {
				log.Info("connection closed")
				return
			}
			var syntaxError *json.SyntaxError
			var unmarshalTypeError *json.UnmarshalTypeError
			switch {
			case errors.As(err, &syntaxError), errors.As(err, &unmarshalTypeError):
				log.Warn("failed to read json message: invalid json", sl.Err(err))
				_ = ws.WriteMessage(websocket.TextMessage, []byte("invalid json"))
				continue
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				log.Warn("failed to read json message: unexpected end of JSON input")
				_ = ws.WriteMessage(websocket.TextMessage, []byte(err.Error()))
				continue
			}

			select {
			case errChan <- fmt.Errorf("%s: failed to read message from web socket: %w", op, err):
			default:
			}
			return
		}
		if msg.QOS > 2 {
			log.Warn("invalid qos", slog.Int("qos", int(msg.QOS)))
			_ = ws.WriteMessage(websocket.TextMessage, []byte("invalid qos"))
			continue
		}

		// save message in channel or exit if context done
		select {
		case msgChan <- msg:
		case <-ctx.Done():
			return
		}
	}
}
