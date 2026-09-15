// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// `RANDOM=n` seeds the generator, in the three dialects that have the
// parameter at all.
//
// Measured 2026-09-14, `env -i PATH=/usr/bin:/bin HOME=<scratch>`, a script
// file, `RANDOM=42; echo "$RANDOM $RANDOM"` written twice and the whole script
// run twice:
//
//	bash 5.3.15   17772 26794    ksh93u+   22700 13681
//	zsh 5.9.2     17766 11151
//
// Every pair repeated, within the run and across runs. No two columns draw the
// same numbers, so what is shared is the *reproducibility* and not the
// sequence — which is why nothing here names a number. The assignment used to
// be heard and stored for the producer to find, and the producer kept no state
// to find it with, so a seeded script was four unrelated numbers (#2827).
//
// One table for the three dialects rather than a case in each, because the
// property is the same in all three and the thing that would go missing is a
// fourth dialect's writer, which a per-package test cannot notice.
func randomPresets() []dialecttest.Preset {
	return []dialecttest.Preset{
		{
			Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
			Diagnostics: bash.Diagnostics, Apply: bash.Apply,
		},
		{
			Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
			Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
		},
		{
			Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
			Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
		},
	}
}

func TestASeededRandomRepeatsItself(t *testing.T) {
	for _, p := range randomPresets() {
		t.Run(p.Name, func(t *testing.T) {
			out, st, err := p.Combined(t, dialecttest.Base{},
				`RANDOM=42; echo "$RANDOM $RANDOM"`+"\n"+
					`RANDOM=42; echo "$RANDOM $RANDOM"`+"\n"+
					`RANDOM=43; echo "$RANDOM $RANDOM"`)
			if err != nil || st != 0 {
				t.Fatalf("status %d, err %v: %s", st, err, out)
			}
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			if len(lines) != 3 {
				t.Fatalf("wrote %q, want three lines", out)
			}
			if lines[0] != lines[1] {
				t.Errorf("the same seed drew %q and then %q", lines[0], lines[1])
			}
			// And the seed is what decides, rather than the sequence being
			// fixed: a different one has to draw something else, or "seeded"
			// would be satisfied by a generator that ignores the number.
			if lines[2] == lines[0] {
				t.Errorf("seeds 42 and 43 both drew %q", lines[0])
			}
			for _, line := range lines {
				for _, field := range strings.Fields(line) {
					n, err := strconv.Atoi(field)
					if err != nil || n < 0 || n > 32767 {
						t.Errorf("drew %q, want a number in 0..32767", field)
					}
				}
			}
		})
	}
}

// A value that is not a number seeds with zero rather than being refused.
// Measured the same day: `RANDOM=abc` draws exactly what `RANDOM=0` draws, in
// all three.
func TestARandomSeedThatIsNotANumberIsZero(t *testing.T) {
	for _, p := range randomPresets() {
		t.Run(p.Name, func(t *testing.T) {
			out, st, err := p.Combined(t, dialecttest.Base{},
				`RANDOM=0; echo "$RANDOM $RANDOM"`+"\n"+
					`RANDOM=abc; echo "$RANDOM $RANDOM"`)
			if err != nil || st != 0 {
				t.Fatalf("status %d, err %v: %s", st, err, out)
			}
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			if len(lines) != 2 || lines[0] != lines[1] {
				t.Errorf("wrote %q, want the two lines to agree", out)
			}
		})
	}
}

// And a shell nobody seeded is still a different shell every time, which is
// what the parameter is for.
//
// Two runners rather than two reads in one, because the thing that would be
// wrong is a *fixed* seed: a generator seeded from a constant repeats itself
// between runs while looking perfectly random inside one.
func TestAnUnseededRandomIsNotReproducible(t *testing.T) {
	p := randomPresets()[0]
	const src = `echo "$RANDOM $RANDOM $RANDOM $RANDOM"`
	first, _, err := p.Combined(t, dialecttest.Base{}, src)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := p.Combined(t, dialecttest.Base{}, src)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Errorf("two fresh shells both drew %q", strings.TrimSpace(first))
	}
}

// A subshell draws from where its parent had got to and leaves the parent
// where it was.
//
// Measured 2026-09-14 with `RANDOM=7; a=$RANDOM; (b=$RANDOM; c=$RANDOM); d=$RANDOM`
// against the same script with the subshell removed: bash 5.3.15 and zsh 5.9.2
// give the same `$a $d` either way, so the subshell's two draws cost the
// parent nothing. ksh93u+ is the one column where they do, and that is
// measured and not reproduced here.
func TestASubshellsDrawsDoNotDisturbTheParent(t *testing.T) {
	p := randomPresets()[0]
	out, st, err := p.Combined(t, dialecttest.Base{},
		`RANDOM=7; a=$RANDOM; (b=$RANDOM; c=$RANDOM;); d=$RANDOM; echo "$a $d"`+"\n"+
			`RANDOM=7; e=$RANDOM; f=$RANDOM; echo "$e $f"`)
	if err != nil || st != 0 {
		t.Fatalf("status %d, err %v: %s", st, err, out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 || lines[0] != lines[1] {
		t.Errorf("wrote %q, want the two lines to agree — the subshell took draws from the parent", out)
	}
}
