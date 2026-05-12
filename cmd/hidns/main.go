package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hidns/internal/config"
	"hidns/internal/server"
)

func main() {
	listen := flag.String("listen", ":53", "UDP listen address")
	cfgPath := flag.String("config", "hosts.txt", "path to hosts file (domain;ip per line)")
	upstream := flag.String("upstream", "114.114.114.114:53", "upstream DNS address")
	timeout := flag.Duration("timeout", 5*time.Second, "upstream read/write deadline")
	verbose := flag.Bool("v", false, "verbose logging (debug)")
	flag.Parse()

	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	tbl, err := config.Load(*cfgPath, func(line int, msg string) {
		logger.Warn("config skip line", "line", line, "reason", msg)
	})
	if err != nil {
		logger.Error("load config", "path", *cfgPath, "err", err)
		os.Exit(1)
	}

	srv := &server.Server{
		ListenAddr: *listen,
		Upstream:   *upstream,
		Timeout:    *timeout,
		Table:      tbl,
		Logger:     logger,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("starting", "entries", tbl.Len())
	if err := srv.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
		logger.Error("server exit", "err", err)
		os.Exit(1)
	}
}
