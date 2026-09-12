// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// runKshWithPrelude is this file's route, because everything it asserts is
// either written in the prelude or reached through an alias defined there.
func runKshWithPrelude(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// The aliases this shell starts with, as `alias` lists them on a bare
// invocation of ksh93u+ 2012-08-01. Measured 2026-09-12 (#2345).
//
// The whole listing rather than a name or two, because the table is the
// measurement: a row added by hand later would not be in it, and a row lost
// would leave `autoload` or `integer` resolving to nothing with no test
// objecting.
func TestTheShellStartsWithItsOwnAliases(t *testing.T) {
	const want = `2d='set -f;_2d'
autoload='typeset -fu'
command='command '
compound='typeset -C'
fc=hist
float='typeset -lE'
functions='typeset -f'
hash='alias -t --'
history='hist -l'
integer='typeset -li'
nameref='typeset -n'
nohup='nohup '
r='hist -s'
redirect='command exec'
source='command .'
stop='kill -s STOP'
suspend='kill -s STOP $$'
times='{ { time;} 2>&1;}'
type='whence -v'
`
	out, st := runKshWithPrelude(t, "alias")
	if st != 0 || out != want {
		t.Errorf("alias =\n%s(status %d), want\n%s", out, st, want)
	}
}

// `autoload -Uz f` is zsh's spelling, and what a real script reaches this
// shell with. Through the alias it becomes `typeset -fu -Uz f`, which is two
// letters `typeset` does not have — so it names **both**, prints one usage
// block under the pair, and ends the script at 2.
//
// Measured 2026-09-12 on ksh93u+ 2012-08-01. Nineteen corpus rows were failing
// on it, and each of the three facts is needed for any of them to pass (#2345).
//
// Spelled as what the alias comes to rather than as `autoload`, because this
// helper parses the snippet before it runs the prelude and no alias defined
// there can reach it — a front end reads the two in the other order. That the
// word arrives here at all is the listing above, which is why the two tests
// are a pair.
func TestAutoloadReachesTypesetAndNamesEveryBadLetter(t *testing.T) {
	out, st := runKshWithPrelude(t, "typeset -fu -Uz f\necho after\n")
	const want = `ksh: typeset: -U: unknown option
ksh: typeset: -z: unknown option
Usage: typeset [-bflmnprstuxACHS] [-a[type]] [-i[base]] [-E[n]] [-F[n]] [-L[n]]
               [-M[mapping]] [-R[n]] [-X[n]] [-h string] [-T[tname]] [-Z[n]]
               [name[=value]...]
   Or: typeset [ options ] -f [name...]
`
	if out != want {
		t.Errorf("typeset -fu -Uz f =\n%q\nwant\n%q", out, want)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2 — and `after` must not be reached", st)
	}
}

// `alias` writes its complaint with nothing in front of it, where `command` a
// line later carries the shell's name. Both carry the usage block, and the
// refusal ends the script though POSIX marks `alias` no more special than
// `command`.
//
// Measured 2026-09-12 on ksh93u+ 2012-08-01: `alias -g x` is exactly these two
// lines at status 2, and `command -Q x` is the prefixed pair at 0 with the
// next command reached (#2345).
func TestAliasWritesItsRefusalBareAndFatal(t *testing.T) {
	out, st := runKshWithPrelude(t, "alias -g x=y\necho after\n")
	const want = "alias: -g: unknown option\nUsage: alias [-ptx] [name[=value]...]\n"
	if out != want || st != 2 {
		t.Errorf("alias -g = %q (status %d), want %q at 2", out, st, want)
	}
	out, st = runKshWithPrelude(t, "command -Q x\necho after\n")
	const wantCmd = "ksh: command: -Q: unknown option\nUsage: command [-pvxV] [command [arg ...]]\nafter\n"
	if out != wantCmd || st != 0 {
		t.Errorf("command -Q = %q (status %d), want %q at 0", out, st, wantCmd)
	}
}

// Every builtin's refusal has a usage block under it here, and a builtin with
// no entry in the table printed the complaint alone. The three below are the
// shapes: one line, the two-line `Or:` form, and a wrapped one.
func TestEveryRefusalCarriesItsUsage(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"unalias -Q x", "Usage: unalias [-a] name...\n"},
		{"pwd -Q", "Usage: pwd [-LP]\n"},
		{"whence -Q x", "Usage: whence [-afpqv] name  ...\n"},
		{"print -Q", "Usage: print [-enprsvC] [-f format] [-u fd] [string ...]\n"},
		{"getopts -Q a b", "Usage: getopts [-a name] opstring name [args...]\n"},
	} {
		out, _ := runKshWithPrelude(t, c.src)
		if !strings.HasSuffix(out, c.want) {
			t.Errorf("%s = %q, want it to end with %q", c.src, out, c.want)
		}
	}
}

