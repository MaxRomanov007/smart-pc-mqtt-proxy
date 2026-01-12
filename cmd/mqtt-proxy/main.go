package main

import (
	"smart-pc-mqtt-proxy/internal/config"
	httpServer "smart-pc-mqtt-proxy/internal/http-server"
	"smart-pc-mqtt-proxy/internal/lib/logger"
	"smart-pc-mqtt-proxy/internal/lib/logger/sl"
)

func main() {
	cfg := config.MustLoad()
	log := logger.SetupLogger(cfg.Env)

	log.Debug("debug messages are enabled")

	srv, err := httpServer.New(log, cfg)
	if err != nil {
		log.Error("failed to create server", sl.Err(err))
		return
	}

	srv.Start()
}
