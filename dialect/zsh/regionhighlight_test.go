// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// `region_highlight`: what a widget writes to colour part of the line.
//
// Measured against zsh 5.9.2 under a pseudo-terminal 2026-09-12; every row
// below is in docs/spec/editing.md under "Coloring the line as it is typed",
// with the escape sequences captured off a real terminal rather than derived
// from the manual — which is how the 256-colour rows came to be right, since
// the manual's `fg_start_code` description would make `fg=200` into
// `ESC[3200m` and it is `ESC[38;5;200m`.

// esc makes an escape sequence readable in a failure message.
func esc(s string) string { return strings.ReplaceAll(s, "\x1b", "ESC") }

// TestAWidgetColorsPartOfTheLine is the feature end to end: a widget writes
// the parameter, and what the editor draws carries the run.
//
// The two-run case rather than one, because one run cannot catch a shell that
// returns the *last* element for every run or that loses the ordering.
func TestAWidgetColorsPartOfTheLine(t *testing.T) {
	r, out := zleRunner(t, `
		paint() { region_highlight=("0 4 fg=red" "4 8 fg=green,bold"); }
		zle -N paint
	`)
	in := repl.Line{Buffer: "AAAABBBB", Cursor: 8}
	if _, ok, _ := runWidget(t, r, out, "paint", in); !ok {
		t.Fatal("the widget did not run")
	}
	runs := zsh.RegionHighlights(r, "AAAABBBB")
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2: %+v", len(runs), runs)
	}
	for i, want := range []repl.Highlight{
		{Start: 0, End: 4, Style: "\x1b[31m"},
		{Start: 4, End: 8, Style: "\x1b[1m\x1b[32m"},
	} {
		if runs[i] != want {
			t.Errorf("run %d: got {%d %d %s}, want {%d %d %s}",
				i, runs[i].Start, runs[i].End, esc(runs[i].Style),
				want.Start, want.End, esc(want.Style))
		}
	}
}

// TestTheOffsetsAreCharactersAndTheRunsAreBytes is the conversion, and it is
// the one place a highlighter that works in the wrong units still looks right
// on an ASCII line.
//
// `héllo` is five characters and six bytes, so a run of the first three
// characters is a run of the first four *bytes*. A shell that passed the
// offsets straight through would colour three bytes — cutting the `é` in half
// — and every ASCII test would still pass.
func TestTheOffsetsAreCharactersAndTheRunsAreBytes(t *testing.T) {
	const line = "héllo wörld"
	r, out := zleRunner(t, `
		paint() { region_highlight=("0 3 fg=red"); }
		zle -N paint
	`)
	if _, ok, _ := runWidget(t, r, out, "paint", repl.Line{Buffer: line}); !ok {
		t.Fatal("the widget did not run")
	}
	runs := zsh.RegionHighlights(r, line)
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	// h(1) é(2) l(1) = 4 bytes for 3 characters.
	if runs[0].Start != 0 || runs[0].End != 4 {
		t.Errorf("got bytes %d..%d, want 0..4", runs[0].Start, runs[0].End)
	}
}

// TestAnElementThisShellCannotDrawIsDropped is the refusal, and each row is a
// different reason.
//
// Dropped and never approximated: a run placed somewhere plausible puts
// colour under text the highlighter did not name, and the editor has no way
// to say that it did.
func TestAnElementThisShellCannotDrawIsDropped(t *testing.T) {
	for _, c := range []struct{ name, elem string }{
		{"a PREDISPLAY this shell does not have", `P0 3 fg=red`},
		{"the same flag glued to the offset", `P0 3 fg=red`},
		{"an end before the start", `4 2 fg=red`},
		{"an end past the line", `0 99 fg=red`},
		{"a start before the line", `-1 3 fg=red`},
		{"offsets that are not numbers", `x y fg=red`},
		{"too few fields", `0 3`},
		{"a spec that paints nothing", `0 3 none`},
		{"the terminal's own colour", `0 3 fg=default`},
		{"a colour that is not one", `0 3 fg=chartreuse`},
		{"a palette entry out of range", `0 3 fg=999`},
	} {
		t.Run(c.name, func(t *testing.T) {
			r, out := zleRunner(t, `
				paint() { region_highlight=("`+c.elem+`"); }
				zle -N paint
			`)
			if _, ok, _ := runWidget(t, r, out, "paint", repl.Line{Buffer: "abcdef"}); !ok {
				t.Fatal("the widget did not run")
			}
			if runs := zsh.RegionHighlights(r, "abcdef"); len(runs) != 0 {
				t.Errorf("got %+v, want nothing drawn", runs)
			}
		})
	}
}

