package presentation

import (
	"github.com/noelruault/lazyaws/apps/aws"
)

// GetBucketDisplayStrings leaves region unknown because ListBuckets omits it.
func GetBucketDisplayStrings(b *aws.Bucket) []string {
	return []string{
		b.Name,
		b.Region,
		b.CreationDate,
	}
}
