package presentation

import (
	"github.com/noelruault/lazyaws/apps/aws"
)

func GetECRRepositoryDisplayStrings(r *aws.ECRRepository) []string {
	mutability := r.TagMutability
	if mutability == "" {
		mutability = "-"
	}
	scanOnPush := "off"
	if r.ScanOnPush {
		scanOnPush = "on"
	}
	return []string{r.Name, mutability, scanOnPush}
}
