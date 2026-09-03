package config

import (
	"flag"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Config struct {
	Region      string
	ShowVersion bool
	ShowGallery bool
	Debug       bool

	// Keymap is the preset named by -keymap=<name> on this run, empty when the flag named nothing.
	// It is a one-time switch rather than a per-run override: main writes it to the config file, so the choice outlives the process that made it.
	Keymap string

	// KeymapReport is -keymap with no value, which asks where the keys live instead of moving them.
	// The path is platform dependent and too long for the help menu, so the flag is where a user finds it.
	KeymapReport bool

	// AllowWrites is the only thing that permits lazyaws to change anything in AWS, and it exists only as a flag.
	// It is deliberately not a config file setting: a first run should be safe without anyone having read the documentation, and a file that grants writes could be inherited from a dotfile repo without the user deciding it today.
	AllowWrites bool

	User UserConfig
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

// keymapFlag lets -keymap be both a question and a switch.
// IsBoolFlag is what allows the bare form: the flag package demands a value for anything else, so `-keymap` alone would be a usage error rather than "tell me where the keys live".
type keymapFlag struct {
	name string
	bare bool
}

func (f *keymapFlag) String() string { return f.name }

func (f *keymapFlag) IsBoolFlag() bool { return true }

func (f *keymapFlag) Set(value string) error {
	// The flag package hands a bare boolean-looking flag the string "true", which is not a layout anyone asked for.
	if value == "true" {
		f.bare = true
		return nil
	}

	f.name = value

	return nil
}

func parse(fs *flag.FlagSet, args []string) Config {
	region := fs.String("region", os.Getenv("AWS_REGION"), "AWS region (overrides AWS_REGION)")
	showVersion := fs.Bool("version", false, "print version and exit")
	showGallery := fs.Bool("gallery", false, "print the UI component gallery and exit")
	debug := fs.Bool("debug", false, "log to ~/.lazyaws/debug.log instead of discarding")
	allowWrites := fs.Bool("allow-writes", false, "permit actions that change AWS state; without it every mutating call is refused")

	keymap := &keymapFlag{}
	fs.Var(keymap, "keymap", "-keymap=<name> switches the navigation layout for good (international, lazy, vim, emacs); -keymap alone says where the file is")

	if !fs.Parsed() {
		_ = fs.Parse(args)
	}

	// A bare -keymap takes no value, so a name given the other way round, `-keymap vim`, arrives as a leftover argument, and flag parsing stops at it so anything after is left unparsed too.
	// Reading the first leftover here means the space form switches rather than silently reporting, which is what a user typing it expects.
	name := keymap.name
	if keymap.bare && fs.NArg() > 0 && !strings.HasPrefix(fs.Arg(0), "-") {
		name = fs.Arg(0)
	}

	return Config{
		Region:       *region,
		ShowVersion:  *showVersion,
		ShowGallery:  *showGallery,
		Debug:        *debug,
		Keymap:       name,
		KeymapReport: keymap.bare && name == "",
		AllowWrites:  *allowWrites,
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
