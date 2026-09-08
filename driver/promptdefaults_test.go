// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// The table this file's tests use. Made up rather than borrowed from a real
// shell, because nothing outside dialect/ names one — and deliberately unlike
// any of them, so a fix that hard-coded `\s-\v\$ ` somewhere in driver would
// fail here rather than pass. What the *real* tables hold, and that they hold
// anything at all, is asserted where they live and graded by the corpus:
// dialect/bash/prompt_test.go, dialect/ksh/prompt_test.go and the
// prompt/default-* cases. A driver test cannot see those and must not pretend
// to (#1386).
func promptTable() interp.PromptStyle {
	return interp.PromptStyle{Default: "P1> ", DefaultContinued: "P2> "}
}

func promptShell(t *testing.T, st interp.PromptStyle, vars map[string]string) (Shell, *interp.Runner) {
	t.Helper()
	sh, r := newTestShell(t, vars)
	sh.PromptStyle = st
	return sh, r
}

// An interactive shell has the dialect's prompts in hand before the first
// startup file runs, because the most common first line of a real
// run-commands file reads one:
//
//	[ -z "$PS1" ] && return
//
// With PS1 unset there the guard fires at a prompt and the whole file is
// skipped, which is #1421.
func TestTheStartupFileSeesTheDefaultPrompts(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "rc.sh"), `SAW_PS1="${PS1-unset}"; SAW_PS2="${PS2-unset}"`+"\n")
	sh, r := promptShell(t, promptTable(), map[string]string{
		"HOME": home, "ENV": filepath.Join(home, "rc.sh"),
	})
	if code := sh.startup(r, source{interactive: true}); code != 0 {
		t.Fatalf("startup reported %d", code)
	}
	for _, c := range []struct{ name, want string }{
		{"SAW_PS1", "P1> "},
		{"SAW_PS2", "P2> "},
	} {
		// The whole value, not its length and not a suffix: an eight-character
		// default satisfies a length assertion whatever it says, and a
		// Contains cannot see a prefix somebody added.
		if got, _ := r.GetVar(c.name); got != c.want {
			t.Errorf("the startup file saw %s = %q, want %q", c.name, got, c.want)
		}
	}
}

// And the shell keeps them afterwards, which is what the prompt is drawn from.
func TestTheDefaultPromptsSurviveTheStartupFiles(t *testing.T) {
	sh, r := promptShell(t, promptTable(), nil)
	if code := sh.startup(r, source{interactive: true}); code != 0 {
		t.Fatalf("startup reported %d", code)
	}
	if got, ok := r.GetVar("PS1"); !ok || got != "P1> " {
		t.Errorf("PS1 = %q (set %v), want %q", got, ok, "P1> ")
	}
	if got, ok := r.GetVar("PS2"); !ok || got != "P2> " {
		t.Errorf("PS2 = %q (set %v), want %q", got, ok, "P2> ")
	}
}

// A startup file that suppresses itself does not suppress these. Measured
// across the panel: with `--norc`, `--noprofile` or `-f` and no startup file
// read at all, every column still has its default sitting in PS1.
func TestSuppressingTheStartupFilesKeepsTheDefaultPrompts(t *testing.T) {
	sh, r := promptShell(t, promptTable(), nil)
	if code := sh.startup(r, source{interactive: true, startup: startupFlags{none: true}}); code != 0 {
		t.Fatalf("startup reported %d", code)
	}
	if got, ok := r.GetVar("PS1"); !ok || got != "P1> " {
		t.Errorf("PS1 = %q (set %v), want %q", got, ok, "P1> ")
	}
}

