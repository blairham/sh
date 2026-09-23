// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// The radix character under a locale whose radix is a comma.
//
// Measured 2026-09-22 on macOS 15 against zsh 5.9.2 with the host's own
// `de_DE.UTF-8` and `fr_FR.ISO8859-1`, which answer alike, under
// `LC_NUMERIC` alone so that the messages stay English and only the numbers
// move. Three rows: what a conversion **writes**, and whether a point and a
// comma are each read as a radix.
//
// This shell is the panel's third answer and the reason the axis is a policy
// rather than a boolean: it reads a point *as well as* the comma, where bash and
// ksh93 call `1.5` an invalid number.
//
// Run against a **fixture** locale database rather than the host's, so the case
// says the same thing on a machine with no such locale installed — what the host
// publishes is a fact about somebody's machine, and this row is about this
// dialect's reading of it. interp/localeradix.go holds the layout.
func TestTheRadixCharacterFollowsTheLocale(t *testing.T) {
	for _, c := range []struct {
		name, src, want, refuses string
	}{
		{name: "written", src: `printf '%.4f' 1`, want: "1,0000"},
		{name: "a point operand", src: `printf '%.2f' 1.5`, want: "1,50", refuses: ""},
		{name: "a comma operand", src: `printf '%.2f' 1,5`, want: "1,50", refuses: ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := radixRun(t, c.src)
			// The number is the last thing written, whether or not a refusal
			// came first — every column in the panel writes one either way, and
			// it is the leading number where the operand was refused.
			if !strings.HasSuffix(out, c.want) {
				t.Errorf("output %q, want it to end with %q", out, c.want)
			}
			if c.refuses == "" {
				if strings.Contains(out, "printf:") {
					t.Errorf("output %q, want the operand taken without complaint", out)
				}
				return
			}
			// The operand **as the script wrote it**. Naming anything else is
			// the failure mode this row exists for: a reader that rewrote the
			// operand to refuse it once reported a word nobody had typed.
			if !strings.Contains(out, "printf: "+c.refuses+": ") {
				t.Errorf("output %q, want a refusal naming %q", out, c.refuses)
			}
		})
	}
}

// radixRun runs src under a locale whose radix is a comma, from a fixture
// database holding just that one locale.
func radixRun(t *testing.T, src string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "xx_XX.UTF-8")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// The three lines interp/localeradix.go reads: the radix character, the
	// thousands separator and the grouping. Only the first is read.
	if err := os.WriteFile(filepath.Join(dir, "LC_NUMERIC"), []byte(",\n.\n3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := preset.Combined(t, dialecttest.Base{
		Dir: t.TempDir(), LocaleDatabase: root,
		Vars: map[string]string{"LC_NUMERIC": "xx_XX.UTF-8"},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out
}
