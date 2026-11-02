// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import "os"

// WhenAWishMissesForTest has f called with each wish's number between the
// kernel refusing it and the park asking whose it is, until restore is
// called. See afterAWishMissed.
func WhenAWishMissesForTest(f func(n int)) (restore func()) {
	afterAWishMissed = f
	return func() { afterAWishMissed = nil }
}

// ClaimForASiblingForTest claims fd for a runner nothing else descends from —
// a sibling's substitution end, as far as any park can tell — and answers what
// drops the claim the way the sibling's own close does, after the close. See
// foreignfds_unix.go.
func ClaimForASiblingForTest(fd int) (drop func()) {
	stamp := claimFd(fd, &fdLineage{forkedAt: fdClock.Add(1)}, false)
	return func() { dropFdClaimMade(fd, stamp) }
}

// BetweenClaimAndLookForTest has f called between a park's reading of a claim
// and its look at the descriptor, until restore is called. See
// betweenClaimAndLook.
func BetweenClaimAndLookForTest(f func(fd int)) (restore func()) {
	betweenClaimAndLook = f
	return func() { betweenClaimAndLook = nil }
}

// BetweenAFilesStatAndOpenForTest has f called with the path between a
// redirection's look at a regular file and its open, until restore is called.
// See betweenAFilesStatAndOpen.
func BetweenAFilesStatAndOpenForTest(f func(path string)) (restore func()) {
	betweenAFilesStatAndOpen = f
	return func() { betweenAFilesStatAndOpen = nil }
}

// WhileAFileIsLowForTest has f called with the path and the descriptor
// between a redirection's open of a regular file and its move, until restore
// is called. See whileAFileIsLow.
func WhileAFileIsLowForTest(f func(path string, fd int)) (restore func()) {
	whileAFileIsLow = func(path string, file *os.File) { f(path, int(file.Fd())) }
	return func() { whileAFileIsLow = nil }
}