// The other half of the guard, and the direction it is easy to break by
// getting the value right. Measured: bash 5.3.15, bash 3.2.57, bash under
// argv[0] `sh` and ksh93 all leave PS1 *unset* in a shell with nobody to
// prompt, on `-c` and on a script file alike — which is exactly what
// `[ -z "$PS1" ]` is written to detect. Assigning unconditionally would put a
// prompt in every script's environment and make the guard never fire.
func TestAShellWithNobodyToPromptHasNoDefaultPrompts(t *testing.T) {
	sh, r := promptShell(t, promptTable(), nil)
	if code := sh.startup(r, source{}); code != 0 {
		t.Fatalf("startup reported %d", code)
	}
	if got, ok := r.GetVar("PS1"); ok {
		t.Errorf("PS1 = %q, want it left unset with nobody to prompt", got)
	}
	if got, ok := r.GetVar("PS2"); ok {
		t.Errorf("PS2 = %q, want it left unset with nobody to prompt", got)
	}
}

// An assigned value wins, and *empty is assigned*. Measured on bash 5.3.15
// through the rc file: `PS1='INH> ' bash` reaches the rc as `INH> `, and
// `PS1= bash` reaches it empty rather than as the default. The rule is unset
// and not empty, because `PS1=` is a prompt of nothing somebody asked for —
// the same reading repl.Shell.prompt already gives an empty value.
func TestAnInheritedPromptBeatsTheDefault(t *testing.T) {
	for _, c := range []struct{ name, inherited string }{
		{"a value", "INH> "},
		{"empty, which is a prompt of nothing rather than a missing one", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			sh, r := promptShell(t, promptTable(), map[string]string{"PS1": c.inherited})
			if code := sh.startup(r, source{interactive: true}); code != 0 {
				t.Fatalf("startup reported %d", code)
			}
			if got, _ := r.GetVar("PS1"); got != c.inherited {
				t.Errorf("PS1 = %q, want the inherited %q", got, c.inherited)
			}
		})
	}
}

// A dialect that has not named a default gets none invented for it. The
// substrate's own `$ ` and `> ` stay what they have always been — something
// the drawer falls back to rather than a value written into the shell — so a
// core without a dialect is still a shell whose PS1 is unset.
func TestASilentTableAssignsNothing(t *testing.T) {
	sh, r := promptShell(t, interp.PromptStyle{}, nil)
	if code := sh.startup(r, source{interactive: true}); code != 0 {
		t.Fatalf("startup reported %d", code)
	}
	if got, ok := r.GetVar("PS1"); ok {
		t.Errorf("PS1 = %q, want nothing invented for a dialect that did not say", got)
	}
}

// One column assigns PS1 late. Measured through a pty with `$ENV` naming a
// file that prints `${PS1+set}`: ksh93 has PS2 and PS4 in hand while that file
// runs and PS1 *unset*, and reads `$ ` by the time a prompt is drawn. The
// value is the same either way and only the moment differs, which is why it is
// a row of the prompt table rather than an axis of Semantics.
func TestALateTableIsNotSeenByTheStartupFileAndIsSetAfterIt(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "rc.sh"), `SAW_PS1="${PS1-unset}"`+"\n")
	st := promptTable()
	st.DefaultsFollowTheStartupFiles = true
	sh, r := promptShell(t, st, map[string]string{
		"HOME": home, "ENV": filepath.Join(home, "rc.sh"),
	})
	if code := sh.startup(r, source{interactive: true}); code != 0 {
		t.Fatalf("startup reported %d", code)
	}
	if got, _ := r.GetVar("SAW_PS1"); got != "unset" {
		t.Errorf("the startup file saw PS1 = %q, want it still unset", got)
	}
	if got, ok := r.GetVar("PS1"); !ok || got != "P1> " {
		t.Errorf("after the startup files PS1 = %q (set %v), want %q", got, ok, "P1> ")
	}
}

// And a late table still yields to what the startup file itself assigned,
// which is the whole reason the order is worth getting right.
func TestALateTableYieldsToTheStartupFile(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "rc.sh"), "PS1='mine> '\n")
	st := promptTable()
	st.DefaultsFollowTheStartupFiles = true
	sh, r := promptShell(t, st, map[string]string{
		"HOME": home, "ENV": filepath.Join(home, "rc.sh"),
	})
	if code := sh.startup(r, source{interactive: true}); code != 0 {
		t.Fatalf("startup reported %d", code)
	}
	if got, _ := r.GetVar("PS1"); got != "mine> " {
		t.Errorf("PS1 = %q, want the startup file's own %q", got, "mine> ")
	}
}

