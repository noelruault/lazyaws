package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"golang.org/x/term"

	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui"
	"github.com/noelruault/lazyaws/ui/presentation"
)

// version stays "dev" unless release builds replace it through -ldflags.
var version = "dev"

// galleryFallbackWidth is used when stdout is not a terminal (a pipe, a redirect), wide enough for the two-column blocks to keep their shape.
const galleryFallbackWidth = 110

func main() {
	cfg := config.Load()
	if cfg.ShowVersion {
		fmt.Println("lazyaws " + version)
		return
	}
	if cfg.ShowGallery {
		// Forced colour because the gallery exists to judge the styled render, and piping it through less -R would otherwise strip the one thing under review.
		color.NoColor = false
		width := galleryFallbackWidth
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
			width = w
		}
		fmt.Println(presentation.Gallery(width))
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
