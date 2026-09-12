// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// TestAPersonsRunCommandsFileIsRead is #807 measured against the binary rather
// than against the library.
//
// The distinction is the point: the dialect's prelude is shell, so it moves
// shell state exactly the way a script does, and a startup rule that works with
// no prelude can still do nothing in the shell a person runs. This dialect has
// four startup files where the other has two, so the ordering between them is
// the part worth pinning here.
func TestAPersonsRunCommandsFileIsRead(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, ".zshrc", strings.Join([]string{
		"alias ll='echo alias-ran'",
		"myfunc() { echo function-ran; }",
		"export FROMRC=yes",
	}, "\n")+"\n")

	out, errs, code := prompt(t, "ll\nmyfunc\necho \"FROMRC=$FROMRC\"\n", "zsh", "-i")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	for _, want := range []string{"alias-ran", "function-ran", "FROMRC=yes"} {
		if !strings.Contains(out, want) {
			t.Errorf("out = %q, want %q — the run-commands file did not take effect", out, want)
		}
	}
}

// The four files, in the measured order, with the run-commands file between the
// two login ones — and the unconditional one read even by a shell with a command
// string to run and nobody to prompt.
func TestTheFourStartupFilesAndTheirOrder(t *testing.T) {
	home := scratchHome(t)
	for _, n := range []string{".zshenv", ".zprofile", ".zshrc", ".zlogin"} {
		writeHomeFile(t, home, n, "echo read"+n+"\n")
	}

	out, _, _ := prompt(t, "", "-zsh", "-i")
	want := "read.zshenv\nread.zprofile\nread.zshrc\nread.zlogin\n"
	if !strings.Contains(out, want) {
		t.Errorf("out = %q, want %q", out, want)
	}

	out, _, _ = prompt(t, "", "zsh", "-i")
	if !strings.Contains(out, "read.zshenv\nread.zshrc\n") || strings.Contains(out, ".zlogin") {
		t.Errorf("out = %q, want the two a non-login prompt reads", out)
	}

	out, _, _ = prompt(t, "", "zsh", "-c", ":")
	if got := strings.TrimSpace(out); got != "read.zshenv" {
		t.Errorf("out = %q, want the unconditional file alone", got)
	}
}

// The directory variable moves every file at once, and it is read afresh for
// each one — which is what makes a person's own unconditional file able to set
// it and have the rest follow.
func TestTheDirectoryVariableMovesEveryFile(t *testing.T) {
	home := scratchHome(t)
	elsewhere := t.TempDir()
	for _, n := range []string{".zshrc", ".zlogin"} {
		writeHomeFile(t, elsewhere, n, "echo moved"+n+"\n")
	}
	writeHomeFile(t, home, ".zshrc", "echo home.zshrc\n")
	writeHomeFile(t, home, ".zshenv", "echo read.zshenv\nexport ZDOTDIR="+elsewhere+"\n")

	out, _, _ := prompt(t, "", "-zsh", "-i")
	want := "read.zshenv\nmoved.zshrc\nmoved.zlogin\n"
	if !strings.Contains(out, want) {
		t.Errorf("out = %q, want %q", out, want)
	}
	if strings.Contains(out, "home.zshrc") {
		t.Errorf("out = %q, want the home directory's file left unread", out)
	}
}

// A file that breaks has to be escapable, and here the escape drops every one
// of the four rather than only the run-commands file.
func TestABrokenStartupFileIsEscapable(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, ".zshrc", "if\n")
	writeHomeFile(t, home, ".zshenv", "echo read.zshenv\n")

	if _, errs, code := prompt(t, "", "zsh", "-i"); code == 0 || !strings.Contains(errs, ".zshrc") {
		t.Errorf("a broken file gave status %d and %q, want it reported", code, errs)
	}
	out, _, code := prompt(t, "echo alive\n", "zsh", "-f", "-i")
	if code != 0 || !strings.Contains(out, "alive") {
		t.Errorf("-f gave status %d and %q, want a working session", code, out)
	}
	if strings.Contains(out, "read.zshenv") {
		t.Errorf("out = %q, want every startup file skipped, not only the broken one", out)
	}
}

// scratchHome points HOME at a directory of this test's own. A test that read
// the developer's real home directory would be measuring their machine, and
// this dialect's directory variable is one a developer's own shell may export.
func scratchHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	// t.Setenv registers the restore; the unset that follows is what is
	// wanted, and there is no t.Unsetenv.
	for _, name := range []string{"ENV", "ZDOTDIR"} {
		t.Setenv(name, "")
		_ = os.Unsetenv(name)
	}
	return dir
}

func writeHomeFile(t *testing.T, home, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// prompt drives the binary's own shell value through an interactive session,
// with the typed lines on a pipe standing in for a person.
func prompt(t *testing.T, typed string, argv ...string) (out, errs string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &o, &e
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString(typed)
		_ = w.Close()
	}()
	t.Cleanup(func() { _ = r.Close() })
	sh.Stdin = r
	code = driver.MainArgs(sh, argv)
	return o.String(), e.String(), code
}

