package ecr

import (
	"context"

	"github.com/noelruault/lazyaws/internal/aws"
)

// State holds ECR-specific UI state.
type State struct {
	Repositories      []string
	FilteredRepos     []string
	SelectedRepoIndex int

	Images      []string
	FilteredImg []string
	SelectedImg int
}

// LoadRepositories fetches ECR repositories.
func LoadRepositories(ctx context.Context, client *aws.Client) ([]string, error) {
	if client == nil {
		return nil, nil
	}
	return client.ListECRRepositories(ctx)
}
