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

// Reading the locale: which name is in force for one category, and what
// encoding that name carries.
//
// The precedence is POSIX's and this package already read it for one category
// — the one that decides whether a length counts characters or bytes. These
// cases are about the other four being reachable and about the reading being
// one reading rather than two.
//
// Nothing here touches the machine's own locale: a runner is given the
// variables the case names and no others, so an unset one is genuinely unset.

// localeRunner is a runner holding exactly the variables given.
func localeRunner(t *testing.T, vars map[string]string) *Runner {
	t.Helper()
	d := syntax.Core()
	return newTestRunner(t, &Runner{Dialect: &d, Vars: vars})
}

// TestALocaleCategoryIsReadUnderLcAllAndOverLang.
//
// Three rungs, and the middle one is the category's own name — which is what
// makes this a function of the category rather than a fixed list. An *empty*
// value on either of the first two rungs is skipped rather than being an
// answer of its own, which is the rule that separates `LC_ALL= LANG=x` from
// `LC_ALL=y LANG=x`.
func TestALocaleCategoryIsReadUnderLcAllAndOverLang(t *testing.T) {
	for _, c := range []struct {
		name     string
		vars     map[string]string
		category string
		want     string
	}{
		{"nothing set at all", map[string]string{}, "LC_TIME", ""},
		{"LANG alone", map[string]string{"LANG": "l"}, "LC_TIME", "l"},
		{"the category beats LANG", map[string]string{"LANG": "l", "LC_TIME": "t"}, "LC_TIME", "t"},
		{"LC_ALL beats both", map[string]string{"LANG": "l", "LC_TIME": "t", "LC_ALL": "a"}, "LC_TIME", "a"},
		{"an empty LC_ALL is skipped", map[string]string{"LC_ALL": "", "LANG": "l"}, "LC_TIME", "l"},
		{"an empty category is skipped", map[string]string{"LC_TIME": "", "LANG": "l"}, "LC_TIME", "l"},
		// Another category's variable is another category's, which is the
		// half a single-list reading would get wrong.
		{"a neighbor decides nothing", map[string]string{"LC_TIME": "t"}, "LC_NUMERIC", ""},
		{"and the one asked for does", map[string]string{"LC_NUMERIC": "n", "LC_TIME": "t"}, "LC_NUMERIC", "n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := localeRunner(t, c.vars).LocaleFor(c.category); got != c.want {
				t.Errorf("LocaleFor(%q) = %q, want %q", c.category, got, c.want)
			}
		})
	}
}

// TestTheCodesetIsTheLocaleNamesOwnSpelling.
//
// As written and not canonicalized, which is the reading the one caller needs:
// a parameter whose whole content is what the encoding is *called* has to say
// `utf-8` for a name spelled that way and `UTF-8` for a name spelled the
// other. A name with no point in it carries no encoding, which is a different
// answer from carrying an empty one — both come back as the empty string here
// and the caller supplies the default, since nothing but the caller knows what
// a locale with no encoding is called.
func TestTheCodesetIsTheLocaleNamesOwnSpelling(t *testing.T) {
	for _, c := range []struct{ locale, want string }{
		{"", ""},
		{"C", ""},
		{"POSIX", ""},
		{"en_US", ""},
		{"en_US.UTF-8", "UTF-8"},
		{"en_US.utf-8", "utf-8"},
		{"en_US.ISO8859-1", "ISO8859-1"},
		{"zh_CN.GB18030", "GB18030"},
		{"C.UTF-8", "UTF-8"},
		// A modifier is not part of the encoding and is cut off first.
		{"sr_RS.UTF-8@latin", "UTF-8"},
		{"sr_RS@latin", ""},
		// A trailing point names an empty encoding, which is what it says.
		{"en_US.", ""},
	} {
		t.Run(c.locale, func(t *testing.T) {
			if got := LocaleCodeset(c.locale); got != c.want {
				t.Errorf("LocaleCodeset(%q) = %q, want %q", c.locale, got, c.want)
			}
		})
	}
}

// TestTheEncodingReadingAndTheCategoryReadingAreOneReading.
//
// The length of a string is counted in characters under a UTF-8 locale and in
// bytes otherwise, and that decision is made from the same two functions. The
// case is here rather than beside the other length cases because what it holds
// is that the two readings cannot drift apart: a shell whose `${#s}` says one
// thing about the locale and whose LocaleFor says another has two readings of
// the same variables.
func TestTheEncodingReadingAndTheCategoryReadingAreOneReading(t *testing.T) {
	d := syntax.Core()
	for _, c := range []struct {
		name   string
		vars   map[string]string
		locale string
		length string
	}{
		{"a UTF-8 LC_CTYPE", map[string]string{"LC_CTYPE": "en_US.UTF-8"}, "en_US.UTF-8", "5"},
		{"a single-byte LC_CTYPE", map[string]string{"LC_CTYPE": "en_US.ISO8859-1"}, "en_US.ISO8859-1", "6"},
		{"LC_ALL over it", map[string]string{"LC_ALL": "C", "LC_CTYPE": "en_US.UTF-8"}, "C", "6"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := localeRunner(t, c.vars)
			if got := r.LocaleFor("LC_CTYPE"); got != c.locale {
				t.Errorf("LocaleFor(LC_CTYPE) = %q, want %q", got, c.locale)
			}
			vars := map[string]string{"s": "héllo"}
			for k, v := range c.vars {
				vars[k] = v
			}
			if got := lengthUnder(t, d, vars); got != c.length {
				t.Errorf("${#s} = %q, want %q", got, c.length)
			}
		})
	}
}

// lengthUnder is `${#s}` in a shell that decodes the locale's encoding, with
// the variables given and no others.
func lengthUnder(t *testing.T, d syntax.Dialect, vars map[string]string) string {
	t.Helper()
	f, err := syntax.Parse("echo ${#s}", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out bytes.Buffer
	s := PosixSemantics()
	s.MultibyteEncodingIsHonored = Yes
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s, Vars: vars})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}
