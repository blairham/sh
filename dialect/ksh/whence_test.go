// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// toolOnPath drops an executable named `tool` into dir, which runKsh already
// has as the whole of PATH.
func toolOnPath(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "tool")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// Bare `whence` answers with the resolution and nothing more — a builtin,
// keyword or function as its own name, an external as its path, and a missing
// name as silence at 1. Measured 2026-09-04, ksh93u+.
func TestBareWhenceIsTheResolution(t *testing.T) {
	dir := t.TempDir()
	path := toolOnPath(t, dir)
	out, st := runKsh(t, dir, `whence echo; whence if; f(){ :; }; whence f; whence tool`)
	if st != 0 || out != "echo\nif\nf\n"+path+"\n" {
		t.Errorf("out %q status %d, want the four bare resolutions", out, st)
	}
	out, st = runKsh(t, dir, `whence nosuchzz`)
	if st != 1 || out != "" {
		t.Errorf("out %q status %d, want silence at 1", out, st)
	}
}

// `whence -v` is the sentence — ksh93's `type` is spelled `whence -v`, and
// the not-found complaint names whence.
func TestWhenceVIsTheSentence(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `whence -v echo; f(){ :; }; whence -v f; whence -v if`)
	if st != 0 || out != "echo is a shell builtin\nf is a function\nif is a keyword\n" {
		t.Errorf("out %q status %d, want type's sentences", out, st)
	}
	out, st = runKsh(t, t.TempDir(), `whence -v nosuchzz`)
	if st != 1 || !strings.Contains(out, "whence: nosuchzz: not found") {
		t.Errorf("out %q status %d, want the whence-prefixed not-found at 1", out, st)
	}
}

// An alias answers as its value, quoted when it needs it — the one resolution
// the core's lookup cannot see, measured as `'ls -l'` bare and `ll is an
// alias for 'ls -l'` under -v.
func TestWhenceSpeaksForAliases(t *testing.T) {
	out, st := runKsh(t, t.TempDir(),
		`alias ll="ls -l"; whence ll; whence -v ll; alias g=grep; whence g`)
	if st != 0 || out != "'ls -l'\nll is an alias for 'ls -l'\ngrep\n" {
		t.Errorf("out %q status %d, want the quoted value, the sentence, and the bare word", out, st)
	}
}

// `-p` is the PATH search alone: a function is invisible to it, a found path
// prints bare, and with -v the path gets type's tracked-alias sentence.
func TestWhencePSearchesPathAlone(t *testing.T) {
	dir := t.TempDir()
	path := toolOnPath(t, dir)
	out, st := runKsh(t, dir, `whence -p tool; f(){ :; }; whence -p f; echo st=$?; whence -pv tool`)
	want := path + "\nst=1\ntool is a tracked alias for " + path + "\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// `-q` is the status with the words withheld.
func TestWhenceQIsQuiet(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `whence -q echo nosuchzz; echo st=$?; whence -q echo; echo st=$?`)
	if out != "st=1\nst=0\n" {
		t.Errorf("out %q status %d, want the statuses alone", out, st)
	}
}

// An unknown letter is `unknown option` with the usage line after it at 2; so
// is a whence with nothing to ask about, the bare line alone. The letters
// ksh93 has and this shell does not — -a, -f — are refused as not
// implemented rather than unknown, which would be a worse answer.
func TestWhenceRefusals(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `whence -z ls`)
	if st != 2 || !strings.Contains(out, "whence: -z: unknown option") ||
		!strings.Contains(out, "Usage: whence [-afpqv] name  ...") {
		t.Errorf("out %q status %d, want the complaint and the usage at 2", out, st)
	}
	out, st = runKsh(t, t.TempDir(), `whence`)
	if st != 2 || out != "Usage: whence [-afpqv] name  ...\n" {
		t.Errorf("out %q status %d, want the bare usage at 2", out, st)
	}
	// `-f` is the letter still missing; `-a` was on this list until #633 and
	// is built now — see TestWhenceAListsEveryResolution.
	out, st = runKsh(t, t.TempDir(), `whence -f echo`)
	if st != 2 || !strings.Contains(out, "whence: -f is not implemented yet") {
		t.Errorf("out %q status %d, want the not-implemented refusal at 2", out, st)
	}
}

// Several names answer in turn, and one that resolves to nothing marks the
// whole command's status without stopping the rest.
func TestWhenceCarriesOnPastAMiss(t *testing.T) {
	dir := t.TempDir()
	path := toolOnPath(t, dir)
	out, st := runKsh(t, dir, `whence echo nosuchzz tool`)
	if st != 1 || out != "echo\n"+path+"\n" {
		t.Errorf("out %q status %d, want both hits and the miss in the status", out, st)
	}
}

