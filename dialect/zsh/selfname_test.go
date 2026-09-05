// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/driver"
)

// zsh names itself in a diagnostic, whatever it was invoked as.
//
// Measured against the binary, three ways, because one way is not enough to
// tell a fixed name from a shortened one:
//
//	$ /bin/zsh <<< 'if'
//	zsh: parse error near `\n'
//	$ ln -s /bin/zsh /tmp/d/myzsh && /tmp/d/myzsh <<< 'if'
//	zsh: parse error near `\n'
//	$ bash -c "exec -a weirdname /bin/zsh" <<< 'if'
//	zsh: parse error near `\n'
//
// Invoked by its path alone it looks like a base name; the symlink and the
// `exec -a` say otherwise. The other three shells print argv[0] whole in all
// three cases — bash under `exec -a weirdname` says `weirdname: line 2: …`.
//
// And `$0` is untouched: it is `/bin/zsh` on both routes that have no other
// name for it, which is why this is an answer of its own and not a shorter
// value for the shell's name.
//
// **The corpus cannot see any of this.** The oracle normalizes the shell's own
// name in a recorded line, so every row agrees whichever of the two is
// printed. It shows up only where a caller invokes the shell by an absolute
// path and reads what it wrote, which is what an embedder does and what this
// file does.
func TestZshNamesItselfWhateverItWasInvokedAs(t *testing.T) {
	if got := zsh.Diagnostics().SelfName; got != "zsh" {
		t.Fatalf("SelfName = %q, want zsh", got)
	}

	// Two paths that a base name would also shorten to `zsh`, and one that it
	// would not. The third is what makes this a test of a fixed name.
	for _, invokedAs := range []string{
		"/usr/local/bin/zsh",
		"/some/where/deep/on/the/machine/zsh",
		"weirdname",
	} {
		t.Run(invokedAs, func(t *testing.T) {
			// A command string and standard input: the two routes where the
			// shell is the only thing there is to name.
			if errs := zshStderr(t, invokedAs, "-c", "nosuchcommand-xyz"); !strings.HasPrefix(errs, "zsh:") {
				t.Errorf("-c said %q, want it to begin with zsh:", errs)
			}
			if errs := zshStderrOnStdin(t, invokedAs, "echo one\nif\n"); !strings.HasPrefix(errs, "zsh:") {
				t.Errorf("standard input said %q, want it to begin with zsh:", errs)
			}
		})
	}
}

// TestZshStillReportsThePathInDollarZero, which is the half a shorter name
// would have broken while every assertion above went on passing.
func TestZshStillReportsThePathInDollarZero(t *testing.T) {
	const invokedAs = "/usr/local/bin/zsh"
	var out bytes.Buffer
	sh := zshShell()
	sh.Stdout = &out
	sh.Stderr = &bytes.Buffer{}
	if code := driver.MainArgs(sh, []string{invokedAs, "-c", "echo $0"}); code != 0 {
		t.Fatalf("status %d", code)
	}
	if got := strings.TrimSpace(out.String()); got != invokedAs {
		t.Errorf("$0 = %q, want the path it was invoked by", got)
	}
}

// TestAScriptIsNamedByItsPathNotByTheShell. The name zsh gives itself is its
// own; a script keeps the one it was invoked with, which is what the binary
// does and what the other three do too.
func TestAScriptIsNamedByItsPathNotByTheShell(t *testing.T) {
	path := filepath.Join(t.TempDir(), "case.sh")
	if err := os.WriteFile(path, []byte("nosuchcommand-xyz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	errs := zshStderr(t, "/usr/local/bin/zsh", path)
	if !strings.Contains(errs, path) {
		t.Errorf("said %q, want the script's own path in it", errs)
	}
	if strings.HasPrefix(errs, "zsh:") {
		t.Errorf("said %q, want the script named rather than the shell", errs)
	}
}

func zshShell() driver.Shell {
	return driver.Shell{
		Name:        "zsh",
		Dialect:     zsh.Dialect(),
		Semantics:   zsh.Semantics(),
		Diagnostics: zsh.Diagnostics(),
		Prelude:     zsh.Prelude(),
		Register:    zsh.Apply,
		PromptStyle: zsh.PromptStyle(),
		EditorStyle: zsh.EditorStyle(),
	}
}

func zshStderr(t *testing.T, argv ...string) string {
	t.Helper()
	var errs bytes.Buffer
	sh := zshShell()
	sh.Stdout = &bytes.Buffer{}
	sh.Stderr = &errs
	driver.MainArgs(sh, argv)
	return errs.String()
}

func zshStderrOnStdin(t *testing.T, invokedAs, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	var errs bytes.Buffer
	sh := zshShell()
	sh.Stdin = f
	sh.Stdout = &bytes.Buffer{}
	sh.Stderr = &errs
	driver.MainArgs(sh, []string{invokedAs})
	return errs.String()
}
