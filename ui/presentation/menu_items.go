package presentation

import "github.com/noelruault/lazyaws/ui/types"

func GetMenuItemDisplayStrings(menuItem *types.MenuItem) []string {
	return menuItem.LabelColumns
}
