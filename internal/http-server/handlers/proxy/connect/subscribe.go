package connect

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"smart-pc-mqtt-proxy/internal/lib/logger/sl"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
)

// startSubscribe running startRead or/and startWrite according to subscribeMode
func startSubscribe(
	ctx context.Context,
	log *slog.Logger,
	subscribeMode string,
	deadline, subscribeTimeout, writeTimeout, publishTimeout time.Duration,
	wsConn *websocket.Conn,
	mqttClient mqtt.Client,
	topic string,
) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// if deadline exists, add timeout to context
	if deadline > 0 {
		var cancelDeadline context.CancelFunc
		ctx, cancelDeadline = context.WithTimeout(ctx, deadline)
		defer cancelDeadline()
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	shutdown := func() {
		cancel()
		mqttClient.Disconnect(250)
		wg.Wait()
		wsConn.Close()
	}

	switch subscribeMode {
	case modeRead:
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := startRead(ctx, subscribeTimeout, writeTimeout, wsConn, mqttClient, topic); err != nil {
				select {
				case errCh <- fmt.Errorf("read error: %w", err):
				default:
				}
			}
		}()

	case modeWrite:
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := startWrite(ctx, publishTimeout, wsConn, mqttClient, topic); err != nil {
				select {
				case errCh <- fmt.Errorf("write error: %w", err):
				default:
				}
			}
		}()

	case modeReadWrite:
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := startRead(ctx, subscribeTimeout, writeTimeout, wsConn, mqttClient, topic); err != nil {
				select {
				case errCh <- fmt.Errorf("read error: %w", err):
				default:
				}
			}
		}()
		go func() {
			defer wg.Done()
			if err := startWrite(ctx, publishTimeout, wsConn, mqttClient, topic); err != nil {
				select {
				case errCh <- fmt.Errorf("write error: %w", err):
				default:
				}
			}
		}()
	}

	// wait for ctx done or error
	select {
	case <-ctx.Done():
		log.Info("context cancelled", slog.String("reason", ctx.Err().Error()))
	case err := <-errCh:
		if errors.Is(err, ErrConnectionStopped) {
			log.Info("connection stopped")
			break
		}
		log.Warn("operation failed", sl.Err(err))
	}

	log.Info("shutting down connection")

	shutdown()
}
