package presentation

import (
	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// BucketWeights sizes the bucket row so the name absorbs the slack; region and creation date are fixed-width.
func BucketWeights() []int {
	return []int{1, 0, 0}
}

// GetBucketDisplayCells leaves region unknown because ListBuckets omits it.
func GetBucketDisplayCells(b *aws.Bucket) []utils.Cell {
	return []utils.Cell{
		{Text: b.Name},
		{Text: b.Region},
		{Text: b.CreationDate},
	}
}
