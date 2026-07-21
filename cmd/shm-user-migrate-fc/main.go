package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ryabkov82/vpnbot/internal/shmmigrate"
)

func main() {
	os.Exit(run())
}

func run() int {
	configPath := flag.String("config", "", "path to FC JSON config (api section)")
	planPath := flag.String("plan", "", "path to fc-only.json allowlist from audit")
	outputDir := flag.String("output", "", "output directory for backup/preflight/result")
	apply := flag.Bool("apply", false, "apply migration (default: dry-run)")
	flag.Parse()

	opt := shmmigrate.Options{
		ConfigPath: *configPath,
		PlanPath:   *planPath,
		OutputDir:  *outputDir,
		Apply:      *apply,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if _, err := shmmigrate.Run(ctx, opt, shmmigrate.StdLogger()); err != nil {
		fmt.Fprintf(os.Stderr, "shm-user-migrate-fc: %v\n", err)
		return 1
	}
	return 0
}
