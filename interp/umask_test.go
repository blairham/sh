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

// umaskRun runs src with a mask kept in a variable rather than in the process,
// which is the point of the hook: nothing here touches the machine's own mask,
// so the tests can assert on exact values and can run in parallel with
// anything else.
func umaskRun(t *testing.T, start int, tweak func(*Semantics), src string) (string, int, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.UmaskPrintsFourDigits = Yes
	sem.UmaskSetWithSPrints = No
	if tweak != nil {
		tweak(&sem)
	}
	dg := Diagnostics{}
	held := start
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	r.SetUmask = func(mask int) (int, error) { old := held; held = mask; return old, nil }
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st, held
}

// umaskSymbolicRun is umaskRun with a Diagnostics of the caller's choosing,
// for the complaints that differ between dialects.
func umaskSymbolicRun(t *testing.T, start int, dg Diagnostics, src string) (string, int, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.UmaskPrintsFourDigits = Yes
	sem.UmaskSetWithSPrints = No
	held := start
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	r.SetUmask = func(mask int) (int, error) { old := held; held = mask; return old, nil }
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st, held
}

// Reading must not change it — which takes a set and a set-back, the system
// call offering no way to ask.
func TestUmaskReadingLeavesItAlone(t *testing.T) {
	out, st, held := umaskRun(t, 0o022, nil, `umask`)
	if strings.TrimSpace(out) != "0022" || st != 0 {
		t.Errorf("said %q status %d, want 0022", out, st)
	}
	if held != 0o022 {
		t.Errorf("the mask is now %#o, want it unchanged at 022", held)
	}
	// Three digits where the dialect drops the leading zero.
	out, _, _ = umaskRun(t, 0o022, func(s *Semantics) { s.UmaskPrintsFourDigits = No }, `umask`)
	if strings.TrimSpace(out) != "022" {
		t.Errorf("said %q, want 022", out)
	}
}

// Setting changes it, and says nothing.
func TestUmaskSetting(t *testing.T) {
	out, st, held := umaskRun(t, 0o022, nil, `umask 077`)
	if out != "" || st != 0 {
		t.Errorf("said %q status %d, want silence", out, st)
	}
	if held != 0o077 {
		t.Errorf("the mask is %#o, want 077", held)
	}
	// A leading zero is the same number.
	if _, _, held := umaskRun(t, 0, nil, `umask 0077`); held != 0o077 {
		t.Errorf("0077 gave %#o", held)
	}
	// And it is octal, not decimal: 022 is 18, not 22.
	if _, _, held := umaskRun(t, 0, nil, `umask 022`); held != 0o022 {
		t.Errorf("022 gave %#o, want 0o022", held)
	}
}

// `-S` writes the permissions the mask allows, not the bits it removes.
func TestUmaskSymbolic(t *testing.T) {
	for _, c := range []struct {
		mask int
		want string
	}{
		{0o022, "u=rwx,g=rx,o=rx"},
		{0o077, "u=rwx,g=,o="},
		{0, "u=rwx,g=rwx,o=rwx"},
		{0o777, "u=,g=,o="},
		{0o111, "u=rw,g=rw,o=rw"},
	} {
		out, _, _ := umaskRun(t, c.mask, nil, `umask -S`)
		if strings.TrimSpace(out) != c.want {
			t.Errorf("%#o = %q, want %q", c.mask, strings.TrimSpace(out), c.want)
		}
	}
}

// `umask -S mask` sets, and echoes only where the dialect says so.
func TestUmaskSymbolicWithAMask(t *testing.T) {
	out, _, held := umaskRun(t, 0o022, func(s *Semantics) { s.UmaskSetWithSPrints = Yes }, `umask -S 077`)
	if strings.TrimSpace(out) != "u=rwx,g=,o=" {
		t.Errorf("said %q, want the new mask echoed", out)
	}
	if held != 0o077 {
		t.Errorf("the mask is %#o, want it set to 077", held)
	}
	out, _, held = umaskRun(t, 0o022, nil, `umask -S 077`)
	if out != "" {
		t.Errorf("said %q, want silence where the dialect does not echo", out)
	}
	if held != 0o077 {
		t.Errorf("the mask is %#o, want it set even so", held)
	}
}

// A mask it cannot read changes nothing.
func TestUmaskRejectsWhatItCannotRead(t *testing.T) {
	for _, bad := range []string{"9999", "abc", "-1", "1777", ""} {
		out, st, held := umaskRun(t, 0o022, nil, `umask `+quoteArg(bad))
		if st == 0 {
			t.Errorf("%q: status 0, want a failure (said %q)", bad, out)
		}
		if held != 0o022 {
			t.Errorf("%q: the mask changed to %#o", bad, held)
		}
	}
}

// Without a hook it says so rather than pretending, which is the whole of
// what went wrong before: a umask that reported success and did nothing.
func TestUmaskWithoutAHook(t *testing.T) {
	f, err := syntax.Parse(`umask 077`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem := permissive()
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if st == 0 {
		t.Errorf("status 0 with no umask to set — the failure this exists to stop")
	}
	if buf.String() == "" {
		t.Errorf("said nothing")
	}
}

func quoteArg(s string) string {
	if s == "" {
		return `""`
	}
	return `"` + s + `"`
}

// `umask -p` prints the mask as a command that would set it again, which is
// what `saved=$(umask -p)` and `eval "$saved"` are for. bash alone has the
// letter; the other four refuse it as an option `umask` does not have.
func TestUmaskPrintsAReusableLine(t *testing.T) {
	has := func(s *Semantics) { s.UmaskHasTheReusableLetter = Yes }
	for _, tc := range []struct{ name, src, want string }{
		{"the octal form", `umask -p`, "umask 0022\n"},
		{"the symbolic form", `umask -p -S`, "umask -S u=rwx,g=rx,o=rx\n"},
		// A bundle, because bash reads one, and in either order.
		{"bundled", `umask -pS`, "umask -S u=rwx,g=rx,o=rx\n"},
		{"bundled the other way", `umask -Sp`, "umask -S u=rwx,g=rx,o=rx\n"},
		// Only the report takes the prefix: setting is silent with `-p`,
		// which is measured.
		{"setting is silent", `umask -p 077; umask`, "0077\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, _ := umaskRun(t, 0o022, has, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q status %d, want %q", out, st, tc.want)
			}
		})
	}

	// The `-S` echo of a mask it just set is bare, which is the other half
	// of "only the report takes the prefix" and needs the echo turned on.
	out, st, _ := umaskRun(t, 0o022, func(s *Semantics) {
		s.UmaskHasTheReusableLetter = Yes
		s.UmaskSetWithSPrints = Yes
	}, `umask -p -S 077`)
	if out != "u=rwx,g=,o=\n" || st != 0 {
		t.Errorf("out = %q status %d, want the bare symbolic form", out, st)
	}

	// And where the shell has not got the letter it is an option refused,
	// not a prefix quietly left off.
	out, st, _ = umaskRun(t, 0o022, func(s *Semantics) { s.UmaskHasTheReusableLetter = No }, `umask -p`)
	if !strings.Contains(out, "-p: invalid option") || st != 2 {
		t.Errorf("out = %q status %d, want the letter refused", out, st)
	}
}
