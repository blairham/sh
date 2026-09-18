// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// A function named after a special builtin, which this shell refuses in POSIX
// mode and defines outside it.
//
// Measured 2026-09-18 on bash 5.3.20, script files and `-c` under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` (#2987).
//
// The mode is what makes this a run-time question rather than the parser's,
// which is where ksh93's and dash's version of the refusal lives: a script may
// enter POSIX mode from inside the same input, and the definition is then
// refused by a mode that was not on when the line was read.
func TestAFunctionNamedAfterASpecialBuiltinIsRefusedInPosixMode(t *testing.T) {
	var out, errs bytes.Buffer
	run := func(argv ...string) (string, string, int) {
		out.Reset()
		errs.Reset()
		code := driver.MainArgs(bashShell(&out, &errs), argv)
		return out.String(), errs.String(), code
	}

	// Outside the mode it defines, under this shell's own name.
	o, e, st := run("bash", "-c", "export() { printf fn; }\nprintf 'after'\n")
	if o != "after" || e != "" || st != 0 {
		t.Errorf("plain: out %q err %q status %d, want it defined", o, e, st)
	}

	// Entered from inside the same input, which is the row a parse-time set
	// could not give: the mode is on by the time the definition is reached.
	o, e, st = run("bash", "-c", "set -o posix\nexport() { :; }\nprintf 'b'\n")
	if want := "bash: line 2: `export': is a special builtin\n"; e != want {
		t.Errorf("in posix mode: err %q, want %q", e, want)
	}
	if o != "" || st != 2 {
		t.Errorf("in posix mode: out %q status %d, want nothing at 2", o, st)
	}

	// And the same binary under the name that starts in the mode.
	o, e, st = run("sh", "-c", "printf 'a'\nexport() { :; }\nprintf 'b'\n")
	if o != "a" {
		t.Errorf("as sh: out %q, want the command in front of it to have run", o)
	}
	if want := "sh: line 2: `export': is a special builtin\n"; e != want {
		t.Errorf("as sh: err %q, want %q", e, want)
	}
	if st != 2 {
		t.Errorf("as sh: status %d, want 2", st)
	}

	// Leaving the mode puts the definition back, which is what says this is
	// a state and not a door.
	o, e, st = run("sh", "-c", "set +o posix\nexport() { printf fn; }\nprintf 'after'\n")
	if o != "after" || e != "" || st != 0 {
		t.Errorf("as sh, out of the mode: out %q err %q status %d", o, e, st)
	}

	// The set is this shell's roster of special builtins, and the names
	// beside it that are builtins and not special ones are defined.
	for _, name := range []string{
		"break", ":", ".", "source", "continue", "eval", "exec", "exit",
		"export", "readonly", "return", "set", "shift", "times", "trap", "unset",
	} {
		_, e, st := run("sh", "-c", name+"() { :; }\nprintf 'after'\n")
		if want := "sh: line 1: `" + name + "': is a special builtin\n"; e != want {
			t.Errorf("%s: err %q, want %q", name, e, want)
		}
		if st != 2 {
			t.Errorf("%s: status %d, want 2", name, st)
		}
	}
	for _, name := range []string{"local", "true", "read", "cd", "echo", "alias", "pwd", "typeset"} {
		o, e, st := run("sh", "-c", name+"() { :; }\nprintf 'after'\n")
		if o != "after" || e != "" || st != 0 {
			t.Errorf("%s: out %q err %q status %d, want it defined", name, o, e, st)
		}
	}

	// And a definition the script never reaches is never refused, which is
	// what says the check is at the definition.
	o, e, st = run("sh", "-c", "if false; then export() { :; }; fi\nprintf 'after'\n")
	if o != "after" || e != "" || st != 0 {
		t.Errorf("a branch never taken: out %q err %q status %d", o, e, st)
	}
}
