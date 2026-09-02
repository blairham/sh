// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
)

// A word that expands to a path something else can open, rather than to text.
// Every case here is measured against bash in internal/oracle; what these add
// is the parts a corpus case cannot see, like what is left in the temporary
// directory afterwards.
func TestProcessSubstitutionReadsACommandAsAFile(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"one", `cat <(echo hi)`, "hi\n"},
		// Two of them in one command each need their own pipe. The first
		// attempt shared the runner's pending list through clone(), so the
		// second substitution closed the first one's end and `one` was lost.
		{"two", `cat <(echo one) <(echo two)`, "one\ntwo\n"},
		{"nested in a substitution", `cat <(cat <(echo deep))`, "deep\n"},
		// The reason it exists rather than a pipeline: the loop runs in this
		// shell, so what it read is still here afterwards.
		{"feeds a loop in this shell", `while read -r l; do n=$((n+1)); done < <(printf "a\nb\nc\n"); echo "$n"`, "3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// The other direction: a path to write *to*, with a command on the far end.
//
// Through a file rather than the shell's stdout, because the substitution runs
// beside the command that named it — reading its output here would be reading
// it while it is still being written.
func TestProcessSubstitutionWritesIntoACommand(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	if _, st := run(t, `echo hi > >(tr a-z A-Z > `+out+`)`, nil); st != 0 {
		t.Fatalf("status %d", st)
	}
	if got := waitForFile(t, out); got != "HI\n" {
		t.Errorf("%s = %q, want %q — the command on the far end never ran", out, got, "HI\n")
	}
}

// What the path names is a pipe, and not a temporary file the shell filled in
// first. The difference is the whole point: a file would have to be complete
// before the reader started, and most things worth substituting are longer
// than that is affordable for.
func TestProcessSubstitutionExpandsToAPipe(t *testing.T) {
	out, st := run(t, `[ -p <(true) ] && echo pipe`, nil)
	if st != 0 || out != "pipe\n" {
		t.Errorf("out = %q status %d, want a pipe", out, st)
	}
}

// The pipe goes away with the command that named it, not at exit — a session
// that runs one in a loop would otherwise fill its directory.
func TestProcessSubstitutionRemovesItsPipe(t *testing.T) {
	out, st := run(t, `p=$(echo <(true)); [ -e "$p" ] && echo still || echo gone`, nil)
	if st != 0 || out != "gone\n" {
		t.Errorf("out = %q status %d, want the pipe removed with its command", out, st)
	}
}

// CleanUp takes the directory as well. A library caller running many scripts
// on one Runner has nothing else that would.
func TestProcessSubstitutionCleanUpRemovesTheDirectory(t *testing.T) {
	// The directory is made under the system's temporary one, so pointing
	// that at a fresh directory is what makes it findable from here.
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	var r *Runner
	if _, st := run(t, `cat <(echo hi) >/dev/null`, func(rr *Runner) { r = rr }); st != 0 {
		t.Fatalf("status %d", st)
	}
	if n := len(subdirs(t, dir)); n != 1 {
		t.Fatalf("%d directories under TMPDIR, want the one the shell made", n)
	}
	r.CleanUp()
	if n := len(subdirs(t, dir)); n != 0 {
		t.Errorf("%d directories left after CleanUp, want none", n)
	}
}

// The path is a filename and reaches the command as one. A temporary
// directory whose name holds a glob character is not hypothetical — TMPDIR is
// whatever the user's environment says — and without escaping the path is
// matched as a pattern, matches nothing, and is passed through as itself only
// by the accident that an unmatched pattern usually is.
func TestProcessSubstitutionPathIsNotGlobbed(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "od[d]dir")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", dir)
	out, st := run(t, `cat <(echo hi)`, func(r *Runner) {
		sem := CoreSemantics()
		// A pattern matching nothing is an error here, which is what turns
		// the unescaped path from silently wrong into visible.
		sem.GlobNoMatchIsError = Yes
		sem.GlobExpansionResults = Yes
		r.Semantics = &sem
	})
	if st != 0 || out != "hi\n" {
		t.Errorf("out = %q status %d, want the path used as a name", out, st)
	}
}

func subdirs(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ents
}

// waitForFile reads a file the substitution is filling in beside us.
func waitForFile(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		b, err := os.ReadFile(path)
		if err == nil && len(b) > 0 {
			return string(b)
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			return strings.TrimSpace(string(b))
		}
		time.Sleep(5 * time.Millisecond)
	}
}
