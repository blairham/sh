// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The `zsh/datetime` module, measured against zsh 5.9.2 on 2026-09-06 with a
// scratch HOME and no startup files.
//
// Every assertion here is a line that shell wrote. The clock is pinned so
// that it can be: a value produced from the wall clock is not the same twice,
// and a test that only checked the *shape* of one would pass on a shell whose
// answer never moved.

// datetimeEpoch is the moment these tests read the clock at:
// 2026-09-06 12:34:56.123456789 UTC, which is 1788698096 seconds.
var datetimeEpoch = time.Unix(1788698096, 123456789)

// runZshAt runs src with the clock pinned to at, in UTC, and returns
// everything the shell wrote.
func runZshAt(t *testing.T, at time.Time, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{
		Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &diag,
		Dir: t.TempDir(), Name: "zsh",
		Dialect: presetDialect(),
		Clock:   func() time.Time { return at.UTC() },
	}
	zsh.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), st
}

// The module loads because all four of its features are here, which is the
// only module in the table that can say so.
func TestTheDatetimeModuleLoads(t *testing.T) {
	out, st := runZshAt(t, datetimeEpoch,
		`zmodload zsh/datetime; echo "st=$?"; zmodload -e zsh/datetime; echo "e=$?"; zmodload`)
	want := "st=0\ne=0\nzsh/datetime\nzsh/main\n"
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
}

