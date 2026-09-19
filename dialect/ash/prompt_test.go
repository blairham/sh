// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// What this shell does to a prompt parameter before drawing it.
//
// This file used to assert the opposite of every row below — an empty escape
// table and a default `PS1` of `$ `, which is the sibling's answer and not
// this applet's. Measured 2026-09-18 in the panel's own image,
// `alpine@sha256:28bd5f…`, BusyBox v1.37.0: with nothing assigned the prompt
// in `/` at uid 0 is `/ # `, so the default is `\w \$ ` and the codes in it
// are drawn (#3570).
func TestPromptStyleAnswers(t *testing.T) {
	p := ash.PromptStyle()
	if p.Expand == nil {
		t.Error("Expand is nil: measured, an inherited PS1='<$LOGNAME>@ ' draws the name")
	}
	if p.Default != `\w \$ ` || p.DefaultContinued != "> " {
		t.Errorf("defaults = %q/%q, want `\\w \\$ ` and `> `", p.Default, p.DefaultContinued)
	}
	// The value a *script* reads is the value a prompt draws, which is what
	// the two fields being the same string says. A script under this shell
	// reports both with nothing inherited.
	if !p.AssignsWithNobodyToPrompt || p.DefaultWithNobodyToPrompt != p.Default {
		t.Errorf("with nobody to prompt: assigns=%v default=%q, want true and %q",
			p.AssignsWithNobodyToPrompt, p.DefaultWithNobodyToPrompt, p.Default)
	}
	// The expansion runs first here, which is measured directly rather than
	// taken from the sibling: `x='\w'; PS1='<<$x>>'` draws the directory, so
	// a code that arrives out of a parameter is decoded.
	if !p.ExpandBeforeEscapes {
		t.Error("ExpandBeforeEscapes = false, and a code out of a parameter is drawn here")
	}
}

// The table, as measured — one probe per prompt, a marker either side of it,
// and `od` on what came back.
func TestPromptCodes(t *testing.T) {
	st := ash.PromptStyle()
	if st.Escape != '\\' {
		t.Errorf("Escape = %q, want a backslash", st.Escape)
	}
	for code, want := range map[rune]interp.PromptField{
		'u': interp.FieldUser, 'h': interp.FieldHost, 'H': interp.FieldHostFull,
		'w': interp.FieldCwd, 'W': interp.FieldCwdBase,
		'$': interp.FieldPrivilege, 'n': interp.FieldNewline,
		't': interp.FieldTime24HM, 'A': interp.FieldTime24HM,
		'T': interp.FieldTime24HM, '@': interp.FieldTime24HM,
		'[': interp.FieldNonPrintingStart, ']': interp.FieldNonPrintingEnd,
	} {
		if got := st.Codes[code]; got != want {
			t.Errorf(`\%c drew field %v, want %v`, code, got, want)
		}
	}
	// The four rows that are **not** the sibling's, each of which would read
	// as a table copied across if it were not measured.
	if st.Codes['t'] == interp.FieldTime24 {
		t.Error(`\t is HH:MM here, where bash draws HH:MM:SS`)
	}
	for _, code := range []rune{'#', '!', 's', 'd', 'j', 'l', 'V', 'q'} {
		if _, ok := st.Codes[code]; ok {
			t.Errorf(`\%c is in the table, and this shell draws the letter alone`, code)
		}
	}
	if st.Unknown != interp.DropEscape {
		t.Errorf("Unknown = %v, want the escape dropped: `\\q` drew `q`", st.Unknown)
	}
	if !st.CwdBaseAtRootIsEmpty {
		t.Error(`CwdBaseAtRootIsEmpty = false: measured, \W in / drew nothing`)
	}
	// The doubled escape is **not** a row. `\\B` drew `B` and `\\\\B` drew
	// `\B`, and a row mapping the escape to itself would make the first of
	// those `\B`: the pair collapses in the expansion that runs first, and
	// what reaches the table is a code it has no field for.
	if _, ok := st.Codes['\\']; ok {
		t.Error(`\\ is in the table, and the doubling happens before the table here`)
	}
	// The C escapes, one byte each.
	for code, want := range map[rune]string{
		'e': "\x1b", 'a': "\a", 'v': "\v", 'f': "\f", 'b': "\b", 'r': "\r",
	} {
		if got := st.Sequences[code]; got != want {
			t.Errorf(`\%c drew %q, want %q`, code, got, want)
		}
	}
	// The numeric spellings. Up to three octal digits, and a hexadecimal
	// spelling the sibling does not have at all.
	if st.Octal != interp.OctalUpToThree {
		t.Errorf("Octal = %v, want up to three: `\\1` drew 01 and `\\10` drew 08", st.Octal)
	}
	if !st.Hex || st.HexWithNoDigits != "?" {
		t.Errorf("Hex = %v / %q, want true and `?`: `\\x41` drew `A` and `\\xg` drew `?g`",
			st.Hex, st.HexWithNoDigits)
	}
	if !st.TrailingEscapeIsDropped {
		t.Error("TrailingEscapeIsDropped = false: a value ending in a backslash drew nothing for it")
	}
	if st.Privilege != "$" {
		t.Errorf("Privilege = %q, want `$`: measured at uid 1000, the prompt is `/ $ `", st.Privilege)
	}
}

// The table reaches a runner, which is the half that was missing when it was
// only a value: a runner draws `\u` as `\u` because this table says so, not
// because nobody told it anything (#1455).
func TestApplyInstallsThePromptStyle(t *testing.T) {
	preset.PromptTableInstalled(t, dialecttest.Base{Dir: t.TempDir()}, ash.PromptStyle())
}
