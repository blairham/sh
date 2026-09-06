// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `autoload`, measured against zsh 5.9.2 (2026-09-06) under `env -i` with a
// scratch HOME and no startup files.

// fpathDir makes a directory of autoloadable files and returns its path.
func fpathDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The whole of the ordinary use: the declaration is silent and 0, **the name
// is a function at once**, and the call is what reads the file. The file's
// contents are the function's *body*, so its `$*` is the call's arguments.
func TestAutoloadDefinesTheNameAtOnceAndReadsTheFileOnTheCall(t *testing.T) {
	fp := fpathDir(t, map[string]string{"myfunc": `print -r -- "myfunc ran with [$*]"`})
	out, st := runZsh(t, t.TempDir(), `fpath=(`+fp+`)
autoload -Uz myfunc
print -r -- "1 decl st=$?"
whence -w myfunc
myfunc a b
print -r -- "2 call st=$?"
myfunc again
typeset -f myfunc`)
	want := "1 decl st=0\nmyfunc: function\nmyfunc ran with [a b]\n2 call st=0\n" +
		"myfunc ran with [again]\n" +
		"myfunc () {\n\tprint -r -- \"myfunc ran with [$*]\"\n}\n"
	if out != want || st != 0 {
		t.Errorf("autoload = %q (status %d), want %q", out, st, want)
	}
}

// A name whose file is not on `$fpath` fails **at the call**, not at the
// declaration — which is the whole shape of the builtin and the thing a
// resolve-now implementation would get wrong while passing every other line.
func TestAutoloadFailsAtTheCallAndNotAtTheDeclaration(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `fpath=()
autoload -Uz nosuchfn
print -r -- "decl st=$?"
nosuchfn 2>&1
print -r -- "call st=$?"`)
	want := "decl st=0\nnosuchfn:1: nosuchfn: function definition file not found\ncall st=1\n"
	if out != want || st != 0 {
		t.Errorf("a missing file = %q (status %d), want %q", out, st, want)
	}
}

// `$fpath` is searched in order and the first file wins.
func TestAutoloadTakesTheFirstFileOnFpath(t *testing.T) {
	first := fpathDir(t, map[string]string{"pick": `print -r -- FIRST`})
	second := fpathDir(t, map[string]string{"pick": `print -r -- SECOND`})
	out, st := runZsh(t, t.TempDir(), `fpath=(`+first+` `+second+`)
autoload -Uz pick
pick`)
	if out != "FIRST\n" || st != 0 {
		t.Errorf("the fpath order = %q (status %d), want %q", out, st, "FIRST\n")
	}
	out, st = runZsh(t, t.TempDir(), `fpath=(`+second+` `+first+`)
autoload -Uz pick
pick`)
	if out != "SECOND\n" || st != 0 {
		t.Errorf("the fpath order reversed = %q (status %d), want %q", out, st, "SECOND\n")
	}
}

// A directory on `$fpath` that has no such file is a search that goes on
// rather than a failure, which is what makes a stale entry harmless.
func TestAutoloadWalksPastADirectoryWithoutTheFile(t *testing.T) {
	empty := t.TempDir()
	real := fpathDir(t, map[string]string{"later": `print -r -- FOUND`})
	out, st := runZsh(t, t.TempDir(), `fpath=(`+empty+` /nonexistent-dir `+real+`)
autoload -Uz later
later`)
	if out != "FOUND\n" || st != 0 {
		t.Errorf("a stale fpath entry = %q (status %d), want %q", out, st, "FOUND\n")
	}
}

// `+X` resolves now and does **not** run, which is the difference between the
// two signs of the same letter.
func TestPlusXResolvesWithoutRunning(t *testing.T) {
	fp := fpathDir(t, map[string]string{"noisy": `print -r -- RAN`})
	out, st := runZsh(t, t.TempDir(), `fpath=(`+fp+`)
autoload -Uz noisy
autoload +X noisy
print -r -- "resolved st=$?"
typeset -f noisy
noisy`)
	want := "resolved st=0\nnoisy () {\n\tprint -r -- RAN\n}\nRAN\n"
	if out != want || st != 0 {
		t.Errorf("+X = %q (status %d), want %q", out, st, want)
	}
}

// The two signs are two commands rather than one with a flag, and the four
// answers say so: `+X` with nothing is silence and 0, `-X` with nothing is
// `bad autoload`, `-X` with a *name* is `bad autoload` too — it never takes
// one — and `+X` with a name it cannot find is the not-found message.
func TestTheTwoSignsOfXAreTwoCommands(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `fpath=()
autoload +X
print -r -- "1 plus-none st=$?"
autoload -X 2>&1
print -r -- "2 minus-none st=$?"
autoload -X foo 2>&1
print -r -- "3 minus-name st=$?"
autoload +X missing 2>&1
print -r -- "4 plus-missing st=$?"`)
	want := "1 plus-none st=0\n" +
		"zsh:autoload:4: bad autoload\n2 minus-none st=1\n" +
		"zsh:autoload:6: bad autoload\n3 minus-name st=1\n" +
		"zsh:8: missing: function definition file not found\n4 plus-missing st=1\n"
	if out != want || st != 0 {
		t.Errorf("the two signs = %q (status %d), want %q", out, st, want)
	}
}

// `-X` acts on the function it is running inside, which is what the stub
// uses and what makes it work from a hand-written function too.
func TestMinusXResolvesTheFunctionItIsInside(t *testing.T) {
	fp := fpathDir(t, map[string]string{"inner": `print -r -- LOADED`})
	out, st := runZsh(t, t.TempDir(), `fpath=(`+fp+`)
inner() { builtin autoload -X; print -r -- "after st=$?"; }
inner
typeset -f inner`)
	want := "after st=0\ninner () {\n\tprint -r -- LOADED\n}\n"
	if out != want || st != 0 {
		t.Errorf("-X inside a function = %q (status %d), want %q", out, st, want)
	}
}

