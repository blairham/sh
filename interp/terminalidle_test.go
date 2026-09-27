// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The arithmetic behind `Runner.TerminalIdle`, pointed at a file whose access
// time is known.
//
// **This is the positive control and it is the reason idleSince is a function
// of its own.** A test process has no terminal, so a test of TerminalIdle on
// a Runner can only ever watch the false branch — a check that cannot produce
// a positive, which is the one result a broken instrument also gives. An
// ordinary file has the same access time a terminal device has and `os.Chtimes`
// can put one anywhere, so the subtraction can be seen to fire.
func TestTheIdleTimeIsMeasuredFromTheAccessTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name string
		read time.Time
		want time.Duration
	}{
		{"read a minute ago", now.Add(-time.Minute), time.Minute},
		{"read this instant", now, 0},
		{"read an hour ago", now.Add(-time.Hour), time.Hour},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := os.Chtimes(path, c.read, c.read); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			// Opening the file does not read from it, so the access time is
			// still the one Chtimes wrote. Asserted rather than assumed: a
			// platform that stamped it on open would make every row zero and
			// the test would agree with itself about nothing.
			got, ok := idleSince(f, now)
			if !ok {
				t.Skip("this platform does not report an access time")
			}
			if got != c.want {
				t.Errorf("idleSince = %v, want %v", got, c.want)
			}
		})
	}
}

// And the false a Runner with no terminal gets, which is the branch every
// case in this tree takes. False rather than zero, because zero is a real
// answer — a terminal read from this instant, which the rows above show the
// arithmetic really can produce.
func TestAShellWithNoTerminalHasNoIdleTimeAtAll(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	if idle, held := r.TerminalIdle(); held {
		t.Errorf("TerminalIdle = %v, true — want false from a runner holding no terminal", idle)
	}
}
