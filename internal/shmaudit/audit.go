package shmaudit

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Logger — минимальный логгер без секретов.
type Logger func(format string, args ...any)

// Run выполняет полный read-only аудит и пишет отчёты.
func Run(ctx context.Context, cfg *Config, opt Options, log Logger) (*Summary, error) {
	if log == nil {
		log = func(string, ...any) {}
	}
	opt.FCCategory = strings.TrimSpace(opt.FCCategory)
	opt.VFFCategory = strings.TrimSpace(opt.VFFCategory)
	if err := ValidateOptions(opt); err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	client, err := NewClient(cfg, opt.RequestDelay)
	if err != nil {
		return nil, err
	}

	log("authenticating with SHM (credentials not logged)")
	if err := client.Authenticate(ctx); err != nil {
		return nil, err
	}

	ds := Dataset{}

	users, err := client.FetchUsers(ctx, opt.PageSize, log)
	if err != nil {
		return nil, fmt.Errorf("fetch users: %w", err)
	}
	ds.Users = users
	log("loaded users=%d", len(users))

	userServices, err := client.FetchUserServices(ctx, opt.PageSize, log)
	if err != nil {
		return nil, fmt.Errorf("fetch user_services: %w", err)
	}
	ds.UserServices = userServices
	log("loaded user_services=%d", len(userServices))

	services, err := client.FetchServices(ctx, opt.PageSize, log)
	if err != nil {
		return nil, fmt.Errorf("fetch services: %w", err)
	}
	ds.Services = services
	log("loaded services=%d", len(services))

	withdrawals, err := client.FetchWithdrawals(ctx, opt.PageSize, log)
	if err != nil {
		return nil, fmt.Errorf("fetch withdrawals: %w", err)
	}
	ds.Withdrawals = withdrawals
	log("loaded withdrawals=%d", len(withdrawals))

	payments, err := client.FetchPayments(ctx, opt.PageSize, log)
	if err != nil {
		return nil, fmt.Errorf("fetch payments: %w", err)
	}
	ds.Payments = payments
	log("loaded payments=%d", len(payments))

	records := ClassifyAll(ds, opt.FCCategory, opt.VFFCategory)
	log("classified legacy users=%d", len(records))

	counts := CountClassifications(records)
	summary := Summary{
		GeneratedAt: time.Now().UTC(),
		Complete:    true,
		BaseURLHost: BaseURLHost(cfg.API.BaseURL),
		FCCategory:  opt.FCCategory,
		VFFCategory: opt.VFFCategory,
		PageSize:    opt.PageSize,
		Fetched: FetchedCounts{
			Users:        len(ds.Users),
			UserServices: len(ds.UserServices),
			Services:     len(ds.Services),
			Withdrawals:  len(ds.Withdrawals),
			Payments:     len(ds.Payments),
		},
		LegacyTelegramUsers: len(records),
		Classifications:     counts,
	}

	if err := WriteReports(opt.OutputDir, summary, records); err != nil {
		return nil, err
	}
	log("reports written to %s", opt.OutputDir)
	log("legacy=%d fc_only=%d vff_only=%d shared=%d empty=%d ambiguous=%d",
		len(records), counts.FCOnly, counts.VFFOnly, counts.Shared, counts.Empty, counts.Ambiguous)
	return &summary, nil
}

// RunFromFlags — удобная обёртка для CLI: загружает конфиг и запускает аудит.
func RunFromFlags(ctx context.Context, opt Options, log Logger) (*Summary, error) {
	if err := ValidateOptions(opt); err != nil {
		return nil, err
	}
	cfg, err := LoadConfig(opt.ConfigPath)
	if err != nil {
		return nil, err
	}
	return Run(ctx, cfg, opt, log)
}

// StdLogger пишет в stderr.
func StdLogger() Logger {
	return func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}