// The fourth route into the function search, and the one that matters: a real
// startup file, read by the real front end, autoloading the names this shell
// ships and calling them.
//
// The other three — a bare call, "autoload -Uz" then a call, a call from
// inside another function — are graded in dialect/zsh against the library.
// This one cannot be: what a startup file is, and when it is read, is the
// front end's to know, and a payload that works under "-c" and not at startup
// is exactly the shape #1968 is about. The names here are the four a real
// ~/.zshrc reaches before it has done anything of its own.
func TestTheShippedFunctionsAreReachableFromAStartupFile(t *testing.T) {
	home := scratchHome(t)
	t.Setenv("FPATH", shippedFunctionDir(t))
	writeHomeFile(t, home, ".zshrc", strings.Join([]string{
		"autoload -Uz is-at-least add-zsh-hook colors regexp-replace",
		`is-at-least 5.0 5.9 && echo "rc: is-at-least ok"`,
		"hookfn() { : }",
		`add-zsh-hook precmd hookfn && echo "rc: add-zsh-hook ok ${precmd_functions[*]}"`,
		`colors && echo "rc: colors ok ${#fg}"`,
		`v=abc; regexp-replace v b X && echo "rc: regexp-replace ok $v"`,
	}, "\n")+"\n")

	out, errs, code := prompt(t, "", "zsh", "-i")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	for _, want := range []string{
		"rc: is-at-least ok",
		"rc: add-zsh-hook ok hookfn",
		"rc: colors ok 11",
		"rc: regexp-replace ok aXc",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("out = %q (stderr %q), want %q", out, errs, want)
		}
	}
	// And nothing complained on the way. "function definition file not found"
	// is what a startup file got for every one of these names before the
	// files existed, and it is the whole of what this is here to notice.
	if strings.Contains(errs, "definition file not found") {
		t.Errorf("stderr = %q, want no unresolved autoload", errs)
	}
}

// shippedFunctionDir is where the function files this shell installs live in
// the checkout, found from this file's own path so the answer does not depend
// on where "go test" was started.
func shippedFunctionDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information: cannot find the shipped function files")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "share", "sh", "functions")
	if _, err := os.Stat(filepath.Join(dir, "is-at-least")); err != nil {
		t.Fatalf("shipped function directory %s: %v", dir, err)
	}
	return dir
}

// The two halves of #1282 joined, which is the only place they meet.
//
// The shipped `add-zsh-hook` puts a name in `precmd_functions` (#1968) and
// the prompt loop reads that array (#1281). Each half is graded on its own
// and neither grading can see the join: dialect/zsh drives the shipped file
// and looks at the array it wrote, and repl/hooks_test.go fires an array a
// test filled in Go. Both passed, in that order, while this shell was in
// exactly the state #1281 measured — registration byte-identical to zsh's,
// and not one hook ever run.
//
// So this drives a startup file that registers hooks the way every plugin in
// the world does, then types a command, and asks whether the functions ran.
// It is the acceptance test for the decision in #1282: a library whose
// registrations nothing reads is a stub in all but spelling.
func TestAHookTheShippedFunctionRegisteredFiresAtThePrompt(t *testing.T) {
	home := scratchHome(t)
	t.Setenv("FPATH", shippedFunctionDir(t))
	writeHomeFile(t, home, ".zshrc", strings.Join([]string{
		"autoload -Uz add-zsh-hook",
		`beforeprompt() { print -r -- "MARK-PRECMD" }`,
		`beforecommand() { print -r -- "MARK-PREEXEC $1" }`,
		"add-zsh-hook precmd beforeprompt",
		"add-zsh-hook preexec beforecommand",
	}, "\n")+"\n")

	out, errs, code := prompt(t, "echo body\nexit\n", "zsh", "-i")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}

	// Two prompts are drawn — the first one and the one after `echo body` —
	// so the prompt hook runs twice. A count rather than a Contains, because
	// a hook that fires once and a hook that fires at every prompt are the
	// same string and a different shell.
	if n := strings.Count(out, "MARK-PRECMD"); n != 2 {
		t.Errorf("prompt hook ran %d times, want 2; out = %q (stderr %q)", n, out, errs)
	}

	// The command hook is told the line, which is what a title-setting hook
	// is for and is the whole of what it receives.
	if !strings.Contains(out, "MARK-PREEXEC echo body") {
		t.Errorf("out = %q (stderr %q), want the command hook to be given the line", out, errs)
	}

	// And it ran *before* the command did. A hook that fired afterwards would
	// satisfy both checks above and be the wrong hook.
	if before, after := strings.Index(out, "MARK-PREEXEC echo body"), strings.Index(out, "body\n"); before < 0 || after < 0 || before > after {
		t.Errorf("out = %q: the command hook ran at %d and the command at %d, want the hook first", out, before, after)
	}
}
