// Adapted from lazydocker's filtered-list tests (MIT, © 2018 Jesse Duffield).
package panels

import (
	"reflect"
	"testing"
)

func TestFilteredListGet(t *testing.T) {
	tests := []struct {
		f    *FilteredList[int]
		args int
		want int
	}{
		{f: &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{0, 1, 2}}, args: 1, want: 2},
		{f: &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{0, 1, 2}}, args: 2, want: 3},
		{f: &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{1}}, args: 0, want: 2},
	}

	for _, tt := range tests {
		if got := tt.f.Get(tt.args); got != tt.want {
			t.Errorf("FilteredList.Get() = %v, want %v", got, tt.want)
		}
	}
}

func TestFilteredListLen(t *testing.T) {
	tests := []struct {
		f    *FilteredList[int]
		want int
	}{
		{f: &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{0, 1, 2}}, want: 3},
		{f: &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{1}}, want: 1},
	}

	for _, tt := range tests {
		if got := tt.f.Len(); got != tt.want {
			t.Errorf("FilteredList.Len() = %v, want %v", got, tt.want)
		}
	}
}

func TestFilteredListFilter(t *testing.T) {
	tests := []struct {
		f    *FilteredList[int]
		args func(int, int) bool
		want []int
	}{
		{
			f:    &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{0, 1, 2}},
			args: func(i int, _ int) bool { return i%2 == 0 },
			want: []int{1},
		},
		{
			f:    &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{0, 1, 2}},
			args: func(i int, _ int) bool { return i%2 == 1 },
			want: []int{0, 2},
		},
	}

	for _, tt := range tests {
		tt.f.Filter(tt.args)
		if !reflect.DeepEqual(tt.f.indices, tt.want) {
			t.Errorf("FilteredList.Filter() indices = %v, want %v", tt.f.indices, tt.want)
		}
	}
}

func TestFilteredListSort(t *testing.T) {
	tests := []struct {
		f    *FilteredList[int]
		args func(int, int) bool
		want []int
	}{
		{
			f:    &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{0, 1, 2}},
			args: func(i int, j int) bool { return i < j },
			want: []int{0, 1, 2},
		},
		{
			f:    &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{0, 1, 2}},
			args: func(i int, j int) bool { return i > j },
			want: []int{2, 1, 0},
		},
	}

	for _, tt := range tests {
		tt.f.Sort(tt.args)
		if !reflect.DeepEqual(tt.f.indices, tt.want) {
			t.Errorf("FilteredList.Sort() indices = %v, want %v", tt.f.indices, tt.want)
		}
	}
}

func TestFilteredListGetIndex(t *testing.T) {
	tests := []struct {
		f    *FilteredList[int]
		args int
		want int
	}{
		{f: &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{0, 1, 2}}, args: 1, want: 0},
		{f: &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{0, 1, 2}}, args: 2, want: 1},
		{f: &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{1}}, args: 0, want: -1},
	}

	for _, tt := range tests {
		if got := tt.f.GetIndex(tt.args); got != tt.want {
			t.Errorf("FilteredList.GetIndex() = %v, want %v", got, tt.want)
		}
	}
}

func TestFilteredListGetItems(t *testing.T) {
	tests := []struct {
		f    *FilteredList[int]
		want []int
	}{
		{f: &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{0, 1, 2}}, want: []int{1, 2, 3}},
		{f: &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{1}}, want: []int{2}},
	}

	for _, tt := range tests {
		got := tt.f.GetItems()
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("FilteredList.GetItems() = %v, want %v", got, tt.want)
		}
	}
}

func TestFilteredListSetItems(t *testing.T) {
	tests := []struct {
		f            *FilteredList[int]
		args         []int
		wantIndices  []int
		wantAllItems []int
	}{
		{
			f:            &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{0, 1, 2}},
			args:         []int{4, 5, 6},
			wantIndices:  []int{0, 1, 2},
			wantAllItems: []int{4, 5, 6},
		},
		{
			f:            &FilteredList[int]{allItems: []int{1, 2, 3}, indices: []int{1}},
			args:         []int{4},
			wantIndices:  []int{0},
			wantAllItems: []int{4},
		},
	}

	for _, tt := range tests {
		tt.f.SetItems(tt.args)
		if !reflect.DeepEqual(tt.f.indices, tt.wantIndices) {
			t.Errorf("FilteredList.SetItems() indices = %v, want %v", tt.f.indices, tt.wantIndices)
		}
		if !reflect.DeepEqual(tt.f.allItems, tt.wantAllItems) {
			t.Errorf("FilteredList.SetItems() allItems = %v, want %v", tt.f.allItems, tt.wantAllItems)
		}
	}
}