// TestWhatASpecPaints is the translation table, measured off a real terminal.
//
// The order rows are the ones a shell that emitted the spec's own order would
// fail: zsh sends attributes, then foreground, then background, whatever
// order they were written in.
func TestWhatASpecPaints(t *testing.T) {
	for _, c := range []struct{ spec, want string }{
		{"fg=red", "\x1b[31m"},
		{"fg=green", "\x1b[32m"},
		{"fg=b", "\x1b[30m"},
		{"fg=bl", "\x1b[30m"},
		{"fg=3", "\x1b[33m"},
		{"fg=200", "\x1b[38;5;200m"},
		{"fg=#ff8800", "\x1b[38;2;255;136;0m"},
		{"fg=#f80", "\x1b[38;2;255;136;0m"},
		{"bg=red", "\x1b[41m"},
		{"bg=200", "\x1b[48;5;200m"},
		{"bold", "\x1b[1m"},
		{"standout", "\x1b[7m"},
		{"underline", "\x1b[4m"},
		{"fg=red,bold", "\x1b[1m\x1b[31m"},
		{"bold,fg=red", "\x1b[1m\x1b[31m"},
		{"fg=cyan,bg=magenta,underline", "\x1b[4m\x1b[36m\x1b[45m"},
		{"fg=red,memo=someplugin", "\x1b[31m"},
	} {
		t.Run(c.spec, func(t *testing.T) {
			r, out := zleRunner(t, `
				paint() { region_highlight=("0 3 `+c.spec+`"); }
				zle -N paint
			`)
			if _, ok, _ := runWidget(t, r, out, "paint", repl.Line{Buffer: "abcdef"}); !ok {
				t.Fatal("the widget did not run")
			}
			runs := zsh.RegionHighlights(r, "abcdef")
			if len(runs) != 1 {
				t.Fatalf("got %d runs, want 1", len(runs))
			}
			if runs[0].Style != c.want {
				t.Errorf("got %s, want %s", esc(runs[0].Style), esc(c.want))
			}
		})
	}
}

// TestItLivesAsLongAsTheLineDoes is the lifetime, and it is two facts that
// have to hold together.
//
// It **persists** from one widget to the next, which is what lets a
// highlighter add to what an earlier keystroke left; and it is **empty again**
// at the next line, which is the manual's "disappears as soon as the line is
// accepted". A shell that cleared it per widget passes the second and fails
// the first; one that never cleared it passes the first and fails the second.
func TestItLivesAsLongAsTheLineDoes(t *testing.T) {
	r, out := zleRunner(t, `
		add() { region_highlight+=("0 3 fg=red"); }
		zle -N add
	`)
	for i := 1; i <= 2; i++ {
		if _, ok, _ := runWidget(t, r, out, "add", repl.Line{Buffer: "abcdef"}); !ok {
			t.Fatalf("widget %d did not run", i)
		}
	}
	if runs := zsh.RegionHighlights(r, "abcdef"); len(runs) != 2 {
		t.Fatalf("across two widgets on one line: got %d runs, want 2", len(runs))
	}
	zsh.ResetRegionHighlight(r)
	if runs := zsh.RegionHighlights(r, "abcdef"); len(runs) != 0 {
		t.Errorf("after the line ended: got %+v, want nothing", runs)
	}
}

// TestItIsNotAParameterOutsideAWidget is the other half of
// `array-local-special`: a script that is not running a widget must not find
// it.
func TestItIsNotAParameterOutsideAWidget(t *testing.T) {
	if got := zleParam(t, `print "[${+region_highlight}]"`); !strings.Contains(got, "[0]") {
		t.Errorf("outside a widget: got %q, want it unset", got)
	}
}

// TestAWidgetSeesItAsASpecialArray is the type, which is what a plugin tests
// before it trusts the parameter at all.
//
// **The want here is this shell's spelling and not zsh's, deliberately.** zsh
// says `array-local-special` and every one of the line parameters is missing
// the same word here — `${(t)BUFFER}` is `scalar-special` where zsh says
// `scalar-local-special` — so this is not `region_highlight`'s gap and is not
// fixed under it. #2493 is the one word, across all of them. Pinning zsh's
// answer here would have made this file fail for a reason that has nothing to
// do with what it tests.
//
// What this *does* pin is the half that is right and that the parameter is
// useless without: it is set, and it is an array.
func TestAWidgetSeesItAsASpecialArray(t *testing.T) {
	r, out := zleRunner(t, `
		kind() { print -r -- "[${(t)region_highlight}] [${+region_highlight}]"; }
		zle -N kind
	`)
	_, ok, said := runWidget(t, r, out, "kind", repl.Line{Buffer: "abc"})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if !strings.Contains(said, "[array-special] [1]") {
		t.Errorf("got %q, want a special array that is set", said)
	}
}
