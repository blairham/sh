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
			sem.RedirectsUseEveryTarget = tc.answer
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
			sem.RedirectsUseEveryTarget = tc.answer
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
			sem.RedirectsUseEveryTarget = tc.answer
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
		sem.RedirectsUseEveryTarget = answer
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

// The same rule for a number the script parked, which is where it was
// missing: the fan-out was written where the switch over the *named* streams
// happened to land, so `exec 3>a 3>b` kept only `b` while `echo x >a >b`
// wrote both — one script saying different things about 1 and about 3 (#734).
//
// Measured 2026-09-12: `exec 3>a 3>b; echo hi >&3` fills both files in zsh
// 5.9.2 and only `b` in dash, bash 5.3, bash 3.2 and ksh93.
func TestANumberedDescriptorJoinsItsOwnFanOut(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		wantA  string
	}{
		{"every target", Yes, "hi\n"},
		{"only the last", No, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			sem := CoreSemantics()
			sem.RedirectsUseEveryTarget = tc.answer
			// Through `exec` so the descriptor is one the script parked
			// rather than one the command carried, which is the shape a
			// script uses a number for at all.
			sem.RedirectErrorOnSpecialBuiltinFatal = No
			out, st := run(t, "exec 3>a 3>b; echo hi >&3; exec 3>&-", func(r *Runner) {
				r.Semantics, r.Dir = &sem, dir
			})
			if st != 0 {
				t.Fatalf("status %d, output %q", st, out)
			}
			if got := readFile(t, dir, "a"); got != tc.wantA {
				t.Errorf("a = %q, want %q", got, tc.wantA)
			}
			if got := readFile(t, dir, "b"); got != "hi\n" {
				t.Errorf("b = %q, want the last target written either way", got)
			}
		})
	}
}

// And the reading half of the same number, which is the direction eachSource
// answers: two sources arrive one after the other rather than the last one
// replacing the first.
func TestANumberedDescriptorReadsFromEverySource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"every source", Yes, "[A\nB]"},
		{"only the last", No, "[B]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			sem := CoreSemantics()
			sem.RedirectsUseEveryTarget = tc.answer
			sem.RedirectErrorOnSpecialBuiltinFatal = No
			// `cat` is an external command, so the descriptor `exec` parked
			// has to be allowed to reach it — a different axis, and not the
			// one this row is about. The substitution strips the last
			// newline, as it does for every command substitution.
			sem.ExecOpenedFdReachesACommand = Yes
			out, st := run(t,
				`printf 'A\n' > f; printf 'B\n' > g; exec 3<f 3<g; printf "[%s]" "$(cat <&3)"`,
				func(r *Runner) { r.Semantics, r.Dir = &sem, dir })
			if st != 0 {
				t.Fatalf("status %d, output %q", st, out)
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// `<>` is left out of it deliberately, and both answers have to agree: one
// descriptor that reads *and* writes cannot join a set on one side without
// modeling half the pair. The second open wins, as it does everywhere.
func TestAReadWriteDescriptorIsNotJoined(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		dir := t.TempDir()
		sem := CoreSemantics()
		sem.RedirectsUseEveryTarget = answer
		sem.RedirectErrorOnSpecialBuiltinFatal = No
		sem.ExecOpenedFdReachesACommand = Yes
		out, st := run(t, `exec 3<>a 3<>b; echo hi >&3; exec 3>&-; printf "[a=%s][b=%s]" "$(cat a)" "$(cat b)"`,
			func(r *Runner) { r.Semantics, r.Dir = &sem, dir })
		if want := "[a=][b=hi]"; out != want || st != 0 {
			t.Errorf("%v: got %q (status %d), want %q at 0", answer, out, st, want)
		}
	}
}

// What a *child naming the number itself* is handed, which is the half this
// does not fully model and must therefore not make worse.
//
// The shell that has the option forks a process to join the files, so its
// child sees a pipe carrying both. Here the table is rebuilt by descriptor
// number and a concatenation has no number, so the child is given the last
// file — which is what the number held before the fan-in existed. The
// alternative is closing 3 in the child, a new wrong answer where there was
// an old one.
//
// Both answers to the axis, because the row is about the child and not about
// the fan: it must read `B` either way.
func TestAChildNamingAFannedDescriptorGetsTheLastFile(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		dir := t.TempDir()
		sem := CoreSemantics()
		sem.RedirectsUseEveryTarget = answer
		sem.RedirectErrorOnSpecialBuiltinFatal = No
		sem.ExecOpenedFdReachesACommand = Yes
		out, st := run(t,
			`printf 'A\n' > f; printf 'B\n' > g; exec 3<f 3<g; /bin/sh -c 'cat <&3'`,
			func(r *Runner) { r.Semantics, r.Dir = &sem, dir })
		if want := "B\n"; out != want || st != 0 {
			t.Errorf("%v: got %q (status %d), want %q at 0", answer, out, st, want)
		}
	}
}
