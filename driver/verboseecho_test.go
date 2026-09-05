// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/syntax"
)

// verboseScript is a program of n lines whose commands cost nothing to run,
// so that what is being measured is the echoing rather than the running.
func verboseScript(n int) string {
	var sb strings.Builder
	for i := range n {
		fmt.Fprintf(&sb, "x%d=%d\n", i, i)
	}
	return sb.String()
}

// TestVerboseEchoesThePhysicalLinesAsTheyAreRead pins what `set -v` is, since
// #580 changed how it finds the lines and not which ones it writes.
//
// It echoes each *physical* line as the shell reads it, which has three
// consequences that a change to the mechanism could quietly lose: a compound
// command's lines are all echoed before any of it runs, a line that runs
// nothing is echoed anyway, and a here-document body is echoed even though it
// is never parsed as input.
func TestVerboseEchoesThePhysicalLinesAsTheyAreRead(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want string
		why  string
	}{
		{
			name: "a line at a time",
			src:  "echo one\necho two\n",
			want: "echo one\none\necho two\ntwo\n",
			why:  "each line is echoed as it is read, so its own output follows it",
		},
		{
			name: "a compound command is read whole",
			src:  "for i in 1 2\ndo\necho $i\ndone\n",
			want: "for i in 1 2\ndo\necho $i\ndone\n1\n2\n",
			why:  "the loop cannot run until it is finished, so all four lines are echoed before either iteration",
		},
		{
			name: "lines that run nothing",
			src:  "# a comment\n\necho one\n",
			want: "# a comment\n\necho one\none\n",
			why:  "the echo is of the input, not of the commands: a comment and a blank line are read and so are written",
		},
		{
			name: "a continued line",
			src:  "echo one \\\ntwo\necho three\n",
			want: "echo one \\\ntwo\none two\necho three\nthree\n",
			why:  "both physical lines belong to one logical line, and both are echoed before it runs",
		},
		{
			name: "no trailing newline",
			src:  "echo one",
			want: "echo one\none\n",
			why:  "the last line is a line whether or not the text ends in a newline",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			// One stream for both, so that the order of the echo against the
			// output it interleaves with is part of what is asserted. Two
			// buffers compared separately would pass a shell that echoed a
			// whole loop *after* running it.
			var out strings.Builder
			sh := shell()
			sh.Stdout, sh.Stderr = &out, &out
			if code := driver.MainArgs(sh, []string{"testsh", "-v", writeScript(t, c.src)}); code != 0 {
				t.Fatalf("status %d", code)
			}
			if got := out.String(); got != c.want {
				t.Errorf("got %q, want %q — %s", got, c.want, c.why)
			}
		})
	}
}

// TestVerboseEchoesAHereDocumentBody: a here-document's body is never parsed
// as input, and is echoed anyway — the echo is of the text the shell read, not
// of the commands it found in it.
//
// Asserted on the echo alone rather than against the command's own output,
// because where the terminator falls in that interleaving is a separate
// question from this one and is not yet the panel's answer: the four real
// shells echo the terminator before running the command and this shell echoes
// it after. That is #582 and predates #580 — it is recorded here so that the
// case is not read as evidence the ordering was checked.
func TestVerboseEchoesAHereDocumentBody(t *testing.T) {
	var out, errs strings.Builder
	sh := shell()
	sh.Stdout, sh.Stderr = &out, &errs
	src := "cat <<END\nbody\nEND\necho after\n"
	if code := driver.MainArgs(sh, []string{"testsh", "-v", writeScript(t, src)}); code != 0 {
		t.Fatalf("status %d", code)
	}
	if got := errs.String(); got != src {
		t.Errorf("echo = %q, want the whole of %q — every physical line, the body included", got, src)
	}
	if want := "body\nafter\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

// TestVerboseStartsAtTheLineAfterTheOneThatTurnedItOn: lines read while the
// option was off are spent rather than saved. It is the case the byte offset
// has to get right without ever having written anything — the walk past the
// quiet lines is what leaves it pointing at the loud one.
func TestVerboseStartsAtTheLineAfterTheOneThatTurnedItOn(t *testing.T) {
	var out strings.Builder
	sh := shell()
	sh.Stdout, sh.Stderr = &out, &out
	src := "echo quiet\nset -v\necho loud\n# a comment\necho last\n"
	if code := driver.MainArgs(sh, []string{"testsh", writeScript(t, src)}); code != 0 {
		t.Fatalf("status %d", code)
	}
	want := "quiet\necho loud\nloud\n# a comment\necho last\nlast\n"
	if got := out.String(); got != want {
		t.Errorf("got %q, want %q — the first two lines are read with the option off and are never echoed", got, want)
	}
}

// TestVerboseEchoResumesOnTheRightByte checks the half of the position that a
// script cannot show: where the walk stops.
//
// The line number alone cannot say, because the text a program on standard
// input has read so far grows between calls. A piece that has not ended in a
// newline is a line for as long as it is the last of the text, and the offset
// has to record that it was taken rather than resting on its first byte.
func TestVerboseEchoResumesOnTheRightByte(t *testing.T) {
	sh := shell()
	sh.Stderr = io.Discard
	for _, c := range []struct {
		name                 string
		src                  string
		upTo, line, off      int
		wantLine, wantOffset int
	}{
		{
			name: "from the start", src: "a\nb\nc\n", upTo: 2,
			wantLine: 2, wantOffset: 4,
		},
		{
			name: "resuming", src: "a\nb\nc\n", upTo: 3, line: 2, off: 4,
			wantLine: 3, wantOffset: 6,
		},
		{
			name: "already past it", src: "a\nb\nc\n", upTo: 1, line: 2, off: 4,
			wantLine: 2, wantOffset: 4,
		},
		{
			name: "the piece after the last newline", src: "a\nb", upTo: 2,
			// One past the end: the piece was taken, and a byte offset resting
			// on len cannot be told from one that has not read it yet.
			wantLine: 2, wantOffset: 4,
		},
		{
			name: "the empty piece after a trailing newline", src: "a\n", upTo: 2,
			wantLine: 2, wantOffset: 3,
		},
		{
			name: "asking past the end of the text", src: "a\n", upTo: 5,
			// Counted rather than echoed again when more of the input arrives.
			wantLine: 5, wantOffset: 3,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			line, off := sh.SayVerboseForTest(c.src, c.upTo, c.line, c.off, true)
			if line != c.wantLine || off != c.wantOffset {
				t.Errorf("got line %d offset %d, want line %d offset %d", line, off, c.wantLine, c.wantOffset)
			}
		})
	}
}

