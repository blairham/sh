// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"strings"
	"testing"
)

// A file is handed to a child as a file, and anything else is guarded.
//
// The guard exists because os/exec copies from a plain io.Reader on a
// goroutine, so two children reading one race. A *os.File is passed to the
// child as a descriptor and copied by nobody — wrapping it would *create* the
// copying the guard is for, and take with it everything a real descriptor
// gives a child: seeking, and a truthful answer to whether it is a terminal.
//
// Nothing observable distinguishes the two in a test that only reads bytes,
// which is why the contract is asserted here rather than through a script.
func TestOnlyANonFileStdinIsGuarded(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	r := newTestRunner(t, &Runner{Stdin: f})
	if got := r.lockedStdin(); got != any(f) {
		t.Errorf("a file was wrapped in %T, want the file itself", got)
	}

	r = newTestRunner(t, &Runner{Stdin: strings.NewReader("x")})
	if _, ok := r.lockedStdin().(*lockedReader); !ok {
		t.Errorf("a reader was left bare, want it guarded")
	}
}
