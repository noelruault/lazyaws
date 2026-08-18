package resources

import (
	"context"
	"time"
)

type ConfirmLevel int

const (
	ConfirmNone      ConfirmLevel = iota // fires immediately
	ConfirmSimple                        // y/n popup
	ConfirmDangerous                     // must type an exact token
)

func (c ConfirmLevel) String() string {
	switch c {
	case ConfirmSimple:
		return "simple"
	case ConfirmDangerous:
		return "dangerous"
	default:
		return "none"
	}
}

type Action struct {
	Name    string
	Confirm ConfirmLevel
	Mutates bool // false means safe to run in read-only mode

	// Token uses recognizable identifiers because unreadable tokens train users to paste blindly.
	Token string

	// Prompt leaves Run's input empty when unset.
	Prompt string

	// Confirmation stays explicit because a generic name cannot carry operation-specific risk.
	Confirmation string

	// Timeout distinguishes single calls from multi-step workflows; zero uses DefaultTimeout.
	Timeout time.Duration

	// Run executes off the UI thread and may block until Timeout.
	Run func(ctx context.Context, input string) error
}

// DefaultTimeout allows SDK retries without leaving the UI stuck on a hung call.
const DefaultTimeout = 15 * time.Second

func (a Action) Deadline() time.Duration {
	if a.Timeout <= 0 {
		return DefaultTimeout
	}
	return a.Timeout
}

// Valid rejects incomplete actions, especially dangerous ones without an exact confirmation token.
func (a Action) Valid() error {
	switch {
	case a.Name == "":
		return errUnnamedAction
	case a.Run == nil:
		return errActionWithoutRun
	case a.Confirm == ConfirmDangerous && a.Token == "":
		return errDangerousWithoutToken
	}
	return nil
}
