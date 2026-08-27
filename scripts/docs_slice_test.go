// Package scripts holds tests for the repo's shell tooling, which has no Go code of its own but is wired into the same `make test` gate so a silent regression is caught where every other one is.
package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The corpus carries both id schemes, a heading whose only number-bearing token is an abbreviated sha, and an id-less prose heading, because those are the three ways ownership has been read wrong.
const sliceCorpus = `# Corpus

Framing every id rests on.

### s8-refresh-engine (d8a15f4)

engine body

### s1-columns (50e891f)

columns body

### res-03-legacy (abc1234)

legacy body

### Group notes (a0bcb37)

sha-only body

### Two-column notes

prose body
`

func writeCorpus(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "corpus.md")
	if err := os.WriteFile(path, []byte(sliceCorpus), 0o600); err != nil {
		t.Fatalf("writing the corpus: %v", err)
	}

	return path
}

func slice(t *testing.T, corpus string, ids ...string) string {
	t.Helper()

	cmd := exec.Command("bash", append([]string{"docs-slice.sh", corpus}, ids...)...)
	cmd.Env = append(os.Environ(), "DOCS_ROOT="+filepath.Dir(corpus))

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docs-slice.sh %v = %v\n%s", ids, err, out)
	}

	return string(out)
}

// A slicer that recognises no id degrades to returning the whole file, which still exits 0 and still reads as a working tool: the only thing that shows it is asserting what was left OUT.
func TestDocsSliceKeepsOnlyTheRequestedIdsSections(t *testing.T) {
	corpus := writeCorpus(t)

	for _, tc := range []struct {
		name    string
		ids     []string
		want    []string
		exclude []string
	}{
		{
			name:    "letter-digit scheme",
			ids:     []string{"s8-refresh-engine"},
			want:    []string{"engine body"},
			exclude: []string{"columns body", "legacy body"},
		},
		{
			name:    "letters-digits scheme",
			ids:     []string{"res-03-legacy"},
			want:    []string{"legacy body"},
			exclude: []string{"engine body", "columns body"},
		},
		{
			name:    "several ids at once",
			ids:     []string{"s8-refresh-engine", "s1-columns"},
			want:    []string{"engine body", "columns body"},
			exclude: []string{"legacy body"},
		},
		{
			name: "an id-less heading and a sha-only heading are shared framing",
			ids:  []string{"s8-refresh-engine"},
			want: []string{"Framing every id rests on.", "prose body", "sha-only body"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := slice(t, corpus, tc.ids...)

			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("the slice dropped %q, which %v owns or rests on:\n%s", want, tc.ids, got)
				}
			}
			for _, exclude := range tc.exclude {
				if strings.Contains(got, exclude) {
					t.Errorf("the slice kept %q, which %v does not own: the whole point is the bytes left out\n%s", exclude, tc.ids, got)
				}
			}
		})
	}
}

// Citations of the form `handoff.md:279` are written into the backlog and the review ledger and have to keep resolving against a slice, which is why the tool prints original line numbers rather than re-flowing what it keeps.
func TestDocsSliceKeepsOriginalLineNumbers(t *testing.T) {
	corpus := writeCorpus(t)

	wantLine := strings.Index(sliceCorpus, "### s8-refresh-engine")
	wantNumber := strings.Count(sliceCorpus[:wantLine], "\n") + 1

	got := slice(t, corpus, "s8-refresh-engine")

	var found bool
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "### s8-refresh-engine") {
			found = true
			if prefix, _, _ := strings.Cut(line, ":"); prefix != strconv.Itoa(wantNumber) {
				t.Errorf("the heading is numbered %q, want %d: a citation into the whole file must resolve against the slice", prefix, wantNumber)
			}
		}
	}
	if !found {
		t.Fatalf("the requested heading is not in the slice:\n%s", got)
	}
}
