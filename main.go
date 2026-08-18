package main

import (
	"fmt"
	"os"

	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui"
)

// version stays "dev" unless release builds replace it through -ldflags.
var version = "dev"

func main() {
	cfg := config.Load()
	if cfg.ShowVersion {
		fmt.Println("lazyaws " + version)
		return
	}
	if err := ui.Run(cfg); err != nil {
		// Print startup failures verbatim because they already contain multiline guidance.
		if ui.IsStartupFailure(err) {
			fmt.Fprint(os.Stderr, err.Error())
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
