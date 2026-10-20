// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// runZshStartupValue runs one snippet with an environment of its own, which is
// what half of these rows are about: nine of the ten names take a value the
// shell was handed and the tenth does not.
func runZshStartupValue(t *testing.T, env []string, src string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir:  dir,
		Vars: map[string]string{"PATH": dir},
		Env:  append([]string{"HOME=" + dir, "PATH=" + dir}, env...),
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// Every name in startupvalues.go, on the three questions #4866 is counted by:
// is it there, what does it say about itself, and what does a listing write.
//
// Measured 2026-09-27 on zsh 5.9.2 under `-f` from a script file, `env -i
// PATH=/usr/bin:/bin` with a scratch `HOME`, one run. `${+NAME}` was **0**
// here for all ten before this, which is what says they were absent rather
// than described with the wrong word — the distinction #4866 was filed on.
func TestTheStartupValuesThisShellHadNoNameFor(t *testing.T) {
	for _, tc := range []struct{ name, word, row string }{
		{"KEYTIMEOUT", "integer", "typeset -i KEYTIMEOUT=40"},
		{"LISTMAX", "integer", "typeset -i LISTMAX=100"},
		{"MAILCHECK", "integer", "typeset -i MAILCHECK=60"},
		{"SAVEHIST", "integer-special", "typeset -i10 SAVEHIST=0"},
		{"TIMEFMT", "scalar", `typeset TIMEFMT='%J  %U user %S system %P cpu %*E total'`},
		{"TMPPREFIX", "scalar", "typeset TMPPREFIX=/tmp/zsh"},
		{"KEYBOARD_HACK", "scalar-special", "typeset KEYBOARD_HACK=''"},
		{"OPTARG", "scalar-special", "typeset OPTARG=''"},
		{"PS3", "scalar-special", `typeset PS3='?# '`},
		{"SPROMPT", "scalar-special", `typeset SPROMPT='zsh: correct '\''%R'\'' to '\''%r'\'' [nyae]? '`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), `print -r -- ${+`+tc.name+`}`)
			if out != "1\n" || st != 0 {
				t.Errorf("${+%s} = %q (status %d), want 1 — the name is there", tc.name, out, st)
			}
			out, st = runZsh(t, t.TempDir(), `print -r -- ${(t)`+tc.name+`}`)
			if out != tc.word+"\n" || st != 0 {
				t.Errorf("${(t)%s} = %q (status %d), want %q", tc.name, out, st, tc.word)
			}
			out, st = runZsh(t, t.TempDir(), `typeset -p `+tc.name)
			if out != tc.row+"\n" || st != 0 {
				t.Errorf("typeset -p %s = %q (status %d), want %q", tc.name, out, st, tc.row)
			}
		})
	}
}

// The value the shell was handed wins for nine of the ten, and the tenth is
// the control that keeps this from reading as "the environment always wins".
//
// Measured in the same run, one shell per cell, the attributes carried through
// and `export` joining them:
//
//	env KEYTIMEOUT=7     export -i KEYTIMEOUT=7
//	env PS3=7            export PS3=7
//	env KEYBOARD_HACK=7  typeset KEYBOARD_HACK=''
func TestAnInheritedStartupValueWinsExceptForTheKeyboardHack(t *testing.T) {
	for _, tc := range []struct{ name, env, want string }{
		{"KEYTIMEOUT", "KEYTIMEOUT=7", "7"},
		{"LISTMAX", "LISTMAX=7", "7"},
		{"SAVEHIST", "SAVEHIST=7", "7"},
		{"TIMEFMT", "TIMEFMT=7", "7"},
		{"TMPPREFIX", "TMPPREFIX=7", "7"},
		{"PS3", "PS3=7", "7"},
		{"SPROMPT", "SPROMPT=7", "7"},
		{"OPTARG", "OPTARG=7", "7"},
		// The one the environment may not supply, which is the row `$UID`
		// and `$IFS` are on.
		{"KEYBOARD_HACK", "KEYBOARD_HACK=7", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshStartupValue(t, []string{tc.env}, `print -r -- "[$`+tc.name+`]"`)
			if want := "[" + tc.want + "]\n"; out != want || st != 0 {
				t.Errorf("$%s with %s = %q (status %d), want %q", tc.name, tc.env, out, st, want)
			}
		})
	}
}

// An inherited integer is **scanned** rather than evaluated, which is the
// reading `HISTSIZE` takes and the reason importedInteger is one function.
//
// Measured 2026-09-27 over `KEYTIMEOUT`, a name with no floor of its own:
// `1+1` is one where an *assigned* `1+1` is two, `0x2` is two, and `abc` is
// zero rather than a refusal.
func TestAnInheritedStartupIntegerIsScanned(t *testing.T) {
	for _, tc := range []struct{ env, want string }{
		{"2x", "2"},
		{"1+1", "1"},
		{" 2 ", "2"},
		{"0x2", "2"},
		{"3.9", "3"},
		{"-1", "-1"},
		{"abc", "0"},
		{"08", "0"},
		{"", "0"},
	} {
		t.Run("["+tc.env+"]", func(t *testing.T) {
			out, st := runZshStartupValue(t, []string{"KEYTIMEOUT=" + tc.env}, `print -r -- $KEYTIMEOUT`)
			if want := tc.want + "\n"; out != want || st != 0 {
				t.Errorf("KEYTIMEOUT=%q in the environment = %q (status %d), want %q", tc.env, out, st, want)
			}
		})
	}
}

