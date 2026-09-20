// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// BASH_ARGC carries one element for the shell's own frame, where nothing was
// carrying one at all (#3887).
//
// Measured 2026-09-20 on bash 5.3.20 at `/opt/homebrew`, `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, **each probe in a run of its own** — which is
// load-bearing, see the note in interp.Runner.bottomFrame: that shell builds
// these two arrays on the first read and keeps what it built, so a second
// probe in the same run is answering about the first one.
//
//	bash -c 'declare -p BASH_ARGC BASH_ARGV'          ([0]="0")   ()
//	bash -c '…' nm a b                                ([0]="2")   (b a)
//	'set -- q; …' with `a b`                          ([0]="1")   (q)
//	'shift; …' with `a b`                             ([0]="1")   (b)
//
// So it is a view of the **positional parameters** rather than a snapshot of
// the invocation, and BASH_ARGV is a stack like the rest of the record, so the
// last parameter is element 0.
func TestBashArgcCarriesTheShellsOwnFrame(t *testing.T) {
	for _, tc := range []struct{ name, src, want, why string }{
		{
			"no parameters",
			`declare -p BASH_ARGC BASH_ARGV`,
			"declare -a BASH_ARGC=([0]=\"0\")\ndeclare -a BASH_ARGV=()",
			"the count is zero and the frame is still there, which is the row the " +
				"issue was filed on — `()` says no frame at all",
		},
		{
			"two parameters",
			`set -- a b; declare -p BASH_ARGC BASH_ARGV`,
			"declare -a BASH_ARGC=([0]=\"2\")\ndeclare -a BASH_ARGV=([0]=\"b\" [1]=\"a\")",
			"the count is the parameters', and BASH_ARGV is a stack so the last is element 0",
		},
		{
			"after shift",
			`set -- a b c; shift; declare -p BASH_ARGC BASH_ARGV`,
			"declare -a BASH_ARGC=([0]=\"2\")\ndeclare -a BASH_ARGV=([0]=\"c\" [1]=\"b\")",
			"a view and not a snapshot of the invocation — `shift` moves it",
		},
		{
			"read as a parameter rather than listed",
			`set -- a b c; echo "${#BASH_ARGC[@]} ${BASH_ARGC[0]} ${BASH_ARGV[0]}"`,
			"1 3 c",
			"the listing and the expansion answer from the same place, which is " +
				"what #3099 was about for the neighboring names",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runWithScriptFile(t, tc.src, "/s/main.sh"); got != tc.want {
				t.Errorf("out = %q, want %q — %s", got, tc.want, tc.why)
			}
		})
	}
}

// The controls, and the first of them is the one the fix could most easily
// have got wrong: a call **hides** the shell's own frame rather than replacing
// it, so a function does not report its own arguments here.
//
// Measured in the same cold-cache way: `bash -c 'f(){ declare -p BASH_ARGC
// BASH_ARGV; }; f x y' nm a b` is `()` and `()` — not the function's two, and
// not the shell's two either. A fix that read the positional parameters
// wherever it was asked would answer `([0]="2")` here.
func TestACallHidesTheShellsOwnFrame(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"inside a function",
			`set -- a b; f(){ declare -p BASH_ARGC BASH_ARGV; }; f x y`,
			"declare -a BASH_ARGC=()\ndeclare -a BASH_ARGV=()",
		},
		{
			"two calls deep",
			`set -- a b; g(){ declare -p BASH_ARGC; }; f(){ g z; }; f x y`,
			"declare -a BASH_ARGC=()",
		},
		{
			"and it is back once the call has returned",
			`set -- a b; f(){ :; }; f x y; declare -p BASH_ARGC`,
			"declare -a BASH_ARGC=([0]=\"2\")",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runWithScriptFile(t, tc.src, "/s/main.sh"); got != tc.want {
				t.Errorf("out = %q, want %q", got, tc.want)
			}
		})
	}
}

