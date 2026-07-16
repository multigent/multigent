package main

import (
	"io"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/multigent/multigent/internal/appconfig"
	"github.com/multigent/multigent/internal/daemon"
)

type serviceLogOptions struct {
	File      string
	Level     string
	Format    string
	MaxSizeMB int
	Stderr    bool
}

func resolveServiceLogOptions(cfg *appconfig.Config, file, level, format string, maxSizeMB int, flagChanged func(string) bool) serviceLogOptions {
	opts := serviceLogOptions{
		File:      daemon.DefaultLogFile(),
		Level:     "info",
		Format:    "json",
		MaxSizeMB: daemon.DefaultLogMaxSize / 1024 / 1024,
		Stderr:    true,
	}
	if cfg != nil {
		if cfg.Logging.File != "" {
			opts.File = cfg.Logging.File
		}
		if cfg.Logging.Level != "" {
			opts.Level = cfg.Logging.Level
		}
		if cfg.Logging.Format != "" {
			opts.Format = cfg.Logging.Format
		}
		if cfg.Logging.MaxSizeMB > 0 {
			opts.MaxSizeMB = cfg.Logging.MaxSizeMB
		}
		if cfg.Logging.Stderr != nil {
			opts.Stderr = *cfg.Logging.Stderr
		}
	}
	if v := os.Getenv("MULTIGENT_LOG_FILE"); v != "" {
		opts.File = v
	}
	if v := os.Getenv("MULTIGENT_LOG_LEVEL"); v != "" {
		opts.Level = v
	}
	if v := os.Getenv("MULTIGENT_LOG_FORMAT"); v != "" {
		opts.Format = v
	}
	if v := os.Getenv("MULTIGENT_LOG_MAX_SIZE_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			opts.MaxSizeMB = n
		}
	} else if v := os.Getenv("MULTIGENT_LOG_MAX_SIZE"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			opts.MaxSizeMB = int(n / 1024 / 1024)
			if opts.MaxSizeMB <= 0 {
				opts.MaxSizeMB = 1
			}
		}
	}
	if v := os.Getenv("MULTIGENT_LOG_STDERR"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			opts.Stderr = b
		}
	}
	if flagChanged("log-file") {
		opts.File = file
	}
	if flagChanged("log-level") {
		opts.Level = level
	}
	if flagChanged("log-format") {
		opts.Format = format
	}
	if flagChanged("log-max-size") && maxSizeMB > 0 {
		opts.MaxSizeMB = maxSizeMB
	}
	return opts
}

func initServiceLogger(opts serviceLogOptions, service string) (func(), error) {
	maxSize := int64(opts.MaxSizeMB) * 1024 * 1024
	if maxSize <= 0 {
		maxSize = daemon.DefaultLogMaxSize
	}
	writer, err := daemon.NewRotatingWriter(opts.File, maxSize)
	if err != nil {
		return nil, err
	}
	var out io.Writer = writer
	if opts.Stderr {
		out = io.MultiWriter(writer, os.Stderr)
	}
	level := parseLogLevel(opts.Level)
	handlerOpts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if strings.EqualFold(opts.Format, "text") {
		handler = slog.NewTextHandler(out, handlerOpts)
	} else {
		handler = slog.NewJSONHandler(out, handlerOpts)
	}
	logger := slog.New(handler).With("service", service)
	slog.SetDefault(logger)
	log.SetOutput(stdLogWriter{})
	log.SetFlags(0)
	log.Printf("logger initialized level=%s format=%s file=%s max_size_mb=%d stderr=%v", opts.Level, opts.Format, opts.File, opts.MaxSizeMB, opts.Stderr)
	return func() { _ = writer.Close() }, nil
}

func parseLogLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type stdLogWriter struct{}

func (stdLogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		slog.Info(msg)
	}
	return len(p), nil
}
