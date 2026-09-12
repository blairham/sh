// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/pty"
	. "github.com/blairham/sh/interp"
)

// `read` with a letter that takes a number, optionally, and reads characters
// from the terminal the shell holds rather than from the stream in front of it.
//
// Nothing here names a shell, for the reason every test in this package does
// not: what the letter is called and what it means are the dialect's, and the
// measurements are next to them in dialect/zsh and in the corpus. What is
// asserted here is the *shape* — an optstring marked `#`, a terminal this
// Runner was handed, and the characters that come back.

// readsKeys turns the letter on for a Runner, spelled as an optstring the way
// a dialect spells one.
func readsKeys(r *Runner) {
	s := *r.Semantics
	s.ReadOptions = "rk#u:t:"
	r.Semantics = &s
}

// A `#` letter is a bare flag when nothing that could be its number follows
// it, and the word after it is still an operand.
func TestANumericOptionalOptionIsAFlagWithoutANumber(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(path, []byte("abcdef"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, src, want string }{
		{
			"the letter alone reads one",
			`read -k -u 5 v; echo "[$v]"`, "[a]",
		},
		{
			"a word that is not a number is the name",
			`read -k -u 5 v; echo "[$v]"`, "[a]",
		},
		{
			"a numeric word after it is the count",
			`read -k 3 -u 5 v; echo "[$v]"`, "[abc]",
		},
		{
			"attached is the count too",
			`read -k3 -u 5 v; echo "[$v]"`, "[abc]",
		},
		{
			"and option reading carries on past it",
			`read -k 2 -r -u 5 v; echo "[$v]"`, "[ab]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, `exec 5<`+path+`; `+tc.src, readsKeys)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A non-digit attached to the letter is another letter, and an unknown one is
// refused as a letter rather than swallowed as an argument.
//
// This is the branch the shape turns on. Reading "anything left in the word is
// the argument" would have made `v` a count and complained about a number,
// where the measured answer is that the letter took none and the bundle
// carried on to a letter that is not there.
func TestANumericOptionalOptionDoesNotEatALetter(t *testing.T) {
	out, status := run(t, `read -kv x`, readsKeys)
	if status == 0 {
		t.Errorf("got %q at 0, want the unknown letter refused", out)
	}
	// **Which complaint, and not merely that there was one.** Taking the rest
	// of the word unconditionally also fails, and fails with `v` in the
	// message — `number expected after -k: v` — so a test asking only for a
	// non-zero status and the letter `v` passes for the wrong reason. It did:
	// a mutation run is what asked.
	if !strings.Contains(out, "invalid option") {
		t.Errorf("got %q, want `v` refused as an option rather than read as a number", out)
	}
	if strings.Contains(out, "number expected") {
		t.Errorf("got %q, want the letter not to have been taken as the count", out)
	}
}

// But a digit attached to it commits the whole rest of the word to being the
// number, so a number with a letter stuck on the end is refused as a number.
func TestANumericOptionalOptionTakesTheWholeAttachedWord(t *testing.T) {
	out, status := run(t, `read -k2v x`, readsKeys)
	if status == 0 {
		t.Errorf("got %q at 0, want the bad number refused", out)
	}
	if !strings.Contains(out, "2v") {
		t.Errorf("got %q, want the whole attached word reported as the bad number", out)
	}
}

// The count is in whatever the locale calls a character, which is the same
// split `${#s}` answers and not a decision this letter makes on its own.
//
// Two reads rather than one, and a second variable rather than a length:
// whether `${#v}` counts characters or bytes is the same question asked of a
// different construct, so asserting on it here would be asserting twice on one
// axis and proving neither. Reading *again* asks only this one — if the first
// read had taken two bytes under a UTF-8 locale, the second would start inside
// the accented letter and answer with its tail.
func TestKeysAreCountedTheLocalesWay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(path, []byte("h\u00e9llo"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The locale is named by a plain assignment, with no export: measured,
	// that is enough for every panel member to change what a character is.
	src := `exec 5<` + path + `; read -k 2 -u 5 v; read -k 2 -u 5 w; echo "[$v][$w]"`
	t.Run("a UTF-8 locale counts characters", func(t *testing.T) {
		out, _ := run(t, `LC_ALL=en_US.UTF-8; `+src, readsKeys)
		if want := "[h\u00e9][ll]"; !strings.Contains(out, want) {
			t.Errorf("got %q, want %q — a byte count would have split the character", out, want)
		}
	})
	t.Run("a single-byte locale counts bytes", func(t *testing.T) {
		out, _ := run(t, `LC_ALL=C; `+src, readsKeys)
		// The first read takes `h` and the accented letter's lead byte; the
		// second takes its tail byte and the first `l`.
		if want := "[h\xc3][\xa9l]"; !strings.Contains(out, want) {
			t.Errorf("got %q, want %q — the locale names no multibyte encoding", out, want)
		}
	})
}

// Nothing is a terminator, a newline included, and the text goes into one name
// with no splitting and no trimming.
func TestKeysStopAtNothingAndFillOneName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(path, []byte("a b\nc d"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _ := run(t,
		`exec 5<`+path+`; w=keep; read -k 5 -u 5 v w; echo "${#v} w=[$w]"`, readsKeys)
	// Five characters — `a`, a space, `b`, the newline, `c` — in one variable,
	// and the second name untouched. Asserted by length because the value has
	// a newline in it.
	if want := "5 w=[keep]"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A short read assigns what came and reports a failure, which is the shape
// this builtin's ordinary end of input has.
func TestAShortKeyReadKeepsWhatArrived(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(path, []byte("ab"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _ := run(t, `exec 5<`+path+`; read -k 5 -u 5 v; echo "st=$? [$v]"`, readsKeys)
	if want := "st=1 [ab]"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q", out, want)
	}
	// And a count of nought is a failure rather than a satisfied read of
	// nothing — the one case where asking for less is not asking for less.
	none, _ := run(t, `exec 5<`+path+`; v=keep; read -k 0 -u 5 v; echo "st=$? [$v]"`, readsKeys)
	if want := "st=1 []"; !strings.Contains(none, want) {
		t.Errorf("got %q, want %q — nought still clears the name", none, want)
	}
}

// Without a descriptor named, the letter reads the terminal this Runner holds
// — and it finds one through *any* of the three streams, not only the one it
// is reading.
//
// That is the whole of why this is not a variation on reading a line. The
// stream in front of the builtin is a file with different text in it, the
// terminal is on standard error, and the characters come from the terminal.
// A shell that read the stream would answer `fi`.
func TestKeysComeFromTheTerminalAndNotTheStream(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() { _ = control.Close(); _ = terminal.Close() })
	if _, err := control.WriteString("XY"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(path, []byte("filed"), 0o644); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close() })
	var out strings.Builder
	run(t, `read -k 2 v; echo "[$v]"`, func(r *Runner) {
		readsKeys(r)
		r.Stdin, r.Stdout, r.Stderr = in, &out, terminal
	})
	if want := "[XY]"; !strings.Contains(out.String(), want) {
		t.Errorf("got %q, want %q from the terminal rather than from the stream", out.String(), want)
	}
}

// With no terminal on any stream, it is a refusal at 1 and the stream in front
// of it is not touched.
//
// **The wording is asserted and not only the status**, which is not fussiness:
// a status of 1 with an empty variable is also what "found the terminal and
// read nothing from it" looks like. Deleting the check that there *is* a
// terminal leaves a nil file, whose Read answers an error rather than
// panicking, so the count is never met and the status is 1 either way — a
// mutation run is what asked, and the first version of this test passed with
// the refusal removed.
func TestKeysWithNoTerminalRefuse(t *testing.T) {
	out, _ := run(t,
		`printf 'abcdef' | { v=keep; read -k 2 v; echo "st=$? [$v]"; }`, readsKeys)
	if !strings.Contains(out, "not interactive and can't open terminal") {
		t.Errorf("got %q, want the refusal said out loud", out)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want a failure", out)
	}
	if strings.Contains(out, "[ab]") {
		t.Errorf("got %q, want the pipe left unread", out)
	}
}

// And a descriptor named with -u is read as itself, with no terminal looked
// for and none needed.
func TestANamedDescriptorOverridesTheTerminal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(path, []byte("filed"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _ := run(t, `exec 5<`+path+`; read -k 2 -u 5 v; echo "[$v]"`, readsKeys)
	if want := "[fi]"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A `read -k` whose deadline expires must not leave a read parked on the
// terminal, because the next key somebody presses would be swallowed by it.
//
// This is the one place the shared timed source cannot be used. It reads on a
// goroutine — an io.Reader cannot be told to stop waiting — and abandons a
// read still in flight when the deadline passes, which its own comment calls
// "the cost of a timeout over a plain pipe […] confined to the stream the
// timeout was used on". For `read -k` the stream is the line editor's own
// input, so it is confined to nothing.
//
// Measured against the built shell through a pseudo-terminal before the fix:
// `read -k -t 1 a; read -k b` with a key pressed after the timeout **hung** —
// the byte went to the abandoned read and the second one waited for another
// that never came. Real zsh answers `b=[X]`. Driving the whole shell with the
// real plugins showed the same thing as a second Up arrow losing its escape
// byte and typing `[A` into the line.
//
// A descriptor can be asked whether a read would block, which an io.Reader
// cannot, so the terminal path waits for readability and reads only what is
// there.
func TestAnExpiredKeyReadDoesNotSwallowTheNextKey(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() { _ = control.Close(); _ = terminal.Close() })

	// Written after the first read has certainly given up, from a goroutine
	// because the second read blocks until it arrives.
	go func() {
		time.Sleep(200 * time.Millisecond)
		_, _ = control.WriteString("X")
	}()

	done := make(chan string, 1)
	go func() {
		var out strings.Builder
		run(t, `read -k -t 0.05 a; echo "a=[$a]"; read -k b; echo "b=[$b]"`,
			func(r *Runner) {
				readsKeys(r)
				r.Stdin, r.Stdout, r.Stderr = terminal, &out, &out
			})
		done <- out.String()
	}()

	select {
	case out := <-done:
		if !strings.Contains(out, "a=[]") {
			t.Errorf("got %q, want the first read to have timed out empty", out)
		}
		if !strings.Contains(out, "b=[X]") {
			t.Errorf("got %q, want the key pressed after the timeout to reach the second read", out)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the second read never returned: the expired read swallowed the key")
	}
}