// The second control: with the record actually on, none of this applies —
// `shopt -s extdebug` pushes the shell's own frame for real, and the
// synthesized one must not be counted beside it.
//
// Measured 2026-09-20 on bash 5.3.20, one run each:
//
//	bash -c 'shopt -s extdebug; f(){ declare -p BASH_ARGC BASH_ARGV; }; f x y' nm a b
//	  ([0]="2" [1]="2")   ([0]="y" [1]="x" [2]="b" [3]="a")
//	bash -c 'f(){ echo "off [${BASH_ARGC[@]}]"; }; f a b; shopt -s extdebug;
//	         g(){ echo "on [${BASH_ARGC[@]}]"; }; h(){ g x y z; }; h a b'
//	  off []   on [3 2 0]
//	bash -c 'f(){ shopt -s extdebug; echo "[${BASH_ARGC[@]}]"; }; f a b'
//	  [2]
//
// The last is the sharpest: the record is turned on *inside* a call, so the
// shell's own frame is not in it and never becomes so.
func TestTheRecordedFramesAreNotDoubledByTheSynthesizedOne(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"two real frames under extdebug",
			`set -- a b; shopt -s extdebug; f(){ declare -p BASH_ARGC BASH_ARGV; }; f x y`,
			"declare -a BASH_ARGC=([0]=\"2\" [1]=\"2\")\n" +
				"declare -a BASH_ARGV=([0]=\"y\" [1]=\"x\" [2]=\"b\" [3]=\"a\")",
		},
		{
			"the trailing zero is the shell's own",
			`f(){ echo "off [${BASH_ARGC[@]}]"; }; f a b; shopt -s extdebug; ` +
				`g(){ echo "on [${BASH_ARGV[@]}] [${BASH_ARGC[@]}]"; }; h(){ g x y z; }; h a b`,
			"off []\non [z y x b a] [3 2 0]",
		},
		{
			"turned on inside a call, so the shell's own frame is not in it",
			`f(){ shopt -s extdebug; echo "[${BASH_ARGC[@]}]"; }; f a b`,
			"[2]",
		},
		{
			// The row that says a real record is read and not re-derived:
			// the entry `shopt` pushed holds the parameters as they were
			// *then*, so moving them afterwards does not move it. Measured:
			// `set -- a b; shopt -s extdebug; set -- x y z; declare -p
			// BASH_ARGC BASH_ARGV` is `([0]="2")` and `([0]="b" [1]="a")`.
			"a real bottom entry is a snapshot where the synthesized one is a view",
			`set -- a b; shopt -s extdebug; set -- x y z; declare -p BASH_ARGC BASH_ARGV`,
			"declare -a BASH_ARGC=([0]=\"2\")\ndeclare -a BASH_ARGV=([0]=\"b\" [1]=\"a\")",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runWithScriptFile(t, tc.src, ""); got != tc.want {
				t.Errorf("out = %q, want %q", got, tc.want)
			}
		})
	}
}

// And the frame is the shell's whatever route it came in by, which is what
// says this is not a fact about having a script file. Measured: `bash -c '…'
// nm a b` and `bash f.sh a b` both answer `([0]="2")`.
func TestTheShellsOwnFrameIsThereOnEveryRoute(t *testing.T) {
	for _, file := range []string{"", "/s/main.sh"} {
		var out bytes.Buffer
		f, err := syntax.Parse(`declare -p BASH_ARGC`, bash.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		sem, dg := bash.Semantics(), bash.Diagnostics()
		r := &interp.Runner{
			Semantics: &sem, Diagnostics: &dg, Stdout: &out, Stderr: &out,
			Name: "testsh", Dialect: presetDialect(), Params: []string{"a", "b"},
		}
		bash.Apply(r)
		if file != "" {
			r.SetScriptFile(file)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		want := "declare -a BASH_ARGC=([0]=\"2\")"
		if got := strings.TrimSpace(out.String()); got != want {
			t.Errorf("file %q: out = %q, want %q", file, got, want)
		}
	}
}
