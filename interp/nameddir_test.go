// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `~user` is the Runner's to answer, and a Runner that cannot answer leaves
// the word as it was written.
//
// The hook is what keeps a user database out of this package. Every row here
// answers from a table written in the test, so nothing reads the machine's
// own users — which is not fussiness: a test naming a real user passes on a
// laptop and fails on a runner that has never heard of them.
func TestAUserDatabaseIsTheRunnersToCarry(t *testing.T) {
	homes := map[string]string{"alder": "/homes/alder", "birch": "/homes/birch"}
	with := func(r *Runner) {
		r.UserHomeDir = func(name string) (string, bool) {
			dir, ok := homes[name]
			return dir, ok
		}
		r.Vars = map[string]string{"HOME": "/homes/mine"}
	}
	for _, tc := range []struct{ name, src, want string }{
		{"a user the database has", `printf "[%s]" ~alder`, "[/homes/alder]"},
		{"and the tail behind it", `printf "[%s]" ~alder/src/x`, "[/homes/alder/src/x]"},
		{"one it has not", `printf "[%s]" ~cedar`, "[~cedar]"},
		// The bare tilde is `$HOME` and never the database's, which is what
		// keeps the hook from being asked about a word it has no name for.
		{"a bare tilde is still HOME", `printf "[%s]" ~`, "[/homes/mine]"},
		{"and so is ~/tail", `printf "[%s]" ~/x`, "[/homes/mine/x]"},
		// Quoting suppresses the whole construct, exactly as it does for the
		// bare tilde.
		{"quoted, it is text", `printf "[%s]" "~alder"`, "[~alder]"},
		// Not at the head of a word, it is text too.
		{"behind something, it is text", `printf "[%s]" x~alder`, "[x~alder]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, with)
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
		})
	}
	// And with no hook at all the word is left alone, which is the library
	// default and is what this package did unconditionally before.
	out, _ := run(t, `printf "[%s]" ~alder`, func(r *Runner) {
		r.Vars = map[string]string{"HOME": "/homes/mine"}
	})
	if out != "[~alder]" {
		t.Errorf("with no hook = %q, want the word as written", out)
	}
}

// A named directory is this shell's own table, and it is asked **before** the
// user database.
//
// Measured on zsh 5.9.2, 2026-09-14: `hash -d root=/tmp; print -r -- ~root`
// is `/tmp` where the same line without the assignment is `/var/root`. The
// order is the whole of this test — a lookup that asked the database first
// would pass every other row in the file.
func TestANamedDirectoryWinsOverAUser(t *testing.T) {
	with := func(r *Runner) {
		r.UserHomeDir = func(name string) (string, bool) {
			if name == "alder" {
				return "/homes/alder", true
			}
			return "", false
		}
		r.Semantics.HashDefinesANamedDirectory = Yes
		r.Vars = map[string]string{"HOME": "/homes/mine"}
	}
	out, _ := run(t, `hash -d alder=/elsewhere; printf "[%s]" ~alder`, with)
	if out != "[/elsewhere]" {
		t.Errorf("= %q, want the named directory rather than the user", out)
	}
	// And a name only the database has still reaches it, so the table is a
	// first answer rather than a replacement.
	out, _ = run(t, `hash -d other=/elsewhere; printf "[%s]" ~alder`, with)
	if out != "[/homes/alder]" {
		t.Errorf("= %q, want the user's home", out)
	}
	// A table this dialect does not have answers nothing: the entry is never
	// written, because the letter means something else there.
	out, _ = run(t, `hash -d alder=/elsewhere; printf "[%s]" ~alder`, func(r *Runner) {
		with(r)
		r.Semantics.HashDefinesANamedDirectory = No
		r.Semantics.HashForgetsOneName = Yes
	})
	if !strings.Contains(out, "/homes/alder") {
		t.Errorf("= %q, want the user's home where the letter forgets a command", out)
	}
}

// `hash -d` is two different builtins under one letter, and the axis is what
// decides which.
func TestTheHashLetterDIsAnAxis(t *testing.T) {
	names := func(r *Runner) {
		r.Semantics.HashDefinesANamedDirectory = Yes
		r.Semantics.HashForgetsOneName = No
	}
	forgets := func(r *Runner) {
		r.Semantics.HashDefinesANamedDirectory = No
		r.Semantics.HashForgetsOneName = Yes
	}
	for _, tc := range []struct{ name, src, want string }{
		{"define and read back", `hash -d a=/tmp; printf "[%s]" ~a`, "[/tmp]"},
		{"list, sorted by name", `hash -d z=/z b=/b a=/a; hash -d`, "a=/a\nb=/b\nz=/z\n"},
		{"list as the command that puts it back", `hash -d a=/tmp; hash -dL`, "hash -d a=/tmp\n"},
		{"a second assignment stands", `hash -d a=/tmp; hash -d a=/var; printf "[%s]" ~a`, "[/var]"},
		{"an empty value is visible as quoted", `hash -d a=; hash -d`, "a=''\n"},
		{"a value that needs quoting gets it", `hash -d a='x y'; hash -d`, "a='x y'\n"},
		{"the table empties", `hash -d a=/tmp; hash -d -r; hash -d; echo done`, "done\n"},
		{"an empty table lists nothing", `hash -d`, ""},
		{"a name that is there is silent", `hash -d a=/tmp; hash -d a; echo st=$?`, "st=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, names)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
	// The complaints, which are the dialect's words and are asserted through
	// the field rather than at the report site.
	for _, tc := range []struct{ name, src, want string }{
		{"a name that is not there", `hash -d nosuch`, "no such directory name: nosuch"},
		{"a name that could never be read back", `hash -d a/b=/tmp`, "invalid character in directory name: a/b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, names)
			if !strings.Contains(out, tc.want) || st != 1 {
				t.Errorf("%s = %q at %d, want %q at 1", tc.src, out, st, tc.want)
			}
		})
	}
	// Under the other answer the same letter forgets a hashed command and
	// writes no table at all, so `~a` is the word as written — after the
	// complaint that `a=/tmp` is not a name the command hash holds, which is
	// the other reading answering the same line.
	out, _ := run(t, `hash -d a=/tmp; printf "[%s]" ~a`, forgets)
	if !strings.HasSuffix(out, "[~a]") {
		t.Errorf("= %q, want the word as written", out)
	}
	// And unanswered is refused rather than guessed at.
	if _, st := run(t, `hash -d a=/tmp`, func(r *Runner) {
		r.Semantics.HashDefinesANamedDirectory = Unspecified
		r.Semantics.HashForgetsOneName = Unspecified
	}); st != 2 {
		t.Errorf("status %d, want the unanswered letter refused", st)
	}
}

// A subshell owns its table, exactly as it owns the command hash.
func TestASubshellOwnsItsNamedDirectories(t *testing.T) {
	with := func(r *Runner) { r.Semantics.HashDefinesANamedDirectory = Yes }
	out, _ := run(t, `hash -d a=/tmp; (hash -d a=/var; printf "[%s]" ~a); printf "[%s]" ~a`, with)
	if out != "[/var][/tmp]" {
		t.Errorf("= %q, want the subshell's change kept inside it", out)
	}
}
