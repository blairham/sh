// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `$EPOCHSECONDS` and `$EPOCHREALTIME`, measured against bash 5.3.15 on
// 2026-09-12 with `env -i PATH=/usr/bin:/bin` and a scratch HOME, over a
// script file.
//
// The clock is pinned throughout, and that is not a convenience: a value read
// off the wall clock is never the same twice, so an assertion about one would
// have to be a *shape* — and a shape is exactly what a broken implementation
// still has. Every number below is a number a real bash printed for the same
// instant.

// epochAt is the moment these tests read the clock at: 2026-09-06
// 12:34:56.123456789 UTC, which is 1788698096 seconds.
//
// Deliberately the same instant dialect/zsh/datetime_test.go pins, because the
// point of these tests is that the two shells answer it *differently* — six
// places here against ten there — and a different instant in each file would
// let a reader put the difference down to the input.
var epochAt = time.Unix(1788698096, 123456789)

// runBashClock runs src against a bash runner whose clock is pinned to at.
//
// A runner built here rather than through dialecttest.Preset, which has no
// way to pin a clock; everything else is the same four vectors.
func runBashClock(t *testing.T, clock func() time.Time, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &diag,
		Dir: t.TempDir(), Name: "bash",
		Dialect: presetDialect(),
		Clock:   clock,
	}
	bash.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), st
}

func runBashAt(t *testing.T, at time.Time, src string) (string, int) {
	t.Helper()
	return runBashClock(t, func() time.Time { return at.UTC() }, src)
}

// The two parameters this shell has had since 5.0, and the fraction is six
// places — a microsecond — where zsh's is ten.
func TestTheEpochParameters(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the seconds", `echo $EPOCHSECONDS`, "1788698096\n"},
		// Six places exactly, and truncated rather than rounded: 123456789ns
		// is 123456µs and not 123457µs.
		{"the real time", `echo $EPOCHREALTIME`, "1788698096.123456\n"},
		{"how many places", `d=${EPOCHREALTIME#*.}; echo "places=${#d}"`, "places=6\n"},
		// A read inside an expression is the same read.
		{"in an expression", `echo $(( EPOCHSECONDS - 1788698000 ))`, "96\n"},
		// The two halves of one instant never disagree, which is what
		// truncating the fraction buys: rounding 999999999ns would carry into
		// a second `$EPOCHSECONDS` does not have.
		{
			"the two halves agree at the top of a second",
			`echo "$EPOCHSECONDS ${EPOCHREALTIME%.*}"`,
			"1788698096 1788698096\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runBashAt(t, epochAt, tc.src); out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The fraction of a nanosecond count one tick short of a whole second, which
// is the input that tells truncation from rounding: rounded to six places it
// would be `1000000`, a seventh digit and a second this instant has not
// reached.
func TestTheFractionTruncatesRatherThanRounding(t *testing.T) {
	at := time.Unix(1788698096, 999999999)
	const want = "1788698096.999999\n"
	if out, st := runBashAt(t, at, `echo $EPOCHREALTIME`); out != want || st != 0 {
		t.Errorf("$EPOCHREALTIME = %q status %d, want %q at 0", out, st, want)
	}
}

// An assignment is taken and thrown away: status 0, no diagnostic, and the
// parameter still the clock afterwards. This is where the two shells part
// company hardest — zsh's are readonly and `EPOCHSECONDS=5` is fatal there.
//
// The assigned value is `5`, which is one digit against the clock's ten, so
// "the assignment was ignored" and "the assignment took" cannot agree by
// accident the way two values of a similar shape could.
func TestAnAssignmentToTheClockIsIgnoredWithoutASentence(t *testing.T) {
	for _, name := range []string{"EPOCHSECONDS", "EPOCHREALTIME"} {
		t.Run(name, func(t *testing.T) {
			src := name + "=5\necho \"st=$? [${" + name + "%%.*}]\""
			want := "st=0 [1788698096]\n"
			if out, st := runBashAt(t, epochAt, src); out != want || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", src, out, st, want)
			}
		})
	}
}

// And `unset` takes them away, silently and with the parameter gone
// afterwards — not the refusal zsh writes.
//
// What a *later* assignment does is a divergence this shell still has, and it
// is not this parameter's: see the note in epoch.go and #2450.
func TestUnsettingTheClockTakesItAway(t *testing.T) {
	for _, name := range []string{"EPOCHSECONDS", "EPOCHREALTIME"} {
		t.Run(name, func(t *testing.T) {
			src := "unset " + name + "\necho \"st=$? [${" + name + "-unset}]\""
			want := "st=0 [unset]\n"
			if out, st := runBashAt(t, epochAt, src); out != want || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", src, out, st, want)
			}
		})
	}
}

// They are produced and not stored, which is the whole of what a clock has to
// get right: a value read once is wrong from the instant afterwards, and
// silently — the caller still gets a number.
func TestTheEpochParametersAreAViewAndNotASnapshot(t *testing.T) {
	at := epochAt
	// Every read is recorded, so the assertion can name the two seconds the
	// two echoes should have printed rather than a pair guessed at: the shell
	// reads the clock for its own reasons too, and a test that assumed
	// otherwise would break for a reason that is not this one.
	var reads []int64
	out, st := runBashClock(t, func() time.Time {
		// Each read moves the clock on by a second, so a snapshot answers the
		// same number twice and a view does not.
		at = at.Add(time.Second)
		reads = append(reads, at.Unix())
		return at.UTC()
	}, `echo $EPOCHSECONDS; echo $EPOCHSECONDS`)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if len(reads) < 2 {
		t.Fatalf("the clock was read %d times, want at least the two echoes", len(reads))
	}
	last := reads[len(reads)-2:]
	want := strconv.FormatInt(last[0], 10) + "\n" + strconv.FormatInt(last[1], 10) + "\n"
	if out != want || last[0] == last[1] {
		t.Errorf("two reads gave %q, want %q — two different seconds", out, want)
	}
}

// bash has neither of zsh's other two clock features, and saying so is worth a
// test: the failure this guards against is the one the issue named — reaching
// for the `zsh/datetime` registration wholesale because the first two names
// match.
func TestTheZshClockExtrasAreNotHere(t *testing.T) {
	const src = `echo "pair=[${epochtime[1]}] n=${#epochtime[@]}"; strftime %Y 0; echo "st=$?"`
	out, _ := runBashAt(t, epochAt, src)
	const want = "pair=[] n=0\n"
	if len(out) < len(want) || out[:len(want)] != want {
		t.Errorf("got %q, want it to start %q — no $epochtime here", out, want)
	}
	if !bytes.Contains([]byte(out), []byte("strftime")) {
		t.Errorf("got %q, want `strftime` reported as not a command here", out)
	}
}
