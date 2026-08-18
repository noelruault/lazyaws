package presentation

import (
	"reflect"
	"testing"

	"github.com/noelruault/lazyaws/ui/types"
)

func TestGetMenuItemDisplayStrings(t *testing.T) {
	item := &types.MenuItem{LabelColumns: []string{"Start", "start the instance"}}
	got := GetMenuItemDisplayStrings(item)
	want := []string{"Start", "start the instance"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetMenuItemDisplayStrings() = %v, want %v", got, want)
	}
}
