// Package resources keeps navigation and actions provider-agnostic through a shared registry.
package resources

import (
	"strings"
)

// Separator cannot be "/" because AWS identifiers and S3 keys preserve slashes verbatim.
const Separator = ":"

// Ref separates resource identity from an optional selector path so identifiers remain verbatim.
type Ref struct {
	Provider string   // "aws"; the leading segment, defaulted when omitted
	Service  string   // "ecs", "s3", "profiles", "q"
	Resource string   // "clusters", "services"; "" means the service has only one
	Path     []string // selector into the panel: cluster/service names, a profile name
}

// Key excludes selector paths so resource identity remains comparable.
type Key struct {
	Provider string
	Service  string
	Resource string
}

func (r Ref) Key() Key {
	return Key{Provider: r.Provider, Service: r.Service, Resource: r.Resource}
}

func (k Key) Ref() Ref {
	return Ref{Provider: k.Provider, Service: k.Service, Resource: k.Resource}
}

func (k Key) Name() string {
	parts := []string{k.Provider, k.Service}
	if k.Resource != "" {
		parts = append(parts, k.Resource)
	}
	return strings.Join(parts, Separator)
}

func (r Ref) String() string {
	return Separator + strings.Join(append([]string{r.Key().Name()}, r.Path...), Separator)
}

// Split normalizes empty and padded segments but preserves slashes because only Separator divides a ref.
func Split(input string) []string {
	segments := strings.Split(input, Separator)
	for i, segment := range segments {
		segments[i] = strings.TrimSpace(segment)
	}

	kept := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment != "" {
			kept = append(kept, segment)
		}
	}

	return kept
}
