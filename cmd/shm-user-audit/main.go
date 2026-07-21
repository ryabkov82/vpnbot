package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ryabkov82/vpnbot/internal/shmaudit"
)

func main() {
	os.Exit(run())
}

func run() int {
	configPath := flag.String("config", "", "path to FC JSON config (api section only)")
	outputDir := flag.String("output", "", "output directory for audit reports")
	fcCategory := flag.String("fc-category", "", "FC service category (exact match)")
	vffCategory := flag.String("vff-category", "", "VFF service category (exact match)")
	pageSize := flag.Int("page-size", 250, "SHM page size (1..1000)")
	requestDelay := flag.Duration("request-delay", 0, "delay between page requests")
	flag.Parse()

	opt := shmaudit.Options{
		ConfigPath:   *configPath,
		OutputDir:    *outputDir,
		FCCategory:   *fcCategory,
		VFFCategory:  *vffCategory,
		PageSize:     *pageSize,
		RequestDelay: *requestDelay,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log := shmaudit.StdLogger()
	if _, err := shmaudit.RunFromFlags(ctx, opt, log); err != nil {
		fmt.Fprintf(os.Stderr, "shm-user-audit: %v\n", err)
		return 1
	}
	return 0
}
