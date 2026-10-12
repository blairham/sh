// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"os"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A substitution that wishes for the number a body's private copy is sitting
// on gets that number.
//
// The copies used to sit above everything a dialect could publish, so no wish
// ever reached one. That floor was 64 or more in every dialect, which grew
// the kernel's descriptor table on the first pipeline of every run (#6402), so
// they are asked for in the middle of the first table now — where a wish
// *can* reach one, in a table crowded enough. A fork holds no such copy, so
// the number is free in the shell this one imitates and the answer must not
// step over it.
//
// Crowded on purpose: every free number below the band is taken first, so the
// lowest free number — ksh93's rule — is the band's first, and that is where
// the background body's copy of its input lands. Measured against the
// kernel's own answer taken before the run, for the reason the placement
// cases beside this one give.
func TestASubstitutionStepsOntoAPrivateCopyRatherThanOverIt(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	var crowd []int
	defer func() {
		for _, fd := range crowd {
			_ = syscall.Close(fd)
		}
	}()
	for {
		fd, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(syscall.Stdin),
			uintptr(syscall.F_DUPFD_CLOEXEC), uintptr(3))
		if errno != 0 {
			t.Fatalf("crowding the table: %v", errno)
		}
		if int(fd) >= PrivateCopyFloorForTest {
			_ = syscall.Close(int(fd))
			break
		}
		crowd = append(crowd, int(fd))
	}
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	// The open above took the band's first number itself, so it is moved out
	// of the way: what has to sit there during the run is the body's copy.
	g, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(),
		uintptr(syscall.F_DUPFD_CLOEXEC), uintptr(200))
	if errno != 0 {
		t.Fatalf("moving the input out of the band: %v", errno)
	}
	_ = f.Close()
	f = os.NewFile(g, os.DevNull)
	want := lowestFreeFds(t, 1)
	if want[0] != PrivateCopyFloorForTest {
		t.Fatalf("the lowest free number is %d, want the band's first (%d) — the "+
			"table was not crowded the way this case needs", want[0], PrivateCopyFloorForTest)
	}
	got := substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheLowestFreeNumber,
		AllocateDescriptorsFromTen, nil, "{ echo <(true); } & wait", f))
	if !sameFds(got, want) {
		t.Errorf("got %v, want %v — the substitution stepped over the background "+
			"body's private copy of its input, which a fork would not hold", got, want)
	}
}

// A substitution whose own pipe sits on the number it wishes for gets that
// number, even where the next number is held by a private copy.
//
// The original the park duplicates is closed only afterwards, so a wish for
// the number it is sitting on misses, and reparkPreferred takes the number
// once it is free. A wish further down the list that missed because a
// private copy held it used to be published there and then — with the
// descriptor parked above the floor, where reparkPreferred never looks — so
// the substitution stepped over its own free number. Since a substitution's
// shell end became a private copy (#6406), the previous substitution's end is
// exactly such a copy: thirty-five live substitutions in the ksh dialect read
// `… 30 32 31 33 …`.
//
// Built the same way here: every free number below the band's predecessor is
// crowded out, so the first substitution publishes that number and its shell
// end — held open by a body reading a pipe nobody writes to until the command
// ends — sits in the band just above the next one's own pipe.
func TestASubstitutionDoesNotStepOverItsOwnFreeNumber(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	var crowd []int
	defer func() {
		for _, fd := range crowd {
			_ = syscall.Close(fd)
		}
	}()
	for {
		fd, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(syscall.Stdin),
			uintptr(syscall.F_DUPFD_CLOEXEC), uintptr(3))
		if errno != 0 {
			t.Fatalf("crowding the table: %v", errno)
		}
		if int(fd) >= PrivateCopyFloorForTest-1 {
			_ = syscall.Close(int(fd))
			break
		}
		crowd = append(crowd, int(fd))
	}
	want := lowestFreeFds(t, 2)
	if want[0] != PrivateCopyFloorForTest-1 || want[1] != PrivateCopyFloorForTest {
		t.Fatalf("the lowest free numbers are %v, want %d and %d — the table was not "+
			"crowded the way this case needs", want, PrivateCopyFloorForTest-1, PrivateCopyFloorForTest)
	}
	got := substNumbers(t, runSubstPlacement(t, SubstitutionEndsAtTheLowestFreeNumber,
		AllocateDescriptorsFromTen, nil, "echo >(read x) <(true)", nil))
	if !sameFds(got, want) {
		t.Errorf("got %v, want %v — the second substitution stepped over the number "+
			"its own pipe was sitting on", got, want)
	}
}
