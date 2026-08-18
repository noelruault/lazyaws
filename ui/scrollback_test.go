package ui

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestBlockScrollbackEmitsTheSequenceOnATerminal(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() = %v", err)
	}
	defer read.Close()

	// A pipe is not a character device, so nothing may be emitted.
	blockScrollback(write)
	_ = write.Close()

	got, err := io.ReadAll(read)
	if err != nil {
		t.Fatalf("ReadAll() = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("wrote %q to a pipe, want nothing so piped output stays clean", got)
	}
}

// gocui draws by absolute position, so a single line printed by any package scrolls the screen and every later frame renders a row off. Nothing outside main may write to the terminal.
func TestNoPackageWritesToTheTerminal(t *testing.T) {
	out, err := exec.Command("grep", "-rnE",
		`fmt\.Print|fmt\.Fprint(ln|f)?\(os\.(Stdout|Stderr)|^\s*println\(`,
		"--include=*.go", "../apps", "../config", "../ui").CombinedOutput()
	if err != nil && len(out) == 0 {
		return // grep exits 1 when it finds nothing, which is the passing case
	}

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" || strings.Contains(line, "_test.go") {
			continue
		}
		// Writing into a gocui view is the supported path; only the raw terminal is off limits.
		if strings.Contains(line, "fmt.Fprint(v,") || strings.Contains(line, "fmt.Fprint(self.View,") {
			continue
		}
		t.Errorf("writes to the terminal while the TUI owns it: %s", line)
	}
}