// The default is not exported. Measured: with no inherited PS1, bash's
// `export -p` names none and a child's environment has none. An inherited
// exported one keeps its export attribute, which falls out of never touching
// it.
func TestTheDefaultPromptIsNotExported(t *testing.T) {
	sh, r := promptShell(t, promptTable(), nil)
	r.Stdout = sh.Stdout
	if code := sh.startup(r, source{interactive: true}); code != 0 {
		t.Fatalf("startup reported %d", code)
	}
	// Asked of the shell rather than of a map, because what "exported" means
	// is the shell's answer and a child's environment is where it shows.
	if code := sh.sourceText(r, "exports", "export -p\n"); code != 0 {
		t.Fatalf("listing the exports reported %d", code)
	}
	listed := sh.Stdout.(*strings.Builder).String()
	if strings.Contains(listed, "PS1") || strings.Contains(listed, "PS2") {
		t.Errorf("the default prompt was exported; the list was:\n%s", listed)
	}
}

// Two of the panel assign a prompt to a shell with nobody to prompt as well,
// and they do not assign the same thing. Measured on `-c` and on a script file
// alike with nothing inherited: dash reports PS1 `$ ` and PS2 `> `, while zsh
// 5.9.2 reports PS1 and PS2 **set and empty**. Set-and-empty is a third answer
// rather than a spelling of unset — `${PS1+set}` tells them apart, and the
// guard at the top of a real rc file is written on exactly that distinction —
// so the table says whether it assigns separately from what it assigns.
func TestADialectThatPromptsAScriptToo(t *testing.T) {
	for _, c := range []struct {
		name     string
		ps1, ps2 string
		wantSet  bool
		want1    string
		want2    string
	}{
		{name: "values, as dash has them", ps1: "$ ", ps2: "> ", wantSet: true, want1: "$ ", want2: "> "},
		{name: "set and empty, as zsh has them", wantSet: true, want1: "", want2: ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := promptTable()
			st.AssignsWithNobodyToPrompt = true
			st.DefaultWithNobodyToPrompt, st.DefaultContinuedWithNobodyToPrompt = c.ps1, c.ps2
			sh, r := promptShell(t, st, nil)
			if code := sh.startup(r, source{}); code != 0 {
				t.Fatalf("startup reported %d", code)
			}
			got1, ok1 := r.GetVar("PS1")
			if ok1 != c.wantSet || got1 != c.want1 {
				t.Errorf("PS1 = %q (set %v), want %q (set %v)", got1, ok1, c.want1, c.wantSet)
			}
			got2, ok2 := r.GetVar("PS2")
			if ok2 != c.wantSet || got2 != c.want2 {
				t.Errorf("PS2 = %q (set %v), want %q (set %v)", got2, ok2, c.want2, c.wantSet)
			}
		})
	}
}

// And such a dialect still prompts a *person* with its interactive values.
// The two are separate entries and reading the wrong one is the whole hazard:
// dash's happen to be the same text, zsh's do not.
func TestPromptingAScriptDoesNotChangeWhatAPersonGets(t *testing.T) {
	st := promptTable()
	st.AssignsWithNobodyToPrompt = true
	st.DefaultWithNobodyToPrompt, st.DefaultContinuedWithNobodyToPrompt = "script1> ", "script2> "
	sh, r := promptShell(t, st, nil)
	if code := sh.startup(r, source{interactive: true}); code != 0 {
		t.Fatalf("startup reported %d", code)
	}
	if got, _ := r.GetVar("PS1"); got != "P1> " {
		t.Errorf("PS1 = %q, want the interactive %q", got, "P1> ")
	}
	if got, _ := r.GetVar("PS2"); got != "P2> " {
		t.Errorf("PS2 = %q, want the interactive %q", got, "P2> ")
	}
}
