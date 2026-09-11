// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// What `set -C` protects is a *regular* file, and openThroughNoclobber is
// where that line is drawn. See interp/noclobberopen.go for the panel.

// runNoclobber runs src in a scratch directory with the given wordings, and
// hands back what the shell said and the status it left.
func runNoclobber(t *testing.T, src string, dg interp.Diagnostics) (string, int, string) {
	t.Helper()
	dir := t.TempDir()
	out, st := run(t, src, func(r *interp.Runner) {
		r.Dir = dir
		r.Diagnostics = &dg
	})
	return out, st, dir
}

// TestNoclobberWritesToAFileThatIsNotRegular is the whole of #1711: a
// character device is not a file anybody can overwrite, so the option has
// nothing to refuse and `2>/dev/null` has to keep working.
func TestNoclobberWritesToAFileThatIsNotRegular(t *testing.T) {
	out, st, _ := runNoclobber(t, `set -C; echo probe > /dev/null; printf "[%s]" "$?"`, interp.Diagnostics{})
	if out != "[0]" {
		t.Errorf("out = %q, want the write to go through at status 0", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// TestNoclobberStillRefusesARegularFile is the control, and the half that
// must not be lost to the half above: the option exists for this case.
func TestNoclobberStillRefusesARegularFile(t *testing.T) {
	dg := interp.Diagnostics{NoclobberRefusal: "%[1]s: cannot overwrite existing file"}
	out, _, dir := runNoclobber(t, `set -C; echo first > f; echo second > f`, dg)
	if !strings.Contains(out, "cannot overwrite existing file") {
		t.Errorf("out = %q, want the refusal", out)
	}
	b, err := os.ReadFile(filepath.Join(dir, "f"))
	if err != nil || string(b) != "first\n" {
		t.Errorf("f = %q (%v), want the first write kept", b, err)
	}
}

// TestNoclobberFollowsTheSymlinkRatherThanTheName. A symlink is refused or
// allowed by what it reaches, which is what keeps the rule about file types
// rather than about paths — and a symlink to nothing is refused, because
// there is no type to find.
func TestNoclobberFollowsTheSymlinkRatherThanTheName(t *testing.T) {
	for _, tc := range []struct {
		name, target string
		wantStatus   int
	}{
		{"to a device", "/dev/null", 0},
		{"to nothing", "/nonexistent-target-of-a-link", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Symlink(tc.target, filepath.Join(dir, "link")); err != nil {
				t.Fatal(err)
			}
			out, st := run(t, `set -C; echo probe > link`, func(r *interp.Runner) { r.Dir = dir })
			if st != tc.wantStatus {
				t.Errorf("status = %d (%q), want %d", st, out, tc.wantStatus)
			}
		})
	}
}

// TestNoclobberRefusalCoversAFailedOpenIsTheDialectsChoice. A name holding
// something that is not a regular file and cannot be opened either — a
// directory is the portable one — is reported two ways, and the switch is a
// wording rather than an axis: nothing about *what happens* differs.
func TestNoclobberRefusalCoversAFailedOpenIsTheDialectsChoice(t *testing.T) {
	for _, tc := range []struct {
		name    string
		covers  bool
		wantSub string
		notSub  string
	}{
		{"the open's own reason", false, "is a directory", "will not overwrite"},
		{"the option's refusal", true, "will not overwrite", "is a directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
				t.Fatal(err)
			}
			dg := interp.Diagnostics{
				CannotCreate:                      "%[1]s: %[2]s",
				NoclobberRefusal:                  "%[1]s: will not overwrite",
				NoclobberRefusalCoversAFailedOpen: tc.covers,
			}
			out, st := run(t, `set -C; echo probe > d`, func(r *interp.Runner) {
				r.Dir, r.Diagnostics = dir, &dg
			})
			if st == 0 {
				t.Fatalf("status 0 for a write to a directory: out = %q", out)
			}
			if !strings.Contains(strings.ToLower(out), tc.wantSub) {
				t.Errorf("out = %q, want it to contain %q", out, tc.wantSub)
			}
			if strings.Contains(strings.ToLower(out), tc.notSub) {
				t.Errorf("out = %q, want it not to contain %q", out, tc.notSub)
			}
		})
	}
}

// TestNoclobberFallbackIsWordedAsAnOpen. The second open creates nothing, and
// one dialect's verb says so. Both answers here, because a wording asserted
// in one direction cannot tell a switch from a constant.
func TestNoclobberFallbackIsWordedAsAnOpen(t *testing.T) {
	for _, tc := range []struct {
		name     string
		isAnOpen bool
		want     string
	}{
		{"created", false, "sh: cannot create d: is a directory\n"},
		{"opened", true, "sh: cannot open d: is a directory\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
				t.Fatal(err)
			}
			dg := interp.Diagnostics{
				LowercaseReason:           true,
				NoclobberFallbackIsAnOpen: tc.isAnOpen,
			}
			out, _ := run(t, `set -C; echo probe > d`, func(r *interp.Runner) {
				r.Dir, r.Diagnostics = dir, &dg
			})
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}
