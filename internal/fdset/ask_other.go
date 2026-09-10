// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package fdset

import "time"

// ReadableNow cannot be asked on a system without a descriptor set, so every
// stream counts as ready — the same answer a caller gives anything else it
// cannot ask.
func ReadableNow(int) bool { return true }

// ReadableWithin cannot be asked here either, and says so rather than
// answering "not ready" — a builtin whose contract is a timeout has to know
// the difference. See the select-backed file for what the second result is
// for.
func ReadableWithin(int, time.Duration) (ready, asked bool) { return false, false }

// Wait has nothing to wait on here, and says so rather than answering "nothing
// is ready" — which a caller would read as an idle moment that had passed.
func Wait(int, []int) ([]int, bool, error) { return nil, false, ErrUnsupported }

// Ready cannot be asked on a system with no descriptor set, and says so
// rather than answering "nothing is ready" — which a caller would read as a
// wait that had happened and found nothing. See the select-backed file.
func Ready([]int, []int, []int, *time.Duration) (r, w, e []int, err error) {
	return nil, nil, nil, ErrUnsupported
}
