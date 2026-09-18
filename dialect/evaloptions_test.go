// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// What `eval` makes of a leading dash-word, in all five dialects — #3216,
// where this shell made the same thing of it in every one of them: the first
// word of the text, whatever it looked like.
//
// Measured 2026-09-16 from a script file under `env -i` with a scratch `HOME`
// and no startup files, both streams captured separately, the status printed
// after each line and a marker after that:
//
//	column               eval -q echo hi                        eval -- echo hi       continued
//	bash 5.3.20          `eval: -q: invalid option` + usage, 2  `hi`, 0               yes
//	bash as `sh`         the same two lines, 2                  `hi`, 0               **no**
//	bash 3.2.57          the same two lines, 2                  `hi`, 0               yes
//	ksh93u+ 2012-08-01   `eval: -q: unknown option` + usage, 2  `hi`, 0               **no**
//	zsh 5.9.2            `command not found: -q`, 127           `hi`, 0               yes
//	dash 0.5.12          `eval: -q: not found`, 127             `--: not found`, 127  yes
//	BusyBox ash 1.37.0   the same, 127                          the same, 127         yes
//
// Three answers over the five presets, which is why `Semantics.EvalOptions` is
// not the boolean `DotReadsOptions` is.
//
// The per-dialect suite tiers carry the bytes for bash, ksh and zsh, each
// graded against its own reference. They cannot carry dash or BusyBox ash —
// dash's tier has no reference row for this and ash's is graded inside a
// container — and `core/` cannot hold any of it, because its claim is that
// every reference wrote the same bytes and here three groups did not. So this
// is where the two "reads nothing" columns are pinned, and where a preset
// mutant is caught: a value flipped in one preset leaves `go test ./interp/`
// green, since the interp tests set the vector themselves and read no preset.
func evalOptionPresets() []struct {
	dialecttest.Preset
	// marker is whether `eval -- echo hi` runs `echo hi`.
	marker bool
	// refuses is whether `eval -q echo hi` is a usage error rather than a
	// command that was not there.
	refuses bool
	// eatsALoneDash is whether a command word that is exactly `-` is thrown
	// away by this dialect, which is a fact about the *command word* and not
	// about `eval` — see Semantics.LoneDashInCommandPositionIsDiscarded. It
	// is a column of this table because the two rows it changes are rows
	// this suite already asks, and reading either of them as `eval`'s doing
	// is exactly the mistake #3236 exists to record.
	eatsALoneDash bool
} {
	return []struct {
		dialecttest.Preset
		marker        bool
		refuses       bool
		eatsALoneDash bool
	}{
		{dialecttest.Preset{
			Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
			Diagnostics: bash.Diagnostics, Apply: bash.Apply,
		}, true, true, false},
		{dialecttest.Preset{
			Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
			Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
		}, true, true, false},
		{dialecttest.Preset{
			Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
			Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
		}, true, false, true},
		{dialecttest.Preset{
			Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
			Diagnostics: dash.Diagnostics, Apply: dash.Apply,
		}, false, false, false},
		{dialecttest.Preset{
			Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
			Diagnostics: ash.Diagnostics, Apply: ash.Apply,
		}, false, false, false},
	}
}

