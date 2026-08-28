package config

import (
	"flag"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	Region      string
	ShowVersion bool
	ShowGallery bool
	Debug       bool
	User        UserConfig
}

var (
	cfg      Config
	loadOnce sync.Once
)

func Load() Config {
	loadOnce.Do(func() {
		cfg = parse(flag.CommandLine, os.Args[1:])

		if user, err := LoadUserConfig(); err == nil {
			cfg.User = user
		} else {
			cfg.User = DefaultUserConfig()
		}

		setupLogging(cfg.Debug)
	})
	return cfg
}

func parse(fs *flag.FlagSet, args []string) Config {
	region := fs.String("region", os.Getenv("AWS_REGION"), "AWS region (overrides AWS_REGION)")
	showVersion := fs.Bool("version", false, "print version and exit")
	showGallery := fs.Bool("gallery", false, "print the UI component gallery and exit")
	debug := fs.Bool("debug", false, "log to ~/.lazyaws/debug.log instead of discarding")
	if !fs.Parsed() {
		_ = fs.Parse(args)
	}

	return Config{
		Region:      *region,
		ShowVersion: *showVersion,
		ShowGallery: *showGallery,
		Debug:       *debug,
	}
}

// setupLogging discards unrouted logs because stderr output would corrupt gocui's terminal.
func setupLogging(debug bool) {
	// A dependency logging through the standard logger would land on stderr, and gocui draws by absolute position, so one stray line scrolls the screen and every later frame renders a row off.
	log.SetOutput(io.Discard)

	if !debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".lazyaws")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, nil)))
	log.SetOutput(f)
}
