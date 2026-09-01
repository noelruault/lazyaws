package presentation

import (
	"strings"
	"testing"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/ui/utils"
)

// Any release ahead counts, patch releases included: the bullet answers "is this the newest build", not "is this close enough".
func TestUpdateStateFor(t *testing.T) {
	cases := []struct {
		name            string
		version, latest string
		want            UpdateState
	}{
		{"same release", "v0.3.0", "v0.3.0", UpdateCurrent},
		{"a patch behind", "v0.3.0", "v0.3.1", UpdateOutdated},
		{"a minor behind", "v0.3.0", "v0.4.0", UpdateOutdated},
		{"a major behind", "v0.3.0", "v1.0.0", UpdateOutdated},
		// Double-digit fields are why the comparison parses numbers instead of comparing the strings.
		{"a two-digit minor behind", "v0.9.0", "v0.10.0", UpdateOutdated},
		{"ahead of the newest release", "v0.4.0", "v0.3.0", UpdateCurrent},
		{"a source build", "dev", "v0.3.0", UpdateUnknown},
		{"an untagged build between releases", "v0.3.1-0.20260827212429-f22808126dc8", "v0.3.0", UpdateUnknown},
		{"the toolchain's own devel marker", "(devel)", "v0.3.0", UpdateUnknown},
		{"no answer yet", "v0.3.0", "", UpdateUnknown},
	}

	for _, c := range cases {
		if got := UpdateStateFor(c.version, c.latest); got != c.want {
			t.Errorf("%s: UpdateStateFor(%q, %q) = %d, want %d", c.name, c.version, c.latest, got, c.want)
		}
	}
}

// The bullet carries the whole message, so its colour is the assertion: green and yellow must not be renderable as each other.
func TestVersionBullet(t *testing.T) {
	previous := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = previous })

	current := VersionBullet("v0.3.0", UpdateCurrent)
	outdated := VersionBullet("v0.3.0", UpdateOutdated)

	if got := utils.Decolorise(current); got != "v0.3.0 ●" {
		t.Errorf("VersionBullet(current) = %q, want %q", got, "v0.3.0 ●")
	}
	if current == outdated {
		t.Error("VersionBullet renders the same string whether or not a newer release exists")
	}
	if want := utils.ColoredString("●", color.FgGreen); !strings.Contains(current, want) {
		t.Errorf("VersionBullet(current) = %q, want a green bullet", current)
	}
	if want := utils.ColoredString("●", color.FgYellow); !strings.Contains(outdated, want) {
		t.Errorf("VersionBullet(outdated) = %q, want a yellow bullet", outdated)
	}

	// An unchecked build must not wear a bullet in the information line's own colour, which would read as "up to date".
	if got := VersionBullet("v0.3.0", UpdateUnknown); got != "v0.3.0" {
		t.Errorf("VersionBullet(unknown) = %q, want the bare version", got)
	}
	if got := VersionBullet("", UpdateCurrent); got != "" {
		t.Errorf("VersionBullet(no version) = %q, want nothing rendered", got)
	}
}

func TestVersionLine(t *testing.T) {
	cases := []struct {
		name            string
		version, latest string
		state           UpdateState
		want            string
	}{
		{"outdated names the release", "v0.3.0", "v0.4.0", UpdateOutdated, "lazyaws v0.3.0 (v0.4.0 available)"},
		{"current stays quiet", "v0.3.0", "v0.3.0", UpdateCurrent, "lazyaws v0.3.0"},
		{"unchecked stays quiet", "v0.3.0", "", UpdateUnknown, "lazyaws v0.3.0"},
		{"a source build still names itself", "dev", "", UpdateUnknown, "lazyaws dev"},
	}

	for _, c := range cases {
		if got := utils.Decolorise(VersionLine(c.version, c.latest, c.state)); got != c.want {
			t.Errorf("%s: VersionLine() = %q, want %q", c.name, got, c.want)
		}
	}
}