// `$status`, on the four listing forms and on being `$?`.
//
// `$ARGC` is in every row as the control: it has carried this seam since it
// was implemented, so a change that made every frozen produced name silent —
// or none of them — moves both columns together and a row that moves alone is
// this name's own.
func TestTheLastStatusIsAParameter(t *testing.T) {
	for _, name := range []string{"status", "ARGC"} {
		t.Run(name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), `print -r -- ${(t)`+name+`}`)
			if out != "integer-readonly-special\n" || st != 0 {
				t.Errorf("${(t)%s} = %q (status %d), want integer-readonly-special", name, out, st)
			}
			// Silent to `-p`, as $LINENO and $PPID are.
			out, st = runZsh(t, t.TempDir(), `typeset -p `+name+`; print -r -- "rc=$?"`)
			if out != "rc=0\n" || st != 0 {
				t.Errorf("typeset -p %s = %q (status %d), want no row at 0", name, out, st)
			}
			out, _ = runZsh(t, t.TempDir(), `readonly -p`)
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, name+"=") {
					t.Errorf("readonly -p wrote %q, want no row for %s", line, name)
				}
			}
			// And the two forms that do write it still do.
			for _, form := range []string{"readonly", "typeset -r"} {
				out, _ = runZsh(t, t.TempDir(), form)
				if !strings.Contains(out, name+"=") {
					t.Errorf("%s = %q, want a row for %s", form, out, name)
				}
			}
		})
	}
	// The value is `$?` at the moment of the read and not a copy taken at
	// startup, which is the whole of what the name is for.
	out, st := runZsh(t, t.TempDir(), `false; print -r -- "$status $?"`)
	if out != "1 1\n" || st != 0 {
		t.Errorf("after false, $status $? = %q (status %d), want 1 1", out, st)
	}
	out, st = runZsh(t, t.TempDir(), `f() { return 5 }; f; print -r -- $status`)
	if out != "5\n" || st != 0 {
		t.Errorf("after a function returning 5, $status = %q (status %d), want 5", out, st)
	}
	// And it is frozen: an assignment is the refusal that ends the script.
	out, st = runZsh(t, t.TempDir(), `status=3; print -r -- unreached`)
	if st == 0 || strings.Contains(out, "unreached") {
		t.Errorf("status=3 = %q at %d, want a refusal that ends the script", out, st)
	}
	wantWholeLines(t, out, "zsh:1: read-only variable: status")
}

// `$prompt` is `$PS1` under a fifth spelling, and the lower-case name is the
// only one of its family the reference has.
//
// Measured 2026-09-27, one fresh shell per direction, and `${+prompt2}`,
// `${+prompt3}`, `${+prompt4}` and `${+rprompt}` all 0 in the same run — which
// is the half that has to be measured, since a rule read off the four
// upper-case pairs would have added three names that are not there.
func TestTheLowerCasePromptIsTheSameParameterAsPS1(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `prompt=PP; print -r -- "$PS1|$PROMPT"`)
	if out != "PP|PP\n" || st != 0 {
		t.Errorf("after prompt=PP, $PS1|$PROMPT = %q (status %d), want PP|PP", out, st)
	}
	out, st = runZsh(t, t.TempDir(), `PS1=QQ; print -r -- "$prompt"`)
	if out != "QQ\n" || st != 0 {
		t.Errorf("after PS1=QQ, $prompt = %q (status %d), want QQ", out, st)
	}
	out, st = runZsh(t, t.TempDir(), `print -r -- ${(t)prompt}`)
	if out != "scalar-special\n" || st != 0 {
		t.Errorf("${(t)prompt} = %q (status %d), want scalar-special", out, st)
	}
	// The three spellings that are *not* there, which is what stops this
	// becoming a rule about lower case.
	out, st = runZsh(t, t.TempDir(), `print -r -- "${+prompt2}${+prompt3}${+prompt4}${+rprompt}"`)
	if out != "0000\n" || st != 0 {
		t.Errorf("the other lower-case prompt names = %q (status %d), want 0000", out, st)
	}
}

// And the pair `PS3` completes: `$PROMPT3` has been produced over `$PS3` since
// promptnames.go, so a shell with no `PS3` answered it with nothing where the
// reference writes `?# `.
func TestTheSelectionPromptReachesBothOfItsNames(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -r -- "[$PROMPT3][$PS3]"`)
	if out != "[?# ][?# ]\n" || st != 0 {
		t.Errorf("$PROMPT3 and $PS3 = %q (status %d), want both at '?# '", out, st)
	}
	out, st = runZsh(t, t.TempDir(), `PROMPT3=zz; print -r -- "$PS3"`)
	if out != "zz\n" || st != 0 {
		t.Errorf("after PROMPT3=zz, $PS3 = %q (status %d), want zz", out, st)
	}
}
