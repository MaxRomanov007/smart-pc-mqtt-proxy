package httpServer

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"smart-pc-mqtt-proxy/internal/config"
	"smart-pc-mqtt-proxy/internal/http-server/handlers/proxy/connect"
	mwAuth "smart-pc-mqtt-proxy/internal/http-server/middlewares/auth"
	mwLogger "smart-pc-mqtt-proxy/internal/http-server/middlewares/logger"
	"smart-pc-mqtt-proxy/internal/lib/logger/sl"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/gorilla/websocket"
)

type Server struct {
	log *slog.Logger
	srv *http.Server
}

func New(log *slog.Logger, cfg *config.Config) (*Server, error) {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(mwLogger.New(log))
	router.Use(middleware.Recoverer)

	if cfg.HTTPServer.Cors != nil {
		router.Use(corsHandler(cfg.HTTPServer.Cors))
	}

	upgrader := createWebsocketUpgrader(cfg.Websocket)

	for pattern, route := range cfg.Routes {
		handler := connect.New(log, route, upgrader, cfg.MQTT, cfg.Websocket)
		if handler == nil {
			continue
		}

		router.
			With(mwAuth.New(log, route.AdditionalScopes...)).
			Get(pattern, connect.New(log, route, upgrader, cfg.MQTT, cfg.Websocket))
	}

	srv := &http.Server{
		Addr:         cfg.Address,
		Handler:      router,
		ReadTimeout:  cfg.HTTPServer.Timeout,
		WriteTimeout: cfg.HTTPServer.Timeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return &Server{log, srv}, nil
}

func (s *Server) Start() {
	s.log.Info("starting server", slog.String("address", s.srv.Addr))

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		s.log.Info("shutting down server")

		shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownRelease()

		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			s.log.Error("failed to shut down server", sl.Err(err))
		}
	}()

	if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.log.Error("failed to start server", sl.Err(err))
	}

	s.log.Info("server stopped")
}

func corsHandler(cfg *config.Cors) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:     cfg.AllowedOrigins,
		AllowedMethods:     cfg.AllowedMethods,
		AllowedHeaders:     cfg.AllowedHeaders,
		ExposedHeaders:     cfg.ExposedHeaders,
		AllowCredentials:   cfg.AllowCredentials,
		MaxAge:             cfg.MaxAge,
		OptionsPassthrough: cfg.OptionsPassthrough,
		Debug:              cfg.Debug,
	})
}

func createWebsocketUpgrader(cfg *config.Websocket) *websocket.Upgrader {
	return &websocket.Upgrader{
		HandshakeTimeout:  cfg.HandshakeTimeout,
		ReadBufferSize:    cfg.ReadBufferSize,
		WriteBufferSize:   cfg.WriteBufferSize,
		Subprotocols:      cfg.Subprotocols,
		EnableCompression: cfg.EnableCompression,
		CheckOrigin:       func(r *http.Request) bool { return true },
		Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
			return
		},
	}
}
