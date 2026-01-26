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

type StartSubscribeConfig struct {
	SubscribeMode    string
	Deadline         time.Duration
	SubscribeTimeout time.Duration
	WriteTimeout     time.Duration
	PublishTimeout   time.Duration
	WSConn           *websocket.Conn
	MQTTClient       mqtt.Client
	Topic            string
}

// startSubscribe running startRead or/and startWrite according to subscribeMode
func startSubscribe(
	ctx context.Context,
	log *slog.Logger,
	cfg *StartSubscribeConfig,
) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// if deadline exists, add timeout to context
	if cfg.Deadline > 0 {
		var cancelDeadline context.CancelFunc
		ctx, cancelDeadline = context.WithTimeout(ctx, cfg.Deadline)
		defer cancelDeadline()
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	shutdown := func() {
		cancel()
		cfg.MQTTClient.Disconnect(250)
		wg.Wait()
		_ = cfg.WSConn.Close()
	}

	switch cfg.SubscribeMode {
	case modeRead:
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := startRead(ctx, log, &StartReadConfig{
				Topic:            cfg.Topic,
				MQQTClient:       cfg.MQTTClient,
				WSConn:           cfg.WSConn,
				SubscribeTimeout: cfg.SubscribeTimeout,
				WriteTimeout:     cfg.WriteTimeout,
			}); err != nil {
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
			if err := startWrite(ctx, log, &StartWriteConfig{
				Topic:          cfg.Topic,
				PublishTimeout: cfg.PublishTimeout,
				WSConn:         cfg.WSConn,
				MQTTClient:     cfg.MQTTClient,
			}); err != nil {
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
			if err := startRead(ctx, log, &StartReadConfig{
				Topic:            cfg.Topic,
				MQQTClient:       cfg.MQTTClient,
				WSConn:           cfg.WSConn,
				SubscribeTimeout: cfg.SubscribeTimeout,
				WriteTimeout:     cfg.WriteTimeout,
			}); err != nil {
				select {
				case errCh <- fmt.Errorf("read error: %w", err):
				default:
				}
			}
		}()
		go func() {
			defer wg.Done()
			if err := startWrite(ctx, log, &StartWriteConfig{
				Topic:          cfg.Topic,
				PublishTimeout: cfg.PublishTimeout,
				WSConn:         cfg.WSConn,
				MQTTClient:     cfg.MQTTClient,
			}); err != nil {
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
