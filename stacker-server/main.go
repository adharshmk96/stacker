package main

import (
	"fmt"
	"os"

	"stacker/internal/config"
	"stacker/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err == nil {
		err = server.Run(cfg)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
