package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ryabkov82/vpnbot/internal/config"
)

func main() {
	configPath := flag.String("config", "", "path to vpnbot JSON config")
	flag.Parse()

	path := strings.TrimSpace(*configPath)
	if path == "" {
		fmt.Fprintln(os.Stderr, "configcheck: -config is required")
		fmt.Fprintln(os.Stderr, "usage: configcheck -config /path/to/config.json")
		os.Exit(1)
	}

	cfg, err := config.LoadFromFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configcheck: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(config.FormatSafeBrandSummary(cfg))
}
