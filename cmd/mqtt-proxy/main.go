package main

import (
	"fmt"
	"smart-pc-mqtt-proxy/internal/config"
)

func main() {
	cfg := config.MustLoad()

	fmt.Println(cfg)
}