// The three parameters are one clock read each, through Runner.Now — so an
// embedder that pins the clock pins these.
func TestTheClockParameters(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the seconds", `echo $EPOCHSECONDS`, "1788698096\n"},
		// Ten places, which is `typeset -F`'s default precision and what zsh
		// prints. The last of them are the rounding a float64 of a nanosecond
		// epoch gives, exactly as they are there.
		{"the real time", `echo $EPOCHREALTIME`, "1788698096.1234567165\n"},
		{"the pair", `echo "${epochtime[1]} ${epochtime[2]} n=${#epochtime[@]}"`, "1788698096 123456789 n=2\n"},
		// A read inside an expression is the same read.
		{"in an expression", `echo $(( EPOCHSECONDS - 1788698000 ))`, "96\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZshAt(t, datetimeEpoch, tc.src); out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// They are produced and not stored, which is the whole of what a clock has to
// get right: a value read once is wrong from the instant afterwards, and
// silently — the caller still gets a number.
func TestTheClockParametersAreAViewAndNotASnapshot(t *testing.T) {
	at := datetimeEpoch
	f, err := syntax.Parse(`echo $EPOCHSECONDS; echo $EPOCHSECONDS`, zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	// Every read is recorded, so the assertion can name the two seconds the
	// two echoes should have printed rather than a pair guessed at: the
	// shell reads the clock for its own reasons too, and a test that assumed
	// otherwise would break for a reason that is not this one.
	var reads []int64
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{
		Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &diag,
		Dir: t.TempDir(), Name: "zsh", Dialect: presetDialect(),
		// Each read moves the clock on by a second, so a snapshot answers
		// the same number twice and a view does not.
		Clock: func() time.Time {
			at = at.Add(time.Second)
			reads = append(reads, at.Unix())
			return at.UTC()
		},
	}
	zsh.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if len(reads) < 2 {
		t.Fatalf("the clock was read %d times, want at least the two echoes", len(reads))
	}
	last := reads[len(reads)-2:]
	want := itoa(last[0]) + "\n" + itoa(last[1]) + "\n"
	if got := out.String(); got != want || last[0] == last[1] {
		t.Errorf("two reads gave %q, want %q — two different seconds", got, want)
	}
}

// Readonly, which is not decoration: a produced parameter a script can assign
// to is shadowed by the assignment from then on, so it would stop tracking
// the clock and never say so. Measured — zsh refuses all three, and the
// refusal ends the script there.
func TestTheClockParametersAreReadonly(t *testing.T) {
	for _, name := range []string{"EPOCHSECONDS", "EPOCHREALTIME", "epochtime"} {
		t.Run(name, func(t *testing.T) {
			out, st := runZshAt(t, datetimeEpoch, name+"=5\necho after")
			if !strings.Contains(out, "read-only variable: "+name) || strings.Contains(out, "after") {
				t.Errorf("assigning %s = %q at %d, want a refusal and no `after`", name, out, st)
			}
			out, _ = runZshAt(t, datetimeEpoch, "unset "+name+"\necho after")
			if !strings.Contains(out, "read-only variable: "+name) {
				t.Errorf("unsetting %s = %q, want a refusal", name, out)
			}
		})
	}
}

// `strftime` writes a time through a format.
//
// The conversions that name a wall-clock field are asserted against Go's own
// rendering of the same instant in the same zone, rather than against a
// literal: this binary reads the zone from its environment, as the shell it
// models does, so a literal here would be a claim about the machine the test
// runs on. The instant, the epoch operand and everything zone-independent are
// still literal.
func TestStrftimeWrites(t *testing.T) {
	local := time.Unix(1788698096, 0)
	for _, tc := range []struct{ name, src, want string }{
		{"an epoch operand", `strftime "%Y-%m-%d %H:%M:%S" 1788698096`, local.Format("2006-01-02 15:04:05") + "\n"},
		// No operand is the clock, which is why the clock is pinned here.
		{"no operand is now", `strftime "%Y-%m-%d"`, local.Format("2006-01-02") + "\n"},
		{"-n drops the newline", `strftime -n "%Y" 1788698096; echo "|end"`, local.Format("2006") + "|end\n"},
		{"-s assigns instead", `strftime -s v "%Y" 1788698096; echo "v=$v st=$?"`, "v=" + local.Format("2006") + " st=0\n"},
		// The five conversions this builtin has beyond `printf '%(fmt)T'`.
		// The first two are the fraction of a second and say nothing about a
		// zone, so they are literal.
		{"nanoseconds", `strftime "%N" 1788698096 123456789`, "123456789\n"},
		// Nine digits always, which is what a shorter fraction shows:
		// measured, five nanoseconds is `000000005` and not `5`.
		{"nanoseconds are padded", `strftime "%N" 1788698096 5`, "000000005\n"},
		// An epoch given without nanoseconds is that second exactly, where
		// no epoch at all keeps the clock's own fraction — measured, and the
		// difference between an operand and a default.
		{"an epoch alone has no fraction", `strftime "%N|%." 1788698096`, "000000000|000\n"},
		{"the clock keeps its fraction", `strftime "%N"`, "123456789\n"},
		{"the fraction", `strftime "%." 1788698096 123456789`, "123\n"},
		{"a fraction with a width", `strftime "%1.|%6." 1788698096 123456789`, "1|123457\n"},
		// `%f`, `%K` and `%L` are the unpadded spellings of `%d`, `%H` and
		// `%I`, which is the whole of what this row asks: the same three
		// fields, without the zero.
		{
			"unpadded fields",
			`strftime "%f|%K|%L|%d|%H|%I" 1788698096`,
			unpadded(local.Format("02")) + "|" + unpadded(local.Format("15")) + "|" +
				unpadded(local.Format("03")) + "|" + local.Format("02|15|03") + "\n",
		},
		// And the compound ones, which come from interp.Strftime rather than
		// from a second copy of the format language.
		{
			"compound conversions",
			`strftime "%F|%T|%D|%R|%r" 1788698096`,
			local.Format("2006-01-02|15:04:05|01/02/06|15:04|03:04:05 PM") + "\n",
		},
		// A conversion nothing knows keeps its letter and loses the `%`,
		// which is the C library's answer and measured here too.
		{"an unknown conversion", `strftime "%Q|%%" 1788698096`, "Q|%\n"},
		// `--` ends the letters, and the letters cluster: `-ns v` is `-n`
		// and `-s v`, which is how a real script writes them.
		{"the letters end at --", `strftime -- "%Y" 0`, time.Unix(0, 0).Format("2006") + "\n"},
		{"clustered letters", `strftime -ns v "%Y" 0; echo "v=$v"`, "v=" + time.Unix(0, 0).Format("2006") + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZshAt(t, datetimeEpoch, tc.src); out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// unpadded drops a leading zero, which is the only difference between `%d`
// and `%f`, `%H` and `%K`, `%I` and `%L`.
func unpadded(s string) string { return strings.TrimPrefix(s, "0") }

// `strftime -r` reads one back, which is the direction a plugin manager uses
// on an HTTP `Last-Modified` header.
//
// Asserted as a round trip against the same zone, for the reason the writing
// cases are: `2026-09-06` is a different second in every zone, and the
// question here is whether the reading agrees with the writing.
func TestStrftimeReads(t *testing.T) {
	local := time.Unix(1788698096, 0)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local)
	for _, tc := range []struct{ name, src, want string }{
		{
			"a date",
			`strftime -r "%Y-%m-%d" ` + local.Format("2006-01-02"),
			itoa(midnight.Unix()) + "\n",
		},
		{
			"the header a real script reads",
			`strftime -r "%d %b %Y %H:%M:%S GMT" "` + local.Format("02 Jan 2006 15:04:05") + ` GMT"`,
			"1788698096\n",
		},
		{
			"-r with -s in one word",
			`strftime -rs v "%Y-%m-%d" ` + local.Format("2006-01-02") + `; echo "v=$v"`,
			"v=" + itoa(midnight.Unix()) + "\n",
		},
		// An unnamed field is the start of 1900 rather than today — measured,
		// and the answer to keep, because the alternative invents a year.
		{
			"an unnamed field is 1900",
			`strftime -r "%H:%M" "1:2"`,
			itoa(time.Date(1900, 1, 1, 1, 2, 0, 0, time.Local).Unix()) + "\n",
		},
		// Fewer digits than the width still match.
		{
			"a narrow field",
			`strftime -r "%Y-%m-%d" "` + local.Format("2006") + `-` + unpadded(local.Format("01")) +
				`-` + unpadded(local.Format("02")) + `"`,
			itoa(midnight.Unix()) + "\n",
		},
		{
			"a twelve-hour clock",
			`strftime -r "%Y-%m-%d %I:%M %p" "` + local.Format("2006-01-02") + ` 01:00 PM"`,
			itoa(midnight.Add(13*time.Hour).Unix()) + "\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZshAt(t, datetimeEpoch, tc.src); out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// Every complaint carries the builtin's own name in the location, and none of
// them is fatal: status 1 and the script carries on.
func TestStrftimeComplaints(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"no operands", `strftime`, "not enough arguments"},
		{"too many", `strftime "%Y" 1 2 3`, "too many arguments"},
		{"a bad letter", `strftime -Q "%Y" 1`, "bad option: -Q"},
		{"a name that is not one", `strftime -s 1bad "%Y" 1`, "not an identifier: 1bad"},
		{"-s with nothing after it", `strftime -s`, "argument expected: -s"},
		// The name is the next word whatever it looks like, so a letter
		// there is a bad *name* rather than a second option.
		{"-s takes the next word as the name", `strftime -s -n "%Y" 1`, "not an identifier: -n"},
		{"an epoch that is not a number", `strftime "%Y" abc`, "abc: invalid argument"},
		{"input that does not match", `strftime -r "%Y-%m-%d" zzz`, "format not matched"},
		// Not "format not matched": the input may be perfectly good and the
		// shortfall is this shell's, so it is named as this shell's.
		{"a conversion -r has not got", `strftime -r "%V" 3`, "-r: %V is not implemented yet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshAt(t, datetimeEpoch, tc.src+"\necho carried-on")
			if !strings.Contains(out, tc.want) {
				t.Errorf("%s = %q, want %q in it", tc.src, out, tc.want)
			}
			if !strings.Contains(out, "carried-on") || st != 0 {
				t.Errorf("%s = %q at %d, want the script to carry on", tc.src, out, st)
			}
		})
	}
}

// Input left over is a warning and still an answer, which is what zsh does —
// a refusal here would lose a header that reads perfectly well up to a
// trailing field nobody asked about.
func TestStrftimeWarnsAboutTrailingInput(t *testing.T) {
	local := time.Unix(1788698096, 0)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local)
	out, st := runZshAt(t, datetimeEpoch,
		`strftime -r "%Y-%m-%d" "`+local.Format("2006-01-02")+` trailing"; echo "st=$?"`)
	want := "warning: input string not completely matched\n" + itoa(midnight.Unix()) + "\nst=0\n"
	if !strings.HasSuffix(out, want) || st != 0 {
		t.Errorf("got %q status %d, want it to end %q at 0", out, st, want)
	}
}
