package main

import (
	"smart-pc-mqtt-proxy/internal/config"
	"smart-pc-mqtt-proxy/internal/lib/logger"
)

func main() {
	cfg := config.MustLoad()
	log := logger.SetupLogger(cfg.Env)

	log.Debug("debug messages are enabled")
}