// `whence -a` is every resolution rather than the first, always in `-v`'s
// sentences, in the order ksh93 lists them. Measured 2026-09-05, ksh93u+.
//
// The last line is the one #633 recorded as needing FPATH machinery, and the
// measurement says otherwise: `N is an undefined function` appears exactly
// when the name had a builtin or function resolution *and* a PATH hit, with
// FPATH unset, empty or exported alike. A builtin with no file on PATH does
// not get it.
func TestWhenceAListsEveryResolution(t *testing.T) {
	dir := t.TempDir()
	path := toolOnPath(t, dir)
	// A file named after a builtin, which is the only way to reach the
	// builtin-and-on-PATH pair from a scratch PATH.
	echoOnPath := filepath.Join(dir, "echo")
	if err := os.WriteFile(echoOnPath, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, src, want string
		st              int
	}{
		{
			"a builtin with no file on PATH is one line",
			"whence -a whence", "whence is a shell builtin\n", 0,
		},
		{
			// The case the whole rule exists for, and the one a temp PATH
			// has to be built to reach: a builtin that is *also* a file on
			// PATH earns the plain path line and the undefined-function line
			// after it. Verified byte for byte against ksh93u+ with the same
			// directory on its PATH.
			"a builtin that is also on PATH",
			"whence -a echo",
			"echo is a shell builtin\necho is " + echoOnPath + "\n" +
				"echo is an undefined function\n", 0,
		},
		{
			"a PATH hit standing alone keeps the tracked-alias sentence",
			"whence -a tool", "tool is a tracked alias for " + path + "\n", 0,
		},
		{
			"a keyword", "whence -a if", "if is a keyword\n", 0,
		},
		{
			"an alias", "alias al='ls -l'; whence -a al", "al is an alias for 'ls -l'\n", 0,
		},
		{
			"a function", "f() { :; }; whence -a f", "f is a function\n", 0,
		},
		{
			// A function and a PATH hit: the function line, the plain path
			// rather than the tracked alias, and the undefined-function line
			// the pair earns.
			"a function that is also on PATH",
			"tool() { :; }; whence -a tool",
			"tool is a function\ntool is " + path + "\ntool is an undefined function\n", 0,
		},
		{
			// An alias hides nothing, so the function line is still there —
			// and the alias alone would not have earned the last line.
			"an alias over a function that is also on PATH",
			"alias tool=x; tool() { :; }; whence -a tool",
			"tool is an alias for x\ntool is a function\ntool is " + path +
				"\ntool is an undefined function\n", 0,
		},
		{
			// The alias without the function: a PATH hit that is not the
			// only line, and no undefined-function line.
			"an alias over a PATH hit alone",
			"alias tool=x; whence -a tool",
			"tool is an alias for x\ntool is " + path + "\n", 0,
		},
		{
			// The complaint is the same whence-prefixed line every other
			// mode gives, on standard error, which runKsh folds in.
			"a name that is nothing", "whence -a nosuchzz",
			"ksh: whence: nosuchzz: not found\n", 1,
		},
		{
			"every operand is answered", "whence -a if whence",
			"if is a keyword\nwhence is a shell builtin\n", 0,
		},
		{
			// -a already speaks in sentences, so -v adds nothing.
			"-v adds nothing", "whence -av if", "if is a keyword\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, dir, c.src)
			if out != c.want || st != c.st {
				t.Errorf("out %q status %d, want %q and %d", out, st, c.want, c.st)
			}
		})
	}
}

// `-aq` is the status with the words withheld, both ways.
func TestWhenceAQIsQuiet(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `whence -aq if`)
	if out != "" || st != 0 {
		t.Errorf("out %q status %d, want silence at 0", out, st)
	}
	out, st = runKsh(t, t.TempDir(), `whence -aq nosuchzz`)
	if out != "" || st != 1 {
		t.Errorf("out %q status %d, want silence at 1", out, st)
	}
}

// twoOnPath drops two executables of the same name into two directories and
// gives back the PATH that finds them in order, which is the shape `-a` and
// `-ap` are about.
func twoOnPath(t *testing.T) (dir, first, second string) {
	t.Helper()
	dir = t.TempDir()
	for _, sub := range []string{"d1", "d2"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sub, "dup"), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return dir, filepath.Join(dir, "d1", "dup"), filepath.Join(dir, "d2", "dup")
}

