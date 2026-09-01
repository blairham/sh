// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBraceExpansionRunsBeforeParameterExpansion(t *testing.T) {
	// Measured, and the reason braces are scanned across spans rather than
	// within one: `{$a,2}` is a literal, an expansion and another literal,
	// and its braces are in the first and last. A brace range with a variable
	// endpoint therefore cannot work in any shell — the range is resolved
	// before the variable exists.
	tests := []struct{ src, want string }{
		{`echo {1..4}`, "1 2 3 4\n"},
		{`echo a{b,c}d`, "abd acd\n"},
		{`echo {1..3}{x,y}`, "1x 1y 2x 2y 3x 3y\n"},
		{`a=1; echo {$a,2}`, "1 2\n"},
		{`echo pre{a,b}post`, "preapost prebpost\n"},
		// Not brace expressions, and left alone rather than mangled.
		{`echo {a}`, "{a}\n"},
		{`echo "{a,b}"`, "{a,b}\n"},
		{`echo {a..}`, "{a..}\n"},
	}
	for _, tc := range tests {
		if got, _ := run(t, tc.src, nil); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestBraceRangeIsBounded(t *testing.T) {
	// A typo must not hang the shell.
	if got, _ := run(t, `echo {1..999999}`, nil); !strings.HasPrefix(got, "{1..") {
		t.Errorf("an oversized range should be left alone, got %.40q", got)
	}
}

func TestTildeExpansion(t *testing.T) {
	tests := []struct{ src, want string }{
		{`HOME=/h; echo ~`, "/h\n"},
		{`HOME=/h; echo ~/bin`, "/h/bin\n"},
		// Quoting suppresses it, and it is only special at the start of a word.
		{`HOME=/h; echo "~"`, "~\n"},
		{`HOME=/h; echo a~`, "a~\n"},
		// An assignment's value is a tilde context, which is what makes
		// PATH=~/bin work. It comes for free because the value is its own word.
		{`HOME=/h; x=~/b; echo $x`, "/h/b\n"},
		// `~user` needs a user database this package does not carry.
		{`HOME=/h; echo ~nosuchuser`, "~nosuchuser\n"},
	}
	for _, tc := range tests {
		if got, _ := run(t, tc.src, nil); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestCdIsACorePrimitive(t *testing.T) {
	// It changes the runner's own directory — the definition of a primitive,
	// and why it belongs here rather than in a dialect.
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, st := run(t, `cd `+dir+`; pwd`, nil)
	if st != 0 {
		t.Fatalf("status %d: %s", st, got)
	}
	if strings.TrimSpace(got) != dir && strings.TrimSpace(got) != real {
		t.Errorf("pwd = %q, want %q", strings.TrimSpace(got), dir)
	}
	// It records where it came from, which is what `cd -` uses. Whether the
	// move is *announced* is a dialect's answer, so this reads the last line
	// rather than the whole output: three of the four print where they went.
	if got, _ := run(t, `cd `+dir+`; cd /; cd -; pwd`, nil); lastLine(got) != dir &&
		lastLine(got) != real {
		t.Errorf("cd - gave %q", got)
	}
	if _, st := run(t, `cd /definitely/not/a/directory`, nil); st == 0 {
		t.Error("cd to a missing directory should fail")
	}
}

func TestCdDoesNotMoveTheProcess(t *testing.T) {
	// Calling os.Chdir would move every Runner in the program, which is wrong
	// for an interpreter something else embeds.
	before, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	run(t, `cd `+t.TempDir(), nil)
	after, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Errorf("the process directory moved from %q to %q", before, after)
	}
}

func TestReadSetsVariablesInTheCallingShell(t *testing.T) {
	// The other test of a primitive: a child process could not do this.
	got, _ := run(t, `printf 'a b c\n' | { read x y; printf "[%s][%s]" "$x" "$y"; }`, nil)
	// The last variable takes the remainder, which is what makes `read a b`
	// put "b c" in b.
	if got != "[a][b c]" {
		t.Errorf("got %q, want [a][b c]", got)
	}
	if got, _ := run(t, `printf 'one\n' | { read; printf "[%s]" "$REPLY"; }`, nil); got != "[one]" {
		t.Errorf("read with no name should set REPLY, got %q", got)
	}
}

// lastLine is what a command left behind once anything printed before it is
// set aside.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
