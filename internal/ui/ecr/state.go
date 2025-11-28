package ecr

import "github.com/noelruault/lazyaws/internal/aws"

// State holds ECR-specific UI state.
type State struct {
	Repositories      []aws.ECRRepository
	FilteredRepos     []aws.ECRRepository
	SelectedRepoIndex int

	CurrentRepo string

	Images          []aws.ECRImage
	FilteredImages  []aws.ECRImage
	SelectedImage   int
	ImageScanResult *aws.ECRScanResult
}
