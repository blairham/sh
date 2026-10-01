// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// **`-t` and `-i` each have a ceiling, and they are not the same ceiling.**
// Measured against zsh 5.9.2 one value at a time: a timeout stops at 2^30 less
// one second, an interval at 0.999 of the largest 64-bit count of microseconds,
// and only the interval has a floor. A value past either is refused with the
// word quoted as written and status 1, before any file is opened.
func TestFlockTimeoutAndIntervalCeilings(t *testing.T) {
	rows := []struct {
		letter, value string
		refused       bool
	}{
		{"t", "1073741823", false},
		{"t", "1073741823.000001", true},
		{"t", "1073741824", true},
		{"t", "2e9", true},
		{"t", "1e100", true},
		{"t", "inf", true},
		{"t", "1e9", false},
		{"t", "-1e100", false},
		{"t", "-inf", false},
		{"t", "nan", false},
		{"t", "0", false},
		{"i", "1073741824", false},
		{"i", "1e10", false},
		{"i", "9214148664817.9", false},
		{"i", "9214148664817.93", true},
		{"i", "1e100", true},
		{"i", "inf", true},
		{"i", "-inf", true},
		{"i", "nan", false},
		{"i", "0.001", false},
		{"i", "0", true},
		{"i", "-1", true},
	}
	dir := t.TempDir()
	lock := filepath.Join(dir, "lock")
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var script, want strings.Builder
	var complaints []string
	for n, row := range rows {
		other := "-t 0"
		if row.letter == "t" {
			other = "-i 1"
		}
		fmt.Fprintf(&script, "zsystem flock %s -%s %s %s\nprint -r -- \"%d=$?\"\n",
			other, row.letter, row.value, lock, n)
		status := 0
		if row.refused {
			status = 1
			kind := "timeout"
			if row.letter == "i" {
				kind = "interval"
			}
			complaints = append(complaints, fmt.Sprintf("zsh:zsystem:%d: flock: invalid %s value: '%s'", 2*n+1, kind, row.value))
		}
		fmt.Fprintf(&want, "%d=%d\n", n, status)
	}
	out, st, errs := runZshSplitWithSystem(t, dir, script.String())
	if out != want.String() || st != 0 {
		t.Errorf("statuses = %q (status %d), want %q", out, st, want.String())
	}
	wantWholeLines(t, errs, complaints...)
	if got, want := len(splitLines(errs)), len(complaints); got != want {
		t.Errorf("stderr = %q, want exactly %d complaints", errs, want)
	}
}
