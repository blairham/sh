// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `ulimit -a` renders the dialect's own table — see UlimitListingRow — so
// the tests hand it a table and fake limits and assert the exact page.

func ulimitListingRun(t *testing.T, rows []UlimitListingRow, hard bool) (string, string, int) {
	t.Helper()
	src := "ulimit -a"
	if hard {
		src = "ulimit -Ha"
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.UlimitBlockIsKilobyte = No
	var out, errs bytes.Buffer
	r := &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Name: "testsh",
		Diagnostics: &Diagnostics{UlimitListing: rows},
	}
	r.GetRlimit = func(res Resource) (int64, int64, error) {
		switch res {
		case ResourceFileSize:
			return 1024 * 100, 1024 * 200, nil
		case ResourceCPUTime:
			return RlimitInfinity, RlimitInfinity, nil
		}
		return 42, 42, nil
	}
	r.SetRlimit = func(Resource, int64, int64) error { return nil }
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	return out.String(), errs.String(), st
}

// TestUlimitListingRendersTheDialectsTable: live rows in their units, fixed
// rows verbatim, `unlimited` where nothing bounds, and -H reading the hard
// column.
func TestUlimitListingRendersTheDialectsTable(t *testing.T) {
	rows := []UlimitListingRow{
		{Prefix: "file(blocks)   ", Res: ResourceFileSize},
		{Prefix: "time(seconds)  ", Res: ResourceCPUTime, Scale: 1},
		{Prefix: "pipe(bytes)    ", Fixed: "512"},
	}
	out, errs, st := ulimitListingRun(t, rows, false)
	want := "file(blocks)   200\ntime(seconds)  unlimited\npipe(bytes)    512\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("got %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
	out, _, _ = ulimitListingRun(t, rows, true)
	if !strings.HasPrefix(out, "file(blocks)   400\n") {
		t.Errorf("got %q, want -H reading the hard column", out)
	}
}

// TestUlimitListingWithoutATableIsRefused as the unanswered question it is.
func TestUlimitListingWithoutATableIsRefused(t *testing.T) {
	out, errs, st := ulimitListingRun(t, nil, false)
	if out != "" || st != 2 || !strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("out %q errs %q status %d, want the refusal", out, errs, st)
	}
}
