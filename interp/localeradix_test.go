// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A locale database holding one locale whose radix character is a comma.
//
// A fixture rather than the host's own, because what the host publishes is a
// fact about somebody's machine: these cases are about the three readings of a
// radix that is not the point, and a machine with no `de_DE.UTF-8` installed
// would pass every one of them by never reaching the question.
//
// The layout is the one interp/localeradix.go reads: a directory per locale
// holding `LC_NUMERIC`, whose three lines are the radix character, the
// thousands separator and the grouping.
func radixFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "xx_XX.UTF-8")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "LC_NUMERIC"), []byte(",\n.\n3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// The policy decides what a number is written with and what it is read at, and
// the three values are three different shells' answers.
//
// Named after no shell, which is this package's rule: the values are the axis's
// and each dialect's own suite says which one it holds. What is asserted here is
// that the mechanism answers each of them — the writer, the reader, and the
// refusal of the point where a dialect does not read one.
//
// The row that separates the two locale-reading values is `1.5` as an operand:
// under the stricter one it is not a number at all, and the shells that hold it
// say so out loud.
func TestTheRadixPolicyDecidesWhatIsWrittenAndRead(t *testing.T) {
	for _, c := range []struct {
		name         string
		policy       interp.RadixPolicy
		written      string
		point, comma string
	}{
		{
			name:   "never consulting the locale",
			policy: interp.RadixIsAlwaysThePoint,
			// The C locale's answer to everything, which is what this value
			// means: the fixture is never opened.
			written: "1.0000", point: "1.50", comma: "",
		},
		{
			name:    "the locale's radix alone",
			policy:  interp.RadixIsTheLocalesOwn,
			written: "1,0000", point: "", comma: "1,50",
		},
		{
			name:    "the locale's radix or the point",
			policy:  interp.RadixIsTheLocalesOwnOrThePoint,
			written: "1,0000", point: "1,50", comma: "1,50",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			run := func(src string) (string, string) {
				t.Helper()
				return radixRun(t, c.policy, src)
			}
			if out, errs := run(`printf '%.4f' 1`); out != c.written || errs != "" {
				t.Errorf(`printf '%%.4f' 1 wrote %q (stderr %q), want %q`, out, errs, c.written)
			}
			for _, operand := range []struct{ arg, want string }{
				{"1.5", c.point},
				{"1,5", c.comma},
			} {
				out, errs := run(`printf '%.2f' ` + operand.arg)
				if operand.want == "" {
					// Not a number under this policy, so the dialect's own
					// refusal is what comes out and the conversion still
					// writes a zero — which is every panel column's shape.
					if errs == "" {
						t.Errorf("%s: stderr is empty, want the operand refused", operand.arg)
					}
					continue
				}
				if out != operand.want || errs != "" {
					t.Errorf("%s: wrote %q (stderr %q), want %q", operand.arg, out, errs, operand.want)
				}
			}
		})
	}
}

// A locale the database says nothing about, and the C locale itself, are the
// point under every policy — which is what keeps the axis off the common path.
//
// The second half is the one worth pinning: a value the axis has no answer for
// is a refusal only where a locale with its own radix is really in force, so a
// shell with no answer running under `LC_ALL=C` must be silent rather than
// stopped.
func TestARadixThatIsThePointAsksNobodyAnything(t *testing.T) {
	for _, c := range []struct{ name, locale string }{
		{"the C locale", "C"},
		{"a locale with no data", "zz_ZZ.UTF-8"},
		{"no locale named at all", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, policy := range []interp.RadixPolicy{
				interp.RadixUnspecified,
				interp.RadixIsAlwaysThePoint,
				interp.RadixIsTheLocalesOwn,
				interp.RadixIsTheLocalesOwnOrThePoint,
			} {
				out, errs := radixRunIn(t, policy, c.locale, `printf '%.2f' 1.5`)
				if out != "1.50" || errs != "" {
					t.Errorf("policy %d in %q: wrote %q (stderr %q), want 1.50 and silence",
						policy, c.locale, out, errs)
				}
			}
		})
	}
}

// An axis nothing answered, reached by a locale that really has its own radix,
// refuses by name rather than taking a side.
func TestAnUnansweredRadixPolicyRefuses(t *testing.T) {
	_, errs := radixRun(t, interp.RadixUnspecified, `printf '%.4f' 1`)
	if !strings.Contains(errs, "radix character") {
		t.Errorf("stderr = %q, want the axis named in the refusal", errs)
	}
}

func radixRun(t *testing.T, policy interp.RadixPolicy, src string) (out, errs string) {
	t.Helper()
	return radixRunIn(t, policy, "xx_XX.UTF-8", src)
}

// radixRunIn runs src with the fixture database and one locale in force, and
// answers the two streams apart — the refusals here are about which stream
// says what.
func radixRunIn(t *testing.T, policy interp.RadixPolicy, locale, src string) (out, errs string) {
	t.Helper()
	var o, e strings.Builder
	sem := interp.PosixSemantics()
	sem.NumberRadix = policy
	// testrunner:bare — the subject is the locale database, which is a field
	// this test sets to a fixture of its own, and the runs write nothing.
	r := &interp.Runner{
		Stdout: &o, Stderr: &e,
		Semantics:      &sem,
		LocaleDatabase: radixFixture(t),
		Vars:           map[string]string{"LC_ALL": locale},
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return o.String(), e.String()
}
