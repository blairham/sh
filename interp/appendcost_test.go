// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Building a string one append at a time — `s="$s$x"` in a loop, the shape
// every script that collects a list in a variable has — copies the whole of
// the value on every append in every shell, so the work is quadratic
// everywhere and the constant is the whole of the difference. Each append
// should cost one copy of the value and no more.
//
// It cost five: the quoted `$s` was put into the escaped form a byte at a
// time into a builder that grew by doubling, which allocated about four bytes
// per byte of the value before the assignment built the one copy it keeps.
// 20,000 appends took 3.0 s of CPU where dash takes 0.57 (#6357).
//
// The figure asserted is bytes allocated per byte of value appended to, so it
// does not move with the machine: a run that copies the value once is a
// little over 1, and the old path was over 4. The appended piece is long so
// that what the loop itself allocates per iteration is a rounding error.
const (
	appendCount = 600
	appendPiece = 200
)

// appendBytesPerByte runs src with x set to a appendPiece-byte value and
// returns the bytes the run allocated per byte of the value that each of
// appendCount appends had to copy.
func appendBytesPerByte(t *testing.T, src string, enable func(*syntax.Dialect)) float64 {
	t.Helper()
	d := syntax.Core()
	if enable != nil {
		enable(&d)
	}
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := testSemantics()
	diags := CoreDiagnostics()
	var buf output
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diags,
		Env: append(testPATH(), "x="+strings.Repeat("y", appendPiece)), Dialect: &d,
	})
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	runtime.ReadMemStats(&after)
	if want := fmt.Sprint(appendCount * appendPiece); strings.TrimSpace(buf.String()) != want {
		t.Fatalf("%s: printed %q, want %s", src, buf.String(), want)
	}
	// The value before the k-th append is (k-1) pieces long.
	copied := float64(appendPiece) * appendCount * (appendCount - 1) / 2
	return float64(after.TotalAlloc-before.TotalAlloc) / copied
}

func TestAnAppendCopiesTheValueOnce(t *testing.T) {
	loop := func(body string) string {
		return fmt.Sprintf(`s=; i=0; while [ $i -lt %d ]; do %s; i=$((i+1)); done; echo ${#s}`, appendCount, body)
	}
	for _, c := range []struct {
		name, body string
		enable     func(*syntax.Dialect)
	}{
		{"quoted", `s="$s$x"`, nil},
		{"unquoted", `s=$s$x`, nil},
		{"append operator", `s+=$x`, func(d *syntax.Dialect) { d.AppendAssign = true }},
		{"quoted append operator", `s+="$x"`, func(d *syntax.Dialect) { d.AppendAssign = true }},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := appendBytesPerByte(t, loop(c.body), c.enable)
			t.Logf("%s: %.2f bytes allocated per byte of value", c.body, got)
			if got > 2.5 {
				t.Errorf("%s allocated %.2f bytes per byte of the value it appended to, want about 1: "+
					"an append should copy the value once", c.body, got)
			}
		})
	}
}

// BenchmarkAppend is the loop the issue measured, reporting what one append
// allocates. Every append copies the value, so B/append grows with N; what
// should hold is B/append over the mean length staying near 1.
func BenchmarkAppend(b *testing.B) {
	for _, n := range []int{1000, 4000} {
		src := fmt.Sprintf(`s=; i=0; while [ $i -lt %d ]; do s="$s$i,"; i=$((i+1)); done; echo ${#s}`, n)
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			var final int
			for b.Loop() {
				d := syntax.Core()
				sem := testSemantics()
				diags := CoreDiagnostics()
				var buf output
				// testrunner:bare — newTestRunner takes a *testing.T, and a
				// benchmark has a *testing.B; this gives the Runner the same
				// directory, TMPDIR and CleanUp the helper would.
				r := &Runner{
					Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diags,
					Env: append(testPATH(), "TMPDIR="+b.TempDir()), Dialect: &d, Dir: b.TempDir(),
				}
				if _, err := r.Run(context.Background(), f); err != nil {
					b.Fatal(err)
				}
				r.CleanUp()
				fmt.Sscan(buf.String(), &final)
			}
			runtime.ReadMemStats(&after)
			perAppend := float64(after.TotalAlloc-before.TotalAlloc) / float64(b.N) / float64(n)
			b.ReportMetric(perAppend, "B/append")
			b.ReportMetric(perAppend/(float64(final)/2), "B/byte")
		})
	}
}
