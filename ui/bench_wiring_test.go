package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A benchmark in a package `make bench` does not name is not a benchmark, it is a file that compiles.
// The target's package list is written by hand, so adding a bench_test.go elsewhere silently measures nothing, and a benchmark nobody runs reads as coverage the project does not have.
func TestMakeBenchRunsEveryBenchmarkPackage(t *testing.T) {
	root := ".."

	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("reading the Makefile: %v", err)
	}

	target := benchTargetLine(string(makefile))
	if target == "" {
		t.Fatal("the Makefile has no bench target running `go test ... -bench`, so nothing here can be checked")
	}

	var found int
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "node_modules") {
			return fs.SkipDir
		}
		if entry.IsDir() || entry.Name() != "bench_test.go" {
			return nil
		}

		found++
		relative, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}

		pkg := "./" + filepath.ToSlash(relative) + "/"
		if !strings.Contains(target, pkg) {
			t.Errorf("%s holds benchmarks but %s is not in the bench target, so `make bench` never runs them:\n%s", relative, pkg, target)
		}

		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking the repository: %v", walkErr)
	}

	// Guards the walk itself: a filter that matches nothing would make every assertion above vacuous.
	if found == 0 {
		t.Fatal("no bench_test.go files found, so this test proved nothing")
	}
}

func benchTargetLine(makefile string) string {
	for _, line := range strings.Split(makefile, "\n") {
		if strings.Contains(line, "-bench") && strings.Contains(line, "go test") {
			return strings.TrimSpace(line)
		}
	}

	return ""
}
