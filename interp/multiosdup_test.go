// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `>&` duplication is one of the targets a command names, so under the
// dialect that writes to every target it belongs in the fan-out.
//
// It was rebound instead — the duplication path set the stream and the
// fan-out never learned of it — so `echo x >&1 >b` reached the file and not
// the terminal, at status 0 with nothing said (#1261).
func TestADuplicationJoinsTheFanOut(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  Answer
		wantOut string
	}{
		{"every target", Yes, "x\n"},
		{"only the last", No, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			sem := CoreSemantics()
			sem.RedirectsWriteToEveryTarget = tc.answer
			out, st := run(t, "echo x >&1 >b", func(r *Runner) {
				r.Semantics, r.Dir = &sem, dir
			})
			if st != 0 {
				t.Fatalf("status %d, output %q", st, out)
			}
			if out != tc.wantOut {
				t.Errorf("stdout = %q, want %q", out, tc.wantOut)
			}
			if got := readFile(t, dir, "b"); got != "x\n" {
				t.Errorf("b = %q, want the file written either way", got)
			}
		})
	}
}

// What a duplication contributes is the descriptor *as it stands now*, which
// is the discriminating half: written the other way round the file is filled
// twice, because by then standard output already is the fan-out.
//
// A shell that merely dropped the duplication would answer once here, and so
// would one that contributed the stream the command started with.
func TestADuplicationCopiesTheFanOutSoFar(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"every target", Yes, "x\nx\n"},
		{"only the last", No, "x\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			sem := CoreSemantics()
			sem.RedirectsWriteToEveryTarget = tc.answer
			if _, st := run(t, "echo x >b >&1", func(r *Runner) {
				r.Semantics, r.Dir = &sem, dir
			}); st != 0 {
				t.Fatalf("status %d", st)
			}
			if got := readFile(t, dir, "b"); got != tc.want {
				t.Errorf("b = %q, want %q", got, tc.want)
			}
		})
	}
}

// The same rule where the duplication names a descriptor from the table
// rather than a named stream, so what joins the set is a file the command
// itself opened.
func TestATableDescriptorJoinsTheFanOut(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		wantB  string
	}{
		{"every target", Yes, "x\n"},
		{"only the last", No, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			sem := CoreSemantics()
			sem.RedirectsWriteToEveryTarget = tc.answer
			if _, st := run(t, "echo x 3>c >b >&3", func(r *Runner) {
				r.Semantics, r.Dir = &sem, dir
			}); st != 0 {
				t.Fatalf("status %d", st)
			}
			if got := readFile(t, dir, "b"); got != tc.wantB {
				t.Errorf("b = %q, want %q", got, tc.wantB)
			}
			if got := readFile(t, dir, "c"); got != "x\n" {
				t.Errorf("c = %q, want the duplicated descriptor written either way", got)
			}
		})
	}
}

// A close is the boundary of the rule: it does not add a target, it discards
// the ones named before it. Unanimous in the panel, so both answers to the
// axis have to agree here — a fan-out that only ever grew wrote to `b` too.
func TestACloseEmptiesTheFanOut(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		dir := t.TempDir()
		sem := CoreSemantics()
		sem.RedirectsWriteToEveryTarget = answer
		if _, st := run(t, "echo x >b >&- >c", func(r *Runner) {
			r.Semantics, r.Dir = &sem, dir
		}); st != 0 {
			t.Fatalf("status %d", st)
		}
		if got := readFile(t, dir, "b"); got != "" {
			t.Errorf("b = %q, want the targets before the close discarded", got)
		}
		if got := readFile(t, dir, "c"); got != "x\n" {
			t.Errorf("c = %q, want the target after the close written", got)
		}
	}
}

// One duplication of a stream nothing else redirects decides nothing, so the
// axis is not asked — the same rule that keeps a lone `>` out of it.
func TestALoneDuplicationAsksNoAxis(t *testing.T) {
	sem := CoreSemantics()
	out, _ := run(t, "echo x >&1", func(r *Runner) { r.Semantics, r.Dir = &sem, t.TempDir() })
	if strings.Contains(out, "no dialect was chosen") {
		t.Errorf("one duplication: got %q, want no question asked", out)
	}
	if out != "x\n" {
		t.Errorf("stdout = %q, want the line", out)
	}
}

// The axis a duplication now reaches is named for what it covers. Two
// targets and no dialect chosen refuses by name, and the name is `targets`
// rather than `files`: a `>&` duplication is one of them and is not a file.
//
// Asserted because the wording is the whole of what the refusal carries — a
// question nobody can act on is a stub with a diagnostic — and because
// nothing else in the tree reads this string.
func TestTheFanOutAxisIsNamedForTargetsNotFiles(t *testing.T) {
	sem := CoreSemantics()
	out, _ := run(t, "echo x >&1 >b", func(r *Runner) { r.Semantics, r.Dir = &sem, t.TempDir() })
	const want = "a command redirecting one stream to several targets: " +
		"the shells disagree here and no dialect was chosen"
	if !strings.Contains(out, want) {
		t.Errorf("refusal = %q, want it to contain %q", out, want)
	}
}
