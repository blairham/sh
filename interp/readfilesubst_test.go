// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// `$(<file)` is the file's contents and nothing was run to get them.
//
// Every assertion here is on the *bytes*, and that is the point rather than
// the style. The bug this covers (#1747) was a silent empty result: the
// construct produced "" at status 0 with nothing on standard error, so a
// script reading a file this way saw an empty file that existed. A test that
// asserted only "no diagnostic appeared", or that ran over an empty file,
// would have passed against the bug it exists to catch. Non-empty content,
// compared whole, is the only shape that separates "read the file" from
// "read nothing quietly".
func TestASubstitutionOfALoneInputRedirectionIsTheFile(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the contents", `printf 'hello\n' > f; printf "[%s]" "$(<f)"`, "[hello]"},
		{"more than one line", `printf 'a\nb\n' > f; printf "[%s]" "$(<f)"`, "[a\nb]"},
		// The same trailing-newline rule an ordinary command substitution
		// follows, and measured to be the same one rather than assumed.
		{"trailing newlines go", `printf 'A\n\n\n' > f; printf "[%s]" "$(<f)"`, "[A]"},
		{"no trailing newline", `printf 'noeol' > f; printf "[%s]" "$(<f)"`, "[noeol]"},
		{"an empty file", `: > f; printf "[%s]" "$(<f)"`, "[]"},
		// A space between the operator and the operand is the same form.
		{"a space after the operator", `printf 'hello\n' > f; printf "[%s]" "$(< f)"`, "[hello]"},
		// The older spelling of the same substitution.
		{"backquoted", "printf 'hello\\n' > f; printf \"[%s]\" \"`<f`\"", "[hello]"},
		// Standard input written out is still standard input.
		{"an explicit descriptor zero", `printf 'hello\n' > f; printf "[%s]" "$(0<f)"`, "[hello]"},
		// The operand is a redirection target, so it expands the way one
		// does — which is what makes `$(<$cache)` the idiom it is.
		{"the operand expands", `printf 'hello\n' > f; n=f; printf "[%s]" "$(<$n)"`, "[hello]"},
		{"the operand may hold a substitution", `printf 'hello\n' > f; printf "[%s]" "$(<$(echo f))"`, "[hello]"},
		// Unquoted, the result is field-split like any other substitution.
		{"unquoted, it splits", `printf 'a b\n' > f; printf "<%s>" $(<f)`, "<a><b>"},
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

// Nothing runs, which is the half a `$(cat file)` implementation would get
// wrong invisibly.
//
// With no PATH there is no `cat` to find, so a form that had been implemented
// by running one would come back empty here and report a command it could not
// find. The file still reads.
func TestALoneInputRedirectionSubstitutionRunsNoCommand(t *testing.T) {
	out, st := run(t, `printf 'hello\n' > f; PATH=; printf "[%s]" "$(<f)"`, nil)
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
	if out != "[hello]" {
		t.Errorf("out = %q, want %q — the form started a process instead of reading the file", out, "[hello]")
	}
}

// A name that will not open is reported, and the substitution's status is the
// dialect's for a redirection that failed. The result is empty either way, so
// the status and the diagnostic are the whole of what a script can see.
func TestALoneInputRedirectionSubstitutionReportsAFileItCannotOpen(t *testing.T) {
	out, st := run(t, `v=$(<nosuch); printf "st=%s v=[%s]" "$?" "$v"`, nil)
	if st != 0 {
		t.Errorf("status %d, want 0 — the script carries on", st)
	}
	if !strings.Contains(out, "nosuch") {
		t.Errorf("out = %q, want the name in a diagnostic — a file that will not open must not read as an empty one", out)
	}
	if !strings.Contains(out, "st=1 v=[]") {
		t.Errorf("out = %q, want the substitution to report failure and expand to nothing", out)
	}
}

// A read that works reports success whatever the status before it was.
func TestALoneInputRedirectionSubstitutionReportsSuccess(t *testing.T) {
	out, st := run(t, `printf 'hello\n' > f; false; v=$(<f); printf "st=%s v=[%s]" "$?" "$v"`, nil)
	if st != 0 || out != "st=0 v=[hello]" {
		t.Errorf("got %q status %d, want %q", out, st, "st=0 v=[hello]")
	}
}

// What is not the form. Each of these is an ordinary redirection with no
// command name: the file opens, nothing runs, and the substitution is empty —
// which is what the shells that have the form do with every one of them.
func TestOnlyALoneInputRedirectionIsTheFile(t *testing.T) {
	const setup = `printf 'hello\n' > f; printf 'second\n' > g; `
	for _, tc := range []struct{ name, src, want string }{
		// A command word takes the redirection as its input instead.
		{"a command word", `printf "[%s]" "$(<f echo hi)"`, "[hi]"},
		{"an assignment prefix", `printf "[%s]" "$(x=1 <f)"`, "[]"},
		{"a second redirection", `printf "[%s]" "$(<f <g)"`, "[]"},
		{"a descriptor other than zero", `printf "[%s]" "$(3<f)"`, "[]"},
		{"another command in the list", `printf "[%s]" "$(<f; :)"`, "[]"},
		{"a command before it", `printf "[%s]" "$(:; <f)"`, "[]"},
		{"a pipeline", `printf "[%s]" "$(<f | cat)"`, "[]"},
		{"a negated pipeline", `printf "[%s]" "$(! <f)"`, "[]"},
		// A here-string is a body rather than a filename, so the operator
		// being a `<` of some kind is not enough.
		{"a here-string", `printf "[%s]" "$(<<<hi)"`, "[]"},
		// An output redirection is the other direction entirely, and still
		// creates the file.
		{"an output redirection", `printf "[%s]" "$(>made)"; test -f made && printf "made"`, "[]made"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, setup+tc.src, nil)
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// The form is additive: a grammar without it reads the same text as an
// ordinary redirection with no command, which opens the file and produces
// nothing.
//
// Both halves are asserted from the one source, because the flag is the only
// difference between them. Without the "off" half nothing would notice a
// grammar flag that was never consulted.
func TestTheFileReadSubstitutionIsAGrammarFlag(t *testing.T) {
	const src = `printf 'hello\n' > f; printf "[%s]" "$(<f)"`
	on, st := runGrammar(t, src, func(d *syntax.Dialect) { d.ReadFileSubstitution = true }, nil)
	if st != 0 || on != "[hello]" {
		t.Errorf("with the flag: %q status %d, want %q", on, st, "[hello]")
	}
	off, st := runGrammar(t, src, func(d *syntax.Dialect) { d.ReadFileSubstitution = false }, nil)
	if st != 0 || off != "[]" {
		t.Errorf("without the flag: %q status %d, want %q — the redirection still opens the file and still runs nothing", off, st, "[]")
	}
}
