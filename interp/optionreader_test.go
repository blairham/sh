// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// An OptionReader answers what NamedOption answers, for every name and in
// every state, including the names a runner claims for itself after the
// reader was resolved: a negated spelling, the negative of its name for being
// interactive, and a name it declared. Those three are why the reader asks
// the runner at all, and each has a runner below that a reader resolving only
// the substrate's tables would get wrong (#6363).
//
// The second and third runners are contrived on purpose. A negated spelling
// or an interactive negative that is *also* one of the substrate's own names
// is what reaches the reader's guards — a spelling that is not one of them
// falls back to NamedOption whether the guards are there or not.
func TestAnOptionReaderAgreesWithNamedOption(t *testing.T) {
	t.Parallel()
	// Every name the substrate holds beyond the common set, so that a name
	// the runner never declared is asked too: the reader holds its row, and
	// only the runner can say the name is not this shell's.
	names := []string{
		"clobber", "noclobber", "held", "interactive", "nointeractive", "no-such-name",
		"errexit", "nounset", "xtrace", "verbose", "noglob", "noexec", "allexport", "monitor",
		"emacs", "nolog", "braceexpand", "interactive-comments", "pipefail", "hashall",
		"trackall", "histignoredups", "posix", "errtrace", "functrace", "history",
		"histexpand", "keyword", "onecmd", "physical", "privileged", "stdin", "debug",
		"bgnice", "rc", "multiline", "viraw", "globstar", "gmacs", "letoctal", "markdirs",
		"showme",
	}
	for _, tc := range []struct {
		what        string
		negated     map[string]string
		declared    []string
		interactive string
	}{
		{"a negated spelling and a declared name", map[string]string{"clobber": "noclobber"}, []string{"held", "emacs"}, "nointeractive"},
		{"a substrate name spelled negated", map[string]string{"xtrace": "verbose"}, nil, "nointeractive"},
		{"a substrate name as the interactive negative", nil, []string{"interactive"}, "noglob"},
	} {
		s := PosixSemantics()
		s.InteractiveOptionName = "interactive"
		s.NonInteractiveOptionName = tc.interactive
		r := newTestRunner(t, &Runner{Semantics: &s, Name: "sh"})
		if len(tc.declared) > 0 {
			r.AddSetOptions(tc.declared...)
		}
		if tc.negated != nil {
			r.AddNegatedSetOptions(tc.negated)
		}
		seen := map[string]bool{}
		all := append([]string{}, names...)
		for _, o := range r.ListedOptions() {
			all = append(all, o.Name)
		}
		compared, moved := 0, 0
		for _, name := range all {
			if seen[name] {
				continue
			}
			seen[name] = true
			read := NewOptionReader(name)
			check := func(when string) {
				t.Helper()
				want, _ := r.NamedOption(name)
				if got := read.Read(r); got != want {
					t.Errorf("%s: %s %s: reader = %v, NamedOption = %v", tc.what, name, when, got, want)
				}
				compared++
			}
			check("as it stands")
			was, _ := r.NamedOption(name)
			if r.ApplyNamedOption(name, !was) == 0 {
				check("moved")
				moved++
				r.ApplyNamedOption(name, was)
			}
		}
		if compared < len(names) || moved < 8 {
			t.Fatalf("%s: %d comparisons, %d of them moved: too few to say anything", tc.what, compared, moved)
		}
	}
}
