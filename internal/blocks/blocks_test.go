// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/treeguard"
)

// This package writes files for a living, so a test that forgot to take a
// temporary directory would write into the checked-out tree and pass.
func TestMain(m *testing.M) { os.Exit(treeguard.Run(m)) }

// A record is one line, and the characters a redirect is written with survive
// it. Escaping < > & would mangle most of the command lines worth keeping.
func TestARecordIsOneLineAndKeepsItsAngleBrackets(t *testing.T) {
	line, err := encode(Record{V: Version, ID: "X", Command: "a <in >out 2>&1 && b"})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(line), "\n"); n != 1 || !strings.HasSuffix(string(line), "\n") {
		t.Fatalf("record is %q, want exactly one trailing newline", line)
	}
	if !strings.Contains(string(line), `a <in >out 2>&1 && b`) {
		t.Fatalf("record is %q, want the command as written", line)
	}
	// The escaped forms, which are what the encoder produces by default and
	// what would make every redirect in the store unreadable.
	for _, escaped := range []string{"\\u003c", "\\u003e", "\\u0026"} {
		if strings.Contains(string(line), escaped) {
			t.Fatalf("record is %q, want %s left alone", line, escaped)
		}
	}
}

// Zero is meaningful for a status and for a duration, so both are written
// rather than omitted: a consumer that saw no `status` could not tell success
// from a field somebody forgot.
func TestZeroStatusAndDurationAreWritten(t *testing.T) {
	line, err := encode(Record{V: Version, ID: "X", Status: 0, DurationMs: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"status":0`, `"durationMs":0`} {
		if !strings.Contains(string(line), want) {
			t.Errorf("record is %q, want %s in it", line, want)
		}
	}
	// And the output fields are absent, because absent is how a block with no
	// output kept says so.
	for _, unwanted := range []string{`"output"`, `"outputBytes"`, `"truncated"`, `"streams"`} {
		if strings.Contains(string(line), unwanted) {
			t.Errorf("record is %q, want no %s when nothing was kept", line, unwanted)
		}
	}
}
