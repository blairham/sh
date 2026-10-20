// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// Five words this shell reaches only through its own preset aliases, and none
// of them is a command.
//
// `integer` is `typeset -li`, `nameref` is `typeset -n`, `functions` is
// `typeset -f`, `source` is `command .` and `times` is `{ { time;} 2>&1;}` —
// the alias table matches ksh93u+ byte for byte and always has, so the
// *literal* spelling has always agreed. What did not agree is every spelling
// an alias cannot reach, because a name that is also a builtin answers there
// and a name that is only an alias does not.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on /dev/null:
//
//	'integer' a=1        integer: not found, 127
//	cmd=integer; $cmd    the same
//	x=1; \nameref r=x    nameref: not found, 127
//	'functions'          functions: not found, 127
//	'source' /dev/null   source: not found, 127
//	'times'              times: not found, 127
//
// against a completed declaration and `st=0` for the first four here. A quoted
// word and an expanded one are two of the three routes past an alias; the
// third — `unalias` and then the word — takes the read-as-it-runs route a
// script file gets and is measured in the issue rather than pinned here, since
// this helper parses the whole snippet before it runs any of it (#3371).
//
// `float` and `compound` were already right, because this shell never had a
// builtin for either, and that agreement is what said the alias route needs no
// builtin behind it.
func TestTheWordsThisShellHasOnlyAsAliases(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			"a quoted declaration word", `'integer' a=1
print "a=[$a]"`, "ksh: integer: not found\na=[]\n", 0,
		},
		{
			"an expanded one", `cmd=integer
$cmd b=2
print "b=[$b]"`, "ksh: line 2: integer: not found\nb=[]\n", 0,
		},
		{
			"an escaped reference word", `x=1
\nameref r=x
print "r=[$r]"`, "ksh: line 2: nameref: not found\nr=[]\n", 0,
		},
		{
			"the function listing's second name", `'functions'
print "st=$?"`, "ksh: functions: not found\nst=127\n", 0,
		},
		{
			// `source` is `command .` here and nothing else: the substrate
			// registers `.`'s synonym and this dialect takes it away again.
			"the dot's synonym", `'source' /dev/null
print "st=$?"`, "ksh: source: not found\nst=127\n", 0,
		},
		{
			// And the one that costs something to say: `times` is a POSIX
			// special builtin in the substrate and in every other dialect,
			// and this shell does not have it at all — the alias runs the
			// `time` keyword in a group instead.
			"a POSIX special builtin this shell has not got", `'times'
print "st=$?"`, "ksh: times: not found\nst=127\n", 0,
		},
		{
			// The control: the literal spelling reaches the alias and the
			// declaration happens, carrying the lower-case letter the alias
			// spells — `typeset -li`, not `typeset -i`.
			"and the literal spelling still declares", `integer d=3
nameref rr=d
typeset -p d rr`, "typeset -l -i d=3\ntypeset -n rr=d\n", 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// And the same five names are not builtins at all, asked of the runner rather
// than of a script: a dialect that answered the rows above by some other route
// — a refusal wired into the dispatch, say — would pass them and still hold a
// builtin nobody can reach.
func TestTheAliasWordsAreNotRegisteredAsBuiltins(t *testing.T) {
	r := preset.Runner(dialecttest.Base{Dir: t.TempDir()})
	for _, name := range []string{"integer", "nameref", "functions", "source", "times"} {
		if _, ok := r.Builtin(name); ok {
			t.Errorf("%s is a builtin here; ksh93 has it as an alias alone", name)
		}
	}
	// The control, and it is the reason this is a list rather than a sweep:
	// `typeset` is what every one of those aliases expands to, and `.` is
	// what `source` expands to.
	for _, name := range []string{"typeset", ".", "whence", "print"} {
		if _, ok := r.Builtin(name); !ok {
			t.Errorf("%s should still be a builtin here", name)
		}
	}
}
