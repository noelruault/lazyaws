// Ported from lazydocker's pkg/gui/presentation/menu_items.go (MIT, © 2018 Jesse Duffield).
package presentation

import "github.com/noelruault/lazyaws/ui/types"

func GetMenuItemDisplayStrings(menuItem *types.MenuItem) []string {
	return menuItem.LabelColumns
}