// `-a` and `-p` compose: the letter that says *how many rows* and the letter
// that says *what a row is* are different questions, and this shell answers
// both at once.
//
// Measured 2026-09-16 and again 2026-09-18 on ksh93u+ 2012-08-01 over eight
// orderings, with two copies of one name on PATH. The wording is the bare
// path when `p` is the last of `a`, `p` and `v` to appear and the sentence
// otherwise, and the not-found report follows the wording (#3198).
func TestWhenceComposesAllAndPath(t *testing.T) {
	for _, c := range []struct{ name, opts, want string }{
		{"a alone is the sentences", "-a", "dup is a tracked alias for D1\ndup is D2\n"},
		{"p alone is the first path", "-p", "D1\n"},
		{"p after a is every path", "-ap", "D1\nD2\n"},
		{"p before a is every sentence", "-pa", "dup is a tracked alias for D1\ndup is D2\n"},
		{"v last wins over p", "-apv", "dup is a tracked alias for D1\ndup is D2\n"},
		{"p last wins over v", "-avp", "D1\nD2\n"},
		{"and the same from the other orders", "-pav", "dup is a tracked alias for D1\ndup is D2\n"},
		{"p last again", "-vap", "D1\nD2\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, first, second := twoOnPath(t)
			src := "PATH=" + dir + "/d1:" + dir + "/d2\nwhence " + c.opts + " dup\n"
			out, st, err := preset.Combined(t, dialecttest.Base{Dir: dir}, src)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.ReplaceAll(strings.ReplaceAll(c.want, "D1", first), "D2", second)
			if out != want || st != 0 {
				t.Errorf("whence %s dup =\n%q at %d\nwant\n%q at 0", c.opts, out, st, want)
			}
		})
	}
}

// And the letter that restricts the answer to PATH restricts it however it is
// written: a builtin is invisible under `-ap` and under `-pa` alike, and the
// *report* of the miss is the one the wording asks for.
func TestWhencePathHidesWhatIsNotOnThePath(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{"a builtin under -ap is silence", "whence -ap shift", ""},
		{"and under -pa it is the sentence's miss", "whence -pa shift", "ksh: whence: shift: not found\n"},
		{"a missing name under -ap", "whence -ap nosuchthing_zz", ""},
		{"and under -apv", "whence -apv nosuchthing_zz", "ksh: whence: nosuchthing_zz: not found\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src+" 2>&1")
			if out != c.want || st != 1 {
				t.Errorf("%s =\n%q at %d\nwant\n%q at 1", c.src, out, st, c.want)
			}
		})
	}
}

// A pathname operand was never searched for, so this shell's own sentence —
// which says *how* the name was found — does not apply to it, and the plain
// one does. Measured 2026-09-14 and again 2026-09-18; the discriminator is
// the slash and not the hash table (#2953).
func TestAPathnameOperandIsNotATrackedAlias(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "bb"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bb", "tool"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	const plain = "./bb/tool is DIR/./bb/tool\n"
	for _, c := range []struct{ name, src, want string }{
		{"command -V", "command -V ./bb/tool", plain},
		{"type", "type ./bb/tool", plain},
		{"whence -v", "whence -v ./bb/tool", plain},
		{"whence -a", "whence -a ./bb/tool", plain},
		{"whence -pv", "whence -pv ./bb/tool", plain},
		// The control: a name that really was searched for keeps this
		// shell's own sentence, so the second wording is the operand's and
		// not a widening.
		{"a searched name keeps the tracked alias", "command -V tool", "tool is a tracked alias for DIR/tool\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			toolOnPath(t, dir)
			out, st, err := preset.Combined(t, dialecttest.Base{
				Dir: dir, Vars: map[string]string{"PATH": dir},
			}, c.src+"\n")
			if err != nil {
				t.Fatal(err)
			}
			if want := strings.ReplaceAll(c.want, "DIR", dir); out != want || st != 0 {
				t.Errorf("%s =\n%q at %d\nwant\n%q at 0", c.src, out, st, want)
			}
		})
	}
}

// A `command` reached through an expansion reports rather than runs here, and
// a written one runs. Measured 2026-09-16 and again 2026-09-18 (#3369).
func TestAnExpandedCommandReportsInsteadOfRunning(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"a written command runs", "command echo hi", "hi\n", 0},
		{"a quoted one is still written", `"command" echo hi`, "hi\n", 0},
		{"a backslashed one too", `\command echo hi`, "hi\n", 0},
		{"an expanded one names and does not run", "c=command\n$c echo hi", "echo\n", 1},
		{"through braces", "c=command\n${c} echo hi", "echo\n", 1},
		{"through quotes", `c=command` + "\n" + `"$c" echo hi`, "echo\n", 1},
		{"through a substitution", "$(echo command) echo hi", "echo\n", 1},
		{"through eval", "c=command\neval '$c echo hi'", "echo\n", 1},
		// A written reporting letter wins, so the expansion sets a default
		// rather than refusing the run outright.
		{"a written -v is unchanged", "c=command\n$c -v echo", "echo\n", 0},
		{"and so is a written -V", "c=command\n$c -V echo", "echo is a shell builtin\n", 0},
		// And a declaration behind it is not made, which is the row the
		// issue leads with.
		{"a declaration is named, not made", "c=command\n$c typeset v=1\nprint \"v=[${v-UNSET}]\"", "typeset\nv=[UNSET]\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src+"\n")
			if out != c.want || (c.status != 0 && st != c.status) {
				t.Errorf("%s =\n%q at %d\nwant\n%q at %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}