// A name the keyword form will not take is named with its quotes **off**:
// `function 'a$b'` is `a$b: invalid function name` here, where bash quotes the
// word as it was written. An expansion survives, because what comes off is
// quote characters and `${w}` holds none.
//
// Measured 2026-09-12 on ksh93u+ 2012-08-01 (#2345).
func TestARefusedFunctionNameLosesItsQuotes(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`function 'a$b' { echo l; }`, "ksh: a$b: invalid function name\n"},
		{`function 'a b' { echo l; }`, "ksh: a b: invalid function name\n"},
		{`function '' { echo b; }`, "ksh: : invalid function name\n"},
		{`w=zz; function _p_${w} { echo l; }`, "ksh: _p_${w}: invalid function name\n"},
	} {
		out, _ := runKshWithPrelude(t, c.src)
		if out != c.want {
			t.Errorf("%s = %q, want %q", c.src, out, c.want)
		}
	}
}

// The three names that reach a *builtin* through the preset table, each of
// which was answering something other than what ksh93 answers once the alias
// put the word in front of it. All three are gaps the aliases exposed rather
// than caused: nothing else in the tree spelled `typeset -li`, `whence -v -p`
// or `alias -t` (#2345).
func TestThePresetAliasesReachTheRightBuiltin(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// `integer` is `typeset -li`, and `-l` beside `-i` is a *size*
		// rather than a case attribute: the name is an integer and the
		// assignment evaluates. bash 5.3.15 and zsh 5.9.2 answer the same.
		{"integer evaluates", `typeset -li i=3+4; echo "[$i]"`, "[7]\n"},
		{"integer over text", `typeset -li v=AB; echo "[$v]"`, "[0]\n"},
		// And a *later* declaration is the other question, where the three
		// disagree: the case letter wins here, which is the axis this one
		// must not be folded into.
		{"a later case letter still wins", `typeset -i i; typeset -l i; i=3+4; echo "[$i]"`, "[3+4]\n"},
		// `type` is `whence -v`, so `type -p` is `whence -v -p`: the later
		// of the two letters decides the wording, and `-p` last is silent.
		{"the later letter words it", "whence -vp nosuchzz-qq; echo st=$?", "st=1\n"},
		{"and the other way round", "whence -pv nosuchzz-qq; echo st=$?", "ksh: whence: nosuchzz-qq: not found\nst=1\n"},
		// `hash` is `alias -t --`, and the tracked table is the command
		// cache: any operand is a silent success here.
		{"hash bare", "alias -t --; echo st=$?", "st=0\n"},
		{"hash of a builtin", "alias -t -- shift; echo st=$?", "st=0\n"},
		{"hash of nothing", "alias -t -- nosuchcmd-xyz; echo st=$?", "st=0\n"},
		{"hash -r", "alias -t -- -r; echo st=$?", "st=0\n"},
	} {
		out, _ := runKshWithPrelude(t, c.src)
		if out != c.want {
			t.Errorf("%s: %s = %q, want %q", c.name, c.src, out, c.want)
		}
	}
}

// A plus word that follows a minus word on one declaration takes nothing off.
// The order is the rule: a plus word *first* removes as it does everywhere.
//
// Measured 2026-09-12 on ksh93u+ 2012-08-01. It is reachable because `integer`
// is `typeset -li` here, so `integer +i n` is a mixed-sign declaration whether
// or not anybody wrote one (#2345).
func TestAPlusWordAfterAMinusWordTakesNothingOff(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the integer letter", `typeset -li n=5; typeset -li +i n; n=3+4; echo "[$n]"`, "[7]\n"},
		{"a plus word alone still removes", `typeset -i n=5; typeset +i n; n=3+4; echo "[$n]"`, "[3+4]\n"},
		{"the export letter", `typeset -li -x e=1; typeset -li +x e; export -p`, "export e=1\n"},
		{"and alone it unexports", `typeset -x e=1; typeset +x e; export -p`, ""},
	} {
		out, _ := runKshWithPrelude(t, c.src)
		if out != c.want {
			t.Errorf("%s: %s = %q, want %q", c.name, c.src, out, c.want)
		}
	}
}
