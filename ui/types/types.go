// Package types holds shared UI types. Ported from lazydocker's pkg/gui/types (MIT, © 2018 Jesse Duffield).
package types

type MenuItem struct {
	Label string

	LabelColumns []string

	OnPress func() error

	OpensMenu bool

	// Mutates marks an item that changes AWS state (or opens a shell that could). Read-only mode drops these before the menu is shown; see Gui.Menu.
	Mutates bool
}
