// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Two substitution bodies running at once each number from a table of their
// own, as each is a fork in the shell imitated: zsh 5.9.2 answers `cat <(echo
// <(true)) <(echo <(true))` with `/dev/fd/10 /dev/fd/10` every time. Here they
// are goroutines over one table, and whichever parked second found the first
// one's end on the number and stepped over it — `10 12` or `11 10` about one
// run in three on darwin (#6543).
//
// Run many times because the collision is a race: on the code before the fix
// a run of thirty met it on every attempt measured, and on darwin a run of the
// shape alone meets it about once in three.
func TestSiblingBodiesNumberFromTablesOfTheirOwn(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skipf("no cat on PATH: %v", err)
	}
	free := freeFdsAlong(t, 10, 200, 2)
	// The group's saved input is what moves the two outer substitutions off
	// the base, as zsh's own input does at the top of a script; each body
	// holds none of it, so both inner ones ask for the base first.
	src := `{ ` + cat + ` <(echo <(true)) <(echo <(true)); } </dev/null`
	const runs = 100
	for i := range runs {
		got := substNumbers(t, runOwnDescriptors(t, SavedStreamsAndPipeReadEndsTakeNumbers, src))
		if want := []int{free[0], free[0]}; !sameFds(got, want) {
			t.Fatalf("run %d of %d: got %v, want %v — one body's substitution was "+
				"counted in its sibling's table", i+1, runs, got, want)
		}
	}
}

// What starting a program leaves open for the child's life is in nobody's
// table: os/exec's pipe for a stream that is not a file, and on Linux a pidfd.
// A forking shell's background job is a process of its own and holds nothing
// in the shell, so `sleep 1 & echo <(true) <(true)` numbers from the bottom
// in zsh 5.9.2, exactly as the same echo alone does.
//
// The shell's input is a file so that os/exec holds a pipe for the two output
// streams only: one for an input that is not a file is closed as soon as the
// copy into it ends, which is a race against the numbering rather than a
// hold. And it is opened before the low numbers are filled, so that it is not
// itself on the base.
//
// The shell's output streams are buffers here, so os/exec copies `sleep`'s
// through pipes of its own, on the lowest free numbers, for as long as it
// runs; everything below the base is taken first so that those are numbers
// the substitutions ask for. Reading `$!` waits for the start, so the order is
// not a race, and before the fix the substitutions stepped over both on every
// run.
func TestAStartedProgramsPlumbingIsNotInTheNumbers(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("no sleep on PATH: %v", err)
	}
	in, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	fillBelow(t, 10)
	free := freeFdsAlong(t, 10, 200, 4)
	out, errs := runOwnDescriptorsReading(t, SavedStreamsAndPipeReadEndsTakeNumbers,
		sleep+` 0.3 & : $!; echo <(true) <(true) <(true) <(true); wait`, in)
	if errs != "" {
		t.Fatalf("stderr: %s", errs)
	}
	got := substNumbers(t, out)
	if !sameFds(got, free) {
		t.Errorf("got %v, want %v — a started program's own descriptor was counted", got, free)
	}
}