// TestVerboseEchoIsLinearInTheLengthOfTheScript is #580 as a test rather than
// as a timing: the echo used to recover the lines by splitting the whole
// program on newlines once per line, so an n-line script allocated a slice of
// every line in it n times and the cost was n².
//
// Measured as bytes allocated rather than as elapsed time, because that is the
// quantity the mechanism decides and it does not depend on what else the
// machine is doing. Quadrupling the script quadruples a linear echo and
// multiplies a quadratic one by sixteen; the threshold sits between the two
// with room for the fixed cost of running at all.
func TestVerboseEchoIsLinearInTheLengthOfTheScript(t *testing.T) {
	const short, long = 1000, 4000
	cost := func(lines int) uint64 {
		src := writeScript(t, verboseScript(lines))
		sh := shell()
		sh.Stdout, sh.Stderr = io.Discard, io.Discard
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		if code := driver.MainArgs(sh, []string{"testsh", "-v", src}); code != 0 {
			t.Fatalf("status %d", code)
		}
		runtime.ReadMemStats(&after)
		return after.TotalAlloc - before.TotalAlloc
	}
	a, b := cost(short), cost(long)
	const scale = long / short
	// Well under scale², and comfortably above scale so that the assertion is
	// about the shape of the growth rather than about a particular allocator.
	const most = scale * 2
	t.Logf("%d lines: %d bytes; %d lines: %d bytes; factor %.1f", short, a, long, b, float64(b)/float64(a))
	if got := float64(b) / float64(a); got > most {
		t.Errorf("%d lines cost %d bytes and %d lines cost %d bytes, a factor of %.1f;"+
			" want under %d, since %d times the lines is %d times a linear echo and %d times a quadratic one",
			short, a, long, b, got, most, scale, scale, scale*scale)
	}
}

// BenchmarkVerboseEcho is the reason #580 exists, as a number. The same script
// with the option on and with it off, at two lengths, so that both the cost of
// echoing and the way it grows are on one page.
func BenchmarkVerboseEcho(b *testing.B) {
	for _, lines := range []int{2000, 8000} {
		path := filepath.Join(b.TempDir(), "program.sh")
		if err := os.WriteFile(path, []byte(verboseScript(lines)), 0o600); err != nil {
			b.Fatal(err)
		}
		for _, c := range []struct {
			name string
			argv []string
		}{
			{"echoing", []string{"testsh", "-v", path}},
			{"silent", []string{"testsh", path}},
		} {
			b.Run(fmt.Sprintf("%d lines/%s", lines, c.name), func(b *testing.B) {
				for b.Loop() {
					sh := driver.Shell{Name: "testsh", Dialect: syntax.Core(), Stdout: io.Discard, Stderr: io.Discard}
					if code := driver.MainArgs(sh, c.argv); code != 0 {
						b.Fatalf("status %d", code)
					}
				}
			})
		}
	}
}
