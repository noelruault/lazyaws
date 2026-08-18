package resources

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/noelruault/lazyaws/ui/fuzzy"
)

var (
	ErrEmpty   = errors.New("nothing to go to")
	ErrUnknown = errors.New("unknown resource")
	// ErrAmbiguous prevents arbitrary navigation on shared prefixes.
	ErrAmbiguous    = errors.New("ambiguous")
	ErrNotNavigable = errors.New("not navigable")

	errUnnamedAction         = errors.New("action has no name")
	errActionWithoutRun      = errors.New("action has no Run func")
	errDangerousWithoutToken = errors.New("dangerous action has no confirmation token")
)

// MaxSuggestions caps broad fuzzy matches to a terminal-sized list.
const MaxSuggestions = 10

type Entry struct {
	Ref     Ref      // canonical; Path is always nil here
	Title   string   // "ECS Clusters"
	Aliases []string // "ecs", "cluster", "clusters"

	Focus func(Ref) error

	// Actions is evaluated at menu-open time because availability depends on live state.
	Actions func() []Action
}

// Registry drives navigation and actions from one namespace so they cannot drift.
type Registry struct {
	mu      sync.RWMutex
	entries map[Key]*Entry
	names   map[string]Key
	order   []Key
	nameOrd []string

	DefaultProvider string
}

func NewRegistry(defaultProvider string) *Registry {
	return &Registry{
		entries:         map[Key]*Entry{},
		names:           map[string]Key{},
		DefaultProvider: defaultProvider,
	}
}

// Register panics on duplicate explicit names before a resource can be silently shadowed.
func (r *Registry) Register(entries ...*Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range entries {
		switch {
		case entry == nil:
			panic("resources: nil entry")
		case entry.Ref.Provider == "":
			panic("resources: entry with no provider: " + entry.Title)
		case entry.Ref.Service == "":
			panic("resources: entry with no service: " + entry.Title)
		case entry.Ref.Path != nil:
			panic("resources: registered entry must not carry a selector path: " + entry.Ref.String())
		}

		key := entry.Ref.Key()
		if _, exists := r.entries[key]; exists {
			panic("resources: duplicate resource " + key.Name())
		}
		r.entries[key] = entry
		r.order = append(r.order, key)

		r.claim(key.Name(), key, true)
		for _, alias := range entry.Aliases {
			r.claim(alias, key, true)
		}

		// Derived shortcuts may collide across providers, so the first registration wins without weakening explicit-name checks.
		if key.Resource != "" {
			r.claim(key.Service+Separator+key.Resource, key, false)
			r.claim(key.Provider+Separator+key.Service, key, false)
		}
		r.claim(key.Service, key, false)
	}
}

// claim rejects explicit collisions while derived shortcuts yield to the first provider.
func (r *Registry) claim(name string, key Key, explicit bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return
	}

	if existing, taken := r.names[name]; taken {
		if !explicit || existing == key {
			return
		}
		panic(fmt.Sprintf("resources: alias %q claimed by both %s and %s", name, existing.Name(), key.Name()))
	}

	r.names[name] = key
	r.nameOrd = append(r.nameOrd, name)
}

func (r *Registry) Get(key Key) (*Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.entries[key]
	return entry, ok
}

func (r *Registry) Entries() []*Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*Entry, 0, len(r.order))
	for _, key := range r.order {
		out = append(out, r.entries[key])
	}
	return out
}

// Resolve prefers exact names, then unambiguous prefixes, then fuzzy matches so ranking cannot override a precise query.
func (r *Registry) Resolve(input string) (Ref, error) {
	segments := Split(input)
	if len(segments) == 0 {
		return Ref{}, ErrEmpty
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for depth := len(segments); depth >= 1; depth-- {
		name := strings.ToLower(strings.Join(segments[:depth], Separator))
		key, ok := r.names[name]
		if !ok {
			continue
		}
		ref := key.Ref()
		if depth < len(segments) {
			ref.Path = segments[depth:]
		}
		return ref, nil
	}

	needle := strings.ToLower(strings.Join(segments, Separator))

	var prefixHit Key
	found := false
	for _, name := range r.nameOrd {
		if !strings.HasPrefix(name, needle) {
			continue
		}
		key := r.names[name]
		if found && key != prefixHit {
			return Ref{}, fmt.Errorf("%w: %q matches more than one resource", ErrAmbiguous, input)
		}
		prefixHit, found = key, true
	}
	if found {
		return prefixHit.Ref(), nil
	}

	if ranked := fuzzy.Rank(needle, r.nameOrd); len(ranked) > 0 {
		return r.names[ranked[0].Text].Ref(), nil
	}

	return Ref{}, fmt.Errorf("%w: %q", ErrUnknown, input)
}

// Matches stays uncapped because tab completion needs hidden candidates to compute a safe common prefix.
func (r *Registry) Matches(input string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	needle := strings.ToLower(strings.Join(Split(input), Separator))

	out := []string{}
	for _, name := range r.nameOrd {
		if strings.HasPrefix(name, needle) {
			out = append(out, name)
		}
	}

	if len(out) == 0 {
		for _, result := range fuzzy.Rank(needle, r.nameOrd) {
			out = append(out, result.Text)
		}
	}

	return out
}

func (r *Registry) Suggestions(input string) []string {
	matches := r.Matches(input)
	if len(matches) > MaxSuggestions {
		return matches[:MaxSuggestions]
	}
	return matches
}

func CommonPrefix(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}

	prefix := candidates[0]
	for _, name := range candidates[1:] {
		for !strings.HasPrefix(name, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}

	return prefix
}

func (r *Registry) FocusRef(ref Ref) error {
	entry, ok := r.Get(ref.Key())
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknown, ref)
	}
	if entry.Focus == nil {
		return fmt.Errorf("%w: %s", ErrNotNavigable, ref)
	}
	return entry.Focus(ref)
}