// A writing substitution publishes the second number its list offers, its
// reading end having taken the first (#6505) — and that does not depend on
// where `os.Pipe` happened to put the pipe. With one number below the base
// free, the pipe straddles the base, reading end below it and writing end on
// it, and the writing end used to be moved down onto the base once the
// original was closed: the reading end's number, published for the writing
// end. `exec 3>/dev/null; { echo <(true) <(true); } 3> >(true)` answered one
// low in more than half of the whole-table runs on Linux, where the test
// process's earlier children kept the low numbers full (#6543).
func TestAWritingSubstitutionStepsOverItsReadingEndWhereverThePipeLands(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	// One number free below the base and the rest taken, so that `os.Pipe`
	// puts the reading end there and the writing end on the base itself.
	fillBelow(t, 9)
	if fdIsOpen(9) {
		t.Fatal("9 is open — the hole this case needs below the base is not there")
	}
	free := freeFdsAlong(t, 10, 200, 4)
	for _, c := range []struct {
		src  string
		want []int
	}{
		{`echo >(true) >(true)`, []int{1, 2}},
		{`echo <(true) >(true) <(true)`, []int{0, 2, 1}},
		{`{ echo <(true) <(true); } 2> >(true)`, []int{1, 2}},
		{`exec 3>/dev/null; { echo <(true) <(true); } 3> >(true)`, []int{1, 2}},
	} {
		t.Run(c.src, func(t *testing.T) {
			got := substNumbers(t, runOwnDescriptors(t, SavedStreamsAndPipeReadEndsTakeNumbers, c.src))
			var want []int
			for _, i := range c.want {
				want = append(want, free[i])
			}
			if !sameFds(got, want) {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

// fillBelow opens the null device on every free number from 3 up to limit,
// for the rest of the test.
func fillBelow(t *testing.T, limit int) {
	t.Helper()
	for fd := 3; fd < limit; fd++ {
		if fdIsOpen(fd) {
			continue
		}
		f, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
	}
}

// A sibling's end can be let go of between the kernel refusing its number and
// the park asking whose it is, so the park finds the number open a moment ago
// and nothing on it now. It is free, and it is asked for again rather than
// stepped over: before that, about two runs in a hundred of `cat <(echo
// <(true)) <(echo <(true))` answered `10 12` on darwin, the second body taking
// its released number in place of the one let go of. Held open here by the
// test, so the order is not a race: the stand-in on the base is in the way of
// the outer substitution and let go of while the inner one is numbering.
func TestANumberLetGoOfMidParkIsTaken(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	fillBelow(t, 10)
	free := freeFdsAlong(t, 10, 200, 2)
	stand, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer stand.Close()
	if int(stand.Fd()) != free[0] {
		t.Fatalf("the stand-in is on %d, want the base %d", stand.Fd(), free[0])
	}
	misses := 0
	defer WhenAWishMissesForTest(func(n int) {
		if n != free[0] {
			return
		}
		// The outer substitution's park is the first to miss on the base,
		// and the inner one's the second.
		if misses++; misses == 2 {
			_ = stand.Close()
		}
	})()
	got := substNumbers(t, runOwnDescriptors(t, SavedStreamsAndPipeReadEndsTakeNumbers,
		`read x < <(echo <(true)); echo $x`))
	if want := free[:1]; !sameFds(got, want) || misses < 2 {
		t.Errorf("got %v after %d misses on the base, want %v — a number let go of "+
			"mid-park was stepped over", got, misses, want)
	}
}

// A fork holds what its parent held when it was made and nothing the parent
// makes afterwards: `cat <(echo <(true) <(true) <(true)) <(true) <(true)` is
// `/dev/fd/10 /dev/fd/11 /dev/fd/12` in zsh 5.9.2, the body's own numbers
// running on through the two its parent published once the body was already
// a process of its own. Here the parent's are in the one table, and were
// stepped over: `10 11 14` on every run (#6543).
func TestABodyHoldsNothingItsParentMakesLater(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skipf("no cat on PATH: %v", err)
	}
	free := freeFdsAlong(t, 10, 200, 3)
	// The saved input moves the parent's substitutions off the base, as
	// zsh's own input does at the top of a script.
	got := substNumbers(t, runOwnDescriptors(t, SavedStreamsAndPipeReadEndsTakeNumbers,
		`{ `+cat+` <(echo <(true) <(true) <(true)) <(true) <(true); } </dev/null`))
	if !sameFds(got, free) {
		t.Errorf("got %v, want %v — a number the parent published after the fork "+
			"was counted in the body's table", got, free)
	}
}

// And the same let-go landing inside the park's own question: between its
// reading whose the number is and its look at what is on it. The claim has to
// be read first — a sibling's claim is dropped after its close, so a claim
// read and then a number found empty means the sibling let go, and the number
// is taken. Looked at first, the number was open, the claim then gone, and
// the park stepped over a number nobody held: about one whole-table run in
// twelve of TestSiblingBodiesNumberFromTablesOfTheirOwn on Linux (#6543).
//
// The stand-in is nobody's when the outer substitution asks, so it is in the
// way there; it becomes a sibling's when the inner one misses on it, and is
// let go of inside that one's question.
func TestASiblingLettingGoInsideTheParksQuestionIsTaken(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	fillBelow(t, 10)
	free := freeFdsAlong(t, 10, 200, 1)
	sib, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer sib.Close()
	if int(sib.Fd()) != free[0] {
		t.Fatalf("the sibling's stand-in is on %d, want the base %d", sib.Fd(), free[0])
	}
	misses, asked := 0, 0
	var drop func()
	defer WhenAWishMissesForTest(func(n int) {
		if n == free[0] {
			if misses++; misses == 2 {
				drop = ClaimForASiblingForTest(free[0])
			}
		}
	})()
	defer BetweenClaimAndLookForTest(func(fd int) {
		if fd == free[0] && drop != nil {
			if asked++; asked == 1 {
				_ = sib.Close()
				drop()
			}
		}
	})()
	got := substNumbers(t, runOwnDescriptors(t, SavedStreamsAndPipeReadEndsTakeNumbers,
		`read x < <(echo <(true)); echo $x`))
	if !sameFds(got, free) || asked != 1 {
		t.Errorf("got %v after %d let-goes on the base, want %v — a number a sibling "+
			"let go of mid-question was stepped over", got, asked, free)
	}
}

// And a file test asks about the substitution's pipe where its published
// number is borrowed and the pipe is parked on another: a number held in this
// process by a descriptor no fork of the shell would hold — a sibling's end,
// here a regular file claimed as one — is published, and the pipe goes
// elsewhere. `test` stats the path in this process, so it has to take the
// mapping an open takes, or it answers about the file (#6900). bash 5.3,
// ksh93u+ and zsh 5.9.2 all answer yes to both rows, where nothing is in the
// way.
func TestAFileTestOnABorrowedNumberAsksAboutThePipe(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	want := freeFdsAlong(t, 10, 200, 1)[0]
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, nil, 0o444); err != nil {
		t.Fatal(err)
	}
	var extra []*os.File
	var on *os.File
	for on == nil {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case int(f.Fd()) == want:
			on = f
		case int(f.Fd()) > want:
			t.Skipf("could not put a file on %d", want)
		default:
			extra = append(extra, f)
		}
	}
	for _, f := range extra {
		_ = f.Close()
	}
	defer func() { _ = on.Close() }()
	defer ClaimForASiblingForTest(want)()
	for _, src := range []string{
		`test -p <(true) && echo yes`,
		`test -w >(true) && echo yes`,
	} {
		out := runSubstPlacement(t, SubstitutionEndsWhereAnyDescriptorGoes, AllocateDescriptorsFromTen, nil, src+`; echo <(true)`, nil)
		if got, path, _ := strings.Cut(strings.TrimSpace(out), "\n"); got != "yes" || path != "/dev/fd/"+strconv.Itoa(want) {
			t.Errorf("%s: got %q, want yes from a substitution published on %d", src, out, want)
		}
	}
}
