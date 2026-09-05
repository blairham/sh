// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/repl"
)

// // Measured: with PS1='<$LOGNAME>@ ' exported, real bash draws <bhamilton>@,
// and with PS1='<$(echo sub)>@ ' it draws <sub>@ — expanded, and expanded
// again at every prompt.

func TestPromptStyle(t *testing.T) {
	if got := bash.PromptStyle().Expand; got != true {
		t.Errorf("Expand = %v, want %v", got, true)
	}
}

// The table, as measured. Each of these was drawn by real bash at a prompt
// with PS1 set to that one code.
func TestPromptCodes(t *testing.T) {
	st := bash.PromptStyle()
	if st.Escape != '\\' {
		t.Errorf("Escape = %q, want a backslash", st.Escape)
	}
	if st.Unknown != repl.KeepBoth {
		t.Errorf("Unknown = %v, want an unknown code kept whole", st.Unknown)
	}
	if st.Privilege != "$" {
		t.Errorf("Privilege = %q, want $", st.Privilege)
	}
	for code, want := range map[rune]repl.PromptField{
		'u': repl.FieldUser, 'h': repl.FieldHost, 'H': repl.FieldHostFull,
		'w': repl.FieldCwd, 'W': repl.FieldCwdBase, 's': repl.FieldShellName,
		'$': repl.FieldPrivilege, 't': repl.FieldTime24, 'T': repl.FieldTime12,
		'A': repl.FieldTime24HM, '@': repl.FieldTime12AMPM, 'd': repl.FieldDate,
		'\\': repl.FieldEscape,
		'[':  repl.FieldNonPrintingStart, ']': repl.FieldNonPrintingEnd,
	} {
		if got := st.Codes[code]; got != want {
			t.Errorf("\\%c drew field %v, want %v", code, got, want)
		}
	}
	// Not a tab. bash's \t is the time, and nothing carries over from C.
	if st.Codes['t'] == repl.FieldTab {
		t.Error("\\t is the time in bash, not a tab")
	}
	// The clock pads its hour here, which is the half of the difference this
	// dialect owns: measured at six in the morning, bash drew 06:11:40 for
	// `\t` and 06:11 for `\A`.
	if st.Codes['t'] == repl.FieldTime24Unpadded || st.Codes['A'] == repl.FieldTime24HMUnpadded {
		t.Error("bash's clock is unpadded, and it pads")
	}
	// `\W` is the last component of the *abbreviated* path: measured in the
	// home directory itself, bash drew `~` rather than the directory's name.
	if st.Codes['W'] != repl.FieldCwdBase {
		t.Errorf("\\W draws %v, want the abbreviated last component", st.Codes['W'])
	}
}

// The two characters a prompt cannot hold literally, measured as one byte
// each, and the octal escape that names any of the others.
//
// `\e` is what every colored bash prompt is written with, so without it
// `\[\e[32m\]` draws the six characters rather than turning anything green.
func TestPromptSequences(t *testing.T) {
	st := bash.PromptStyle()
	for code, want := range map[rune]string{'e': "\x1b", 'a': "\a"} {
		if got := st.Sequences[code]; got != want {
			t.Errorf("\\%c drew %q, want %q", code, got, want)
		}
	}
	if !st.Octal {
		t.Error("three octal digits are not read; measured, \\007 drew the bell")
	}
	// zsh's visual language is not bash's: `%B` is a percent code, and bash
	// has no letter that draws a color of its own.
	if len(st.Colors) != 0 {
		t.Errorf("bash has color codes %v, and its colors are written by hand", st.Colors)
	}
}

// The version drawn and the version claimed are the same number.
//
// Measured: real bash drew 5.3 for \v and 5.3.15 for \V. A shell whose
// prompt and whose BASH_VERSION disagreed would be lying to one of them.
func TestPromptVersionMatchesTheOneClaimed(t *testing.T) {
	st := bash.PromptStyle()
	if st.Version != "5.3" || st.VersionFull != "5.3.15" {
		t.Errorf("version %q/%q, want 5.3/5.3.15", st.Version, st.VersionFull)
	}
	if !strings.Contains(bash.Prelude(), "BASH_VERSION='"+st.VersionFull+"(") {
		t.Errorf("the prelude does not claim %s", st.VersionFull)
	}
	if st.Default != `\s-\v\$ ` {
		t.Errorf("default prompt = %q, want the one real bash draws", st.Default)
	}
}

// A bare `!` is a bare `!` here: measured, only ksh93 reads it.
func TestNoHistoryCharacter(t *testing.T) {
	if got := bash.PromptStyle().History; got != 0 {
		t.Errorf("History = %q, want none", got)
	}
}