// A file that is found but is not a body this shell can read gets its own
// complaint, not "not found" — saying otherwise would send somebody looking
// for a file that is right there.
//
// zsh reports the parse error itself here — `badfn:1: parse error near 'fi'`
// — where this says the definition was bad without saying why. Same status,
// and the difference is recorded rather than hidden.
func TestAFileThatIsNotAFunctionBodyIsItsOwnComplaint(t *testing.T) {
	fp := fpathDir(t, map[string]string{"badfn": "if then fi fi\n"})
	out, st := runZsh(t, t.TempDir(), `fpath=(`+fp+`)
autoload -Uz badfn
badfn 2>&1
print -r -- "call st=$?"`)
	want := "badfn:1: badfn: bad function definition\ncall st=1\n"
	if out != want || st != 0 {
		t.Errorf("a bad body = %q (status %d), want %q", out, st, want)
	}
}

// `-X` acts on the **innermost** function, not the outermost. One frame deep
// cannot tell the two apart, and this is the shape that can: `inner` called
// from `outer` must replace `inner`. Walking the call stack the wrong way
// round replaced the *caller* with the callee's file, and nothing caught it —
// the generated stub uses `+X NAME` and never comes through that path, so
// only a hand-written `-X` reaches it.
//
// zsh prints `INNER` here as well: its `-X` replaces the function *and the
// shell re-enters it*, so the loaded body runs on the same call. This one
// replaces and returns, which is why the generated stub says `&& NAME "$@"`
// — the re-entry is interpreter machinery and the stub is how a body says
// the same thing. A hand-written `-X` gets the replacement without the
// re-entry, and that is the whole of the difference.
func TestMinusXResolvesTheInnermostFunction(t *testing.T) {
	fp := fpathDir(t, map[string]string{"inner": `print -r -- INNER`})
	out, st := runZsh(t, t.TempDir(), `fpath=(`+fp+`)
inner() { builtin autoload -X; print -r -- "after st=$?"; }
outer() { inner; }
outer
typeset -f inner
typeset -f outer`)
	want := "after st=0\n" +
		"inner () {\n\tprint -r -- INNER\n}\n" +
		"outer () {\n\tinner\n}\n"
	if out != want || st != 0 {
		t.Errorf("a nested -X = %q (status %d), want %q", out, st, want)
	}
}

// The bare form lists the names still waiting, and only those: a name that
// has been called is an ordinary function and this builtin has nothing left
// to say about it.
func TestBareAutoloadListsOnlyTheNamesStillWaiting(t *testing.T) {
	fp := fpathDir(t, map[string]string{"loaded": `print -r -- x`})
	out, st := runZsh(t, t.TempDir(), `fpath=(`+fp+`)
autoload -Uz loaded pending
loaded >/dev/null
plainfn() { print -r -- p; }
autoload`)
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	if !strings.Contains(out, "pending () {") {
		t.Errorf("got %q, want the waiting name listed", out)
	}
	for _, unwanted := range []string{"loaded () {", "plainfn () {"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("%q in %q, want only the names still waiting", unwanted, out)
		}
	}
}

// The letters. `-U` and `-z` are accepted and change nothing — an autoloaded
// file is parsed with this shell's own grammar either way, and every real
// script writes them. The nine zsh has and this shell does not are named as
// missing, and the rest are `bad option`.
func TestAutoloadLetters(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `autoload -Uz ok1 2>&1
print -r -- "1 Uz st=$?"
autoload -t f 2>&1
print -r -- "2 t st=$?"
autoload -k f 2>&1
print -r -- "3 k st=$?"
autoload -w f 2>&1
print -r -- "4 w st=$?"
autoload -Q f 2>&1
print -r -- "5 Q st=$?"`)
	want := "1 Uz st=0\n" +
		"zsh:autoload:3: -t is not implemented yet\n2 t st=1\n" +
		"zsh:autoload:5: -k is not implemented yet\n3 k st=1\n" +
		"zsh:autoload:7: -w is not implemented yet\n4 w st=1\n" +
		"zsh:autoload:9: bad option: -Q\n5 Q st=1\n"
	if out != want {
		t.Errorf("the letters = %q, want %q", out, want)
	}
}

// A name with a directory in it is looked for where it says and nowhere
// else, so `$fpath` plays no part.
func TestAnAutoloadNameWithAPathIsReadFromIt(t *testing.T) {
	fp := fpathDir(t, map[string]string{"direct": `print -r -- BYPATH`})
	out, st := runZsh(t, t.TempDir(), `fpath=()
autoload -Uz `+filepath.Join(fp, "direct")+`
`+filepath.Join(fp, "direct"))
	if out != "BYPATH\n" || st != 0 {
		t.Errorf("a path operand = %q (status %d), want %q", out, st, "BYPATH\n")
	}
}

// The line a plugin manager writes, in the shape it writes it — and the one
// that cost two diagnostics, because the refused declaration was followed by
// a call to a name nothing had defined.
func TestTheDeclarationAndCallAPluginManagerWrites(t *testing.T) {
	fp := fpathDir(t, map[string]string{
		"is-at-least": `[[ $1 == 5.1 ]] && return 0; return 1`,
	})
	out, st := runZsh(t, t.TempDir(), `fpath=(`+fp+`)
builtin autoload -Uz is-at-least
is-at-least 5.1 && print -r -- NEW || print -r -- OLD`)
	if out != "NEW\n" || st != 0 {
		t.Errorf("the plugin manager's two lines = %q (status %d), want %q", out, st, "NEW\n")
	}
}
