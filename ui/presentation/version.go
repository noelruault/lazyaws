package presentation

import (
	"strconv"
	"strings"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/ui/utils"
)

// UpdateState is what this build knows about newer releases.
// UpdateUnknown is the honest state, not a failure: nothing has answered yet, or the build carries no version a release can be compared against.
type UpdateState int

const (
	UpdateUnknown UpdateState = iota
	UpdateCurrent
	UpdateOutdated
)

const versionBullet = "●"

// VersionBullet is the information line's version: the build, plus a bullet naming whether a newer release exists.
// UpdateUnknown renders no bullet at all, because a bullet in the line's own colour would claim a check that never happened.
func VersionBullet(version string, state UpdateState) string {
	if version == "" {
		return ""
	}

	switch state {
	case UpdateCurrent:
		return version + " " + utils.ColoredString(versionBullet, color.FgGreen)
	case UpdateOutdated:
		return version + " " + utils.ColoredString(versionBullet, color.FgYellow)
	default:
		return version
	}
}

// VersionLine is the Settings header, where there is room to name the release the bullet can only point at.
// It never says how to upgrade: the point is to let the reader decide, not to walk them through it.
func VersionLine(version, latest string, state UpdateState) string {
	if version == "" {
		return ""
	}

	line := "lazyaws " + version
	if state != UpdateOutdated || latest == "" {
		return line
	}

	return line + " " + utils.ColoredString("("+latest+" available)", color.FgYellow)
}

// UpdateStateFor compares a build against the newest published release, counting any version behind as out of date: a minor behind is still behind.
// Whatever it cannot compare stays UpdateUnknown rather than being guessed at, which is also what keeps a source build from being nagged about a release it is already ahead of.
func UpdateStateFor(version, latest string) UpdateState {
	current, ok := parseVersion(version)
	if !ok {
		return UpdateUnknown
	}

	newest, ok := parseVersion(latest)
	if !ok {
		return UpdateUnknown
	}

	for i := range current {
		switch {
		case current[i] < newest[i]:
			return UpdateOutdated
		case current[i] > newest[i]:
			return UpdateCurrent
		}
	}

	return UpdateCurrent
}

// parseVersion reads a plain vMAJOR.MINOR.PATCH tag and rejects everything else.
// A pre-release or pseudo-version suffix is a rejection rather than a field to ignore: `v0.3.1-0.20260827212429-f22808126dc8` is an untagged build between releases, and comparing it as v0.3.1 would report a build that is ahead of the newest release as behind it.
func parseVersion(version string) ([3]int, bool) {
	var parsed [3]int

	trimmed, ok := strings.CutPrefix(version, "v")
	if !ok {
		return parsed, false
	}

	fields := strings.Split(trimmed, ".")
	if len(fields) != len(parsed) {
		return parsed, false
	}

	for i, field := range fields {
		number, err := strconv.Atoi(field)
		if err != nil || number < 0 {
			return parsed, false
		}
		parsed[i] = number
	}

	return parsed, true
}
