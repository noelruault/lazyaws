package utils

import (
	"strings"
	"sync"
	"testing"

	"github.com/fatih/color"
)

// ColoredString caches a *color.Color per attribute. A cached Color must still read the global NoColor at print time, or a colour-disabled terminal gets escape codes anyway.
func TestColoredStringHonoursNoColorAfterCaching(t *testing.T) {
	previous := color.NoColor
	t.Cleanup(func() { color.NoColor = previous })

	color.NoColor = false
	coloured := ColoredString("text", color.FgGreen)
	if !strings.Contains(coloured, "\x1b[") {
		t.Fatalf("ColoredString with colour on = %q, want escape codes", coloured)
	}

	// Same attribute, now served from the cache, with colour switched off.
	color.NoColor = true
	if plain := ColoredString("text", color.FgGreen); plain != "text" {
		t.Errorf("ColoredString with colour off = %q, want %q", plain, "text")
	}

	color.NoColor = false
	if again := ColoredString("text", color.FgGreen); again != coloured {
		t.Errorf("ColoredString after re-enabling = %q, want %q", again, coloured)
	}
}

// The precomputed escape pairs must stay byte-identical to what fatih/color produces.
// Nothing else would catch the library changing a reset code under us.
func TestColoredStringMatchesTheLibraryExactly(t *testing.T) {
	previous := color.NoColor
	t.Cleanup(func() { color.NoColor = previous })
	color.NoColor = false

	for attribute := range colorWraps {
		reference := color.New(attribute)
		reference.EnableColor()

		for _, sample := range []string{"", "x", "a longer sample string", "with\nnewline"} {
			if got, want := ColoredString(sample, attribute), reference.Sprint(sample); got != want {
				t.Errorf("ColoredString(%q, %v) = %q, want %q", sample, attribute, got, want)
			}
		}
	}
}

// An attribute outside the table must still colour correctly via the fallback.
func TestColoredStringFallsBackForUntabulatedAttributes(t *testing.T) {
	previous := color.NoColor
	t.Cleanup(func() { color.NoColor = previous })
	color.NoColor = false

	if _, tabulated := colorWraps[color.Underline]; tabulated {
		t.Skip("Underline is in the table; this test needs an attribute that is not")
	}

	reference := color.New(color.Underline)
	reference.EnableColor()
	if got, want := ColoredString("x", color.Underline), reference.Sprint("x"); got != want {
		t.Errorf("fallback = %q, want %q", got, want)
	}
}

func TestColoredStringLeavesFgWhiteAlone(t *testing.T) {
	if got := ColoredString("text", color.FgWhite); got != "text" {
		t.Errorf("ColoredString(FgWhite) = %q, want it uncoloured", got)
	}
}

// The cache is shared, and rendering can run alongside other panels; -race guards the rest.
func TestColoredStringIsConcurrencySafe(t *testing.T) {
	previous := color.NoColor
	t.Cleanup(func() { color.NoColor = previous })
	color.NoColor = false

	want := ColoredString("text", color.FgYellow)

	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 64 {
				if got := ColoredString("text", color.FgYellow); got != want {
					t.Errorf("concurrent ColoredString = %q, want %q", got, want)
					return
				}
			}
		})
	}
	wg.Wait()
}
