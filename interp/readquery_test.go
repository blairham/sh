// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/pty"
	. "github.com/blairham/sh/interp"
)

// `read` with the letter that reads one key and answers whether it was a yes.
//
// Nothing here names a shell, for the reason every test in this package does
// not: what the letter is called is the dialect's, and the measurements are
// next to it in dialect/zsh. What is asserted here is the *shape* — a flag in
// the optstring, the terminal this Runner was handed, the normalized answer
// the name is left holding and the three statuses.

// readsQuery turns the letter on for a Runner, spelled as an optstring the
// way a dialect spells one. The key-count letter is on beside it because the
// two compose, and the rows below that part them need both.
func readsQuery(r *Runner) {
	s := *r.Semantics
	s.ReadOptions = "rk#qu:t:"
	r.Semantics = &s
}

// readQueryFile writes text to a file and returns a source line that opens it
// on descriptor 5, which is how every row here reaches the builtin without a
// terminal — the same route the fetched suite drives this letter through.
func readQueryFile(t *testing.T, text string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return `exec 5<` + path + `; `
}

// The key decides the status and the name is left holding the *normalized*
// answer rather than the key: a capital yes leaves a small one, and every
// other key leaves `n`.
//
// The seed is data no read could produce, so a row that read its name back
// cannot mistake "left alone" for "filled".
func TestAQueryReadAnswersYesForTheTwoSpellingsOfY(t *testing.T) {
	for _, c := range []struct {
		key  string
		want string
	}{
		{"y\n", "st=0 [y]"},
		{"Y\n", "st=0 [y]"},
		{"n\n", "st=1 [n]"},
		{"N\n", "st=1 [n]"},
		{"X\n", "st=1 [n]"},
		{"\n", "st=1 [n]"},
		{" y\n", "st=1 [n]"},
		// Only the first character is read, so the rest of the word never
		// reaches the verdict.
		{"yes\n", "st=0 [y]"},
		{"ny\n", "st=1 [n]"},
	} {
		t.Run(strings.TrimSuffix(c.key, "\n"), func(t *testing.T) {
			out, _ := run(t, readQueryFile(t, c.key)+
				`REPLY=seed; read -q -u 5; echo "st=$? [$REPLY]"`, readsQuery)
			if !strings.Contains(out, c.want) {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The verdict is on **the text that was read**, not on its first character.
//
// This is the pair that parts the two nouns, and without it the rule is
// keyed on whichever of them the writer had in mind: with no count the read
// is one character and the two readings agree on every row above. Here the
// first character is `y` in both cases and only the amount read moves, and
// the answer moves with it.
func TestAQueryReadJudgesTheTextAndNotTheFirstCharacter(t *testing.T) {
	for _, c := range []struct{ name, letter, want string }{
		{"one character of it", "-k1", "st=0 [y]"},
		{"two characters of it", "-k2", "st=1 [n]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, readQueryFile(t, "yy\n")+
				`REPLY=seed; read -q `+c.letter+` -u 5; echo "st=$? [$REPLY]"`, readsQuery)
			if !strings.Contains(out, c.want) {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// A short read is neither answer: it is a third status, with the name still
// left holding the no.
//
// The number is asserted rather than "not zero", because 1 is the ordinary no
// and reading a short read as one would be invisible to a test that only
// asked for a failure.
func TestAShortQueryReadIsItsOwnStatus(t *testing.T) {
	for _, c := range []struct{ name, src, text, want string }{
		{"nothing at all to read", `read -q -u 5`, "", "st=2 [n]"},
		{"fewer characters than asked for", `read -q -k3 -u 5`, "y\n", "st=2 [n]"},
		{"a count of nothing", `read -q -k0 -u 5`, "y\n", "st=2 [n]"},
		{"and the full read is not it", `read -q -k2 -u 5`, "y\n", "st=1 [n]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, readQueryFile(t, c.text)+
				`REPLY=seed; `+c.src+`; echo "st=$? [$REPLY]"`, readsQuery)
			if !strings.Contains(out, c.want) {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The first operand is the name, and it is the only one filled: the names
// after it are left exactly as they were.
func TestAQueryReadFillsOneNamedParameter(t *testing.T) {
	out, _ := run(t, readQueryFile(t, "y\n")+
		`REPLY=seed; a=first; b=second; read -q -u 5 a b; echo "st=$? [$a][$b][$REPLY]"`,
		readsQuery)
	if want := "st=0 [y][second][seed]"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The characters come from the terminal the shell holds and not from the
// stream in front of it, which is the whole reason the letter exists: a
// confirmation is asked of a person while the script's own input is the file
// it is working through.
//
// The same shape the key-count letter is asserted in, and for the same
// reason: a read that answered from the stream would look right on every row
// above, all of which name a descriptor.
func TestAQueryComesFromTheTerminalAndNotTheStream(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() { _ = control.Close(); _ = terminal.Close() })
	// A single byte and no newline. The letter reads a key rather than a
	// line, so a read that waited for a terminator would hang here rather
	// than answer wrongly — which is what makes this the row that says so.
	if _, err := control.WriteString("y"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(path, []byte("nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close() })
	var out strings.Builder
	run(t, `read -q; echo "st=$? [$REPLY]"`, func(r *Runner) {
		readsQuery(r)
		r.Stdin, r.Stdout, r.Stderr = in, &out, terminal
	})
	if want := "st=0 [y]"; !strings.Contains(out.String(), want) {
		t.Errorf("got %q, want %q from the terminal rather than from the stream",
			out.String(), want)
	}
}

// With no terminal on any stream it is the same refusal the key-count letter
// makes, at 1, with the name untouched and the stream in front of it unread.
//
// The wording is asserted and not only the status: 1 is also the ordinary no,
// so a test asking for a failure alone would pass for a shell that had found
// a terminal and read an `n` from it.
func TestAQueryWithNoTerminalRefuses(t *testing.T) {
	out, _ := run(t,
		`printf 'yyy' | { REPLY=seed; read -q; echo "st=$? [$REPLY]"; }`, readsQuery)
	if !strings.Contains(out, "not interactive and can't open terminal") {
		t.Errorf("got %q, want the refusal said out loud", out)
	}
	if !strings.Contains(out, "st=1 [seed]") {
		t.Errorf("got %q, want a failure with the name untouched", out)
	}
	if strings.Contains(out, "[y]") {
		t.Errorf("got %q, want the pipe left unread", out)
	}
}