func TestEachDialectReadsEvalsOptionsItsOwnWay(t *testing.T) {
	for _, p := range evalOptionPresets() {
		t.Run(p.Name, func(t *testing.T) {
			// The marker. Read: the text behind it runs. Not read: the
			// marker is the command, and it is not there.
			out, st, err := p.Combined(t, dialecttest.Base{}, "eval -- echo hi\n")
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if p.marker {
				if out != "hi\n" || st != 0 {
					t.Errorf("eval -- echo hi = %q (status %d), want %q at 0", out, st, "hi\n")
				}
			} else {
				if strings.Contains(out, "hi") || st != 127 {
					t.Errorf("eval -- echo hi = %q (status %d), want the marker run as a command at 127", out, st)
				}
			}

			// A letter this builtin has not got. Refused: a usage error at
			// 2, and the text does not run. Not refused: it is the command,
			// at 127, and the text does not run either — the statuses are
			// what tell the two apart, which is why both are asserted.
			out, st, err = p.Combined(t, dialecttest.Base{}, "eval -q echo hi\n")
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if strings.Contains(out, "hi\n") {
				t.Errorf("eval -q echo hi = %q, want the text never run", out)
			}
			if p.refuses {
				if st != 2 || !strings.Contains(out, "option") {
					t.Errorf("eval -q echo hi = %q (status %d), want a usage error at 2", out, st)
				}
			} else if st != 127 {
				t.Errorf("eval -q echo hi = %q (status %d), want a command not found at 127", out, st)
			}

			// The controls, and they answer alike in all five. Without them
			// a preset that refused everything, or read everything as text,
			// would still pass the two rows above in its own column.
			for _, c := range []struct{ name, src, want string }{
				{"a plain word is the text", "eval echo plain\n", "plain\n"},
				{"a quoted command is the text", "eval 'echo quoted'\n", "quoted\n"},
				{"an eval with nothing runs nothing", "eval\n", ""},
			} {
				out, st, err := p.Combined(t, dialecttest.Base{}, c.src)
				if err != nil {
					t.Fatalf("%s: err %v: %s", c.name, err, out)
				}
				if out != c.want || st != 0 {
					t.Errorf("%s: = %q (status %d), want %q at 0", c.name, out, st, c.want)
				}
			}

			// A lone `-` as a word of its own is outside the reading in
			// every column: it is the first word of the text, and the text
			// runs it as a command. Measured on bash 5.3.20, ksh93u+, dash
			// 0.5.12 and BusyBox ash 1.37.0 — 127 in all four. zsh answers
			// 0 there, and **not** because `eval` ate the dash: a bare `-`
			// in command position is discarded by that shell wherever it
			// appears, which is Semantics.LoneDashInCommandPositionIsDiscarded
			// and not this axis (#3236). eatsALoneDash is that column, and
			// it is deliberately not p.refuses: the shell that discards the
			// word is the one that reads no options at all.
			out, st, err = p.Combined(t, dialecttest.Base{}, "eval - echo hi\n")
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if p.eatsALoneDash {
				if out != "hi\n" || st != 0 {
					t.Errorf("eval - echo hi = %q (status %d), want the dash discarded and %q at 0", out, st, "hi\n")
				}
			} else if strings.Contains(out, "hi\n") || st != 127 {
				t.Errorf("eval - echo hi = %q (status %d), want the dash run as a command at 127", out, st)
			}

			// **A dash-word with the rest of the text inside it is a
			// different question**, and it is the one that separates the
			// three readings hardest. `eval '- echo hi'` is a single
			// argument, so where options are read the whole word is an
			// option bundle — measured, bash 5.3.20 answers `eval: - :
			// invalid option` with its usage line at 2, and ksh93u+ answers
			// a cascade of `unknown option` lines at 2. Where they are not,
			// the word is text. A probe that only ever writes the dash as a
			// word of its own cannot tell those apart, and reading this row
			// as "the dash is always the text" is what the first draft of
			// this test got wrong.
			out, st, err = p.Combined(t, dialecttest.Base{}, "eval '- echo hi'\n")
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			// And this is the row that proves the reading. In the column
			// that discards the word, one argument behaves exactly as two
			// did — `hi` at 0 — which no option reading can produce: an
			// option bundle here is the whole word `- echo hi`. So the dash
			// is being dropped from the *text*, and `eval` never saw an
			// option at all.
			if p.eatsALoneDash {
				if out != "hi\n" || st != 0 {
					t.Errorf("eval '- echo hi' = %q (status %d), want the dash discarded and %q at 0", out, st, "hi\n")
				}
				return
			}
			if strings.Contains(out, "hi\n") {
				t.Errorf("eval '- echo hi' = %q, want the text never run", out)
			}
			if p.refuses {
				if st != 2 || !strings.Contains(out, "option") {
					t.Errorf("eval '- echo hi' = %q (status %d), want a usage error at 2", out, st)
				}
			} else if st != 127 {
				t.Errorf("eval '- echo hi' = %q (status %d), want the word run as a command at 127", out, st)
			}
		})
	}
}
