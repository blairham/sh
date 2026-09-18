// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The case-change operators carry a *pattern* saying which characters to
// convert, and it is matched one character at a time.
//
// Found by the wild sweep. Three faults in one operator, and the worst was
// silent: the doubled forms parsed, discarded the pattern, and converted the
// whole string with status 0 — so a script asking for a subset got the lot.
func TestCaseChangeAppliesItsPattern(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// The silent one: only the characters the pattern matches.
		{"upper, a pattern", `x=abc; echo "${x^^[ab]}"`, "ABc"},
		{"lower, a pattern", `x=ABC; echo "${x,,[AB]}"`, "abC"},
		{"a pattern matching nothing", `x=abc; echo "${x^^[xyz]}"`, "abc"},
		{"a range", `x=a1b2; echo "${x^^[a-z]}"`, "A1B2"},
		{"a negated class", `x=abc; echo "${x^^[!a]}"`, "aBC"},
		// No pattern is every character, which is what `?` would say.
		{"upper, no pattern", `x=abc; echo "${x^^}"`, "ABC"},
		{"lower, no pattern", `x=ABC; echo "${x,,}"`, "abc"},
		{"an explicit ?", `x=abc; echo "${x^^?}"`, "ABC"},

		// The single forms: the first character, and only if it matches.
		{"upper first", `x=abc; echo "${x^}"`, "Abc"},
		{"lower first", `x=ABC; echo "${x,}"`, "aBC"},
		{"first, matching", `x=abc; echo "${x^a}"`, "Abc"},
		{"first, not matching", `x=abc; echo "${x^b}"`, "abc"},
		{"first, a class", `x=abc; echo "${x^[ab]}"`, "Abc"},
		// Only the first, even where a later character matches as well.
		{"first is not every", `x=hello; echo "${x^l}"`, "hello"},
		{"doubled is every", `x=hello; echo "${x^^l}"`, "heLLo"},

		// Toggle, both forms.
		{"toggle first", `x=abc; echo "${x~}"`, "Abc"},
		{"toggle all", `x=aBc; echo "${x~~}"`, "AbC"},
		{"toggle back", `x=ABC; echo "${x~~}"`, "abc"},

		// The pattern is expanded, so it may come from a variable.
		{"a pattern from a variable", `x=abc; p=b; echo "${x^^$p}"`, "aBc"},

		// Nothing to convert stays nothing rather than becoming empty.
		{"an empty value", `x=; echo "[${x^}]"`, "[]"},
		{"an unset name", `echo "[${undef^^}]"`, "[]"},

		// And it reaches an array's elements, each on its own.
		{"every element", `a=(ab cd); echo "${a[@]^^}"`, "AB CD"},
		{"one element", `a=(ab cd); echo "${a[0]^}"`, "Ab"},
		{"elements with a pattern", `a=(ab cd); echo "${a[@]^^[a]}"`, "Ab cd"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runGrammar(t, c.src, func(d *syntax.Dialect) { d.ParamCaseChange = true }, nil)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("said %q, want %q", strings.TrimSpace(out), c.want)
			}
		})
	}
}

// TestCaseConversionConsultsTheLocale — the policy docs/spec/semantics.md
// records: an explicit C or POSIX locale narrows case to ASCII and any other
// value is Unicode-aware. LC_ALL outranks LC_CTYPE outranks LANG. What an
// *unset* locale is is the next test's, because the panel splits on it.
func TestCaseConversionConsultsTheLocale(t *testing.T) {
	for _, tc := range []struct {
		name string
		vars map[string]string
		want string
	}{
		{"explicit C is ASCII alone", map[string]string{"LC_ALL": "C"}, "CAFé"},
		{"POSIX is the same narrowing", map[string]string{"LANG": "POSIX"}, "CAFé"},
		{"a UTF-8 locale cases beyond it", map[string]string{"LC_ALL": "en_US.UTF-8"}, "CAFÉ"},
		{"LC_ALL outranks LANG", map[string]string{"LC_ALL": "en_US.UTF-8", "LANG": "C"}, "CAFÉ"},
		{"LC_CTYPE outranks LANG", map[string]string{"LC_CTYPE": "C", "LANG": "en_US.UTF-8"}, "CAFé"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := syntax.Core()
			d.ParamCaseChange = true
			f, err := syntax.Parse(`x=café; echo "${x^^}"`, d)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			sem := permissive()
			vars := map[string]string{}
			for k, v := range tc.vars {
				vars[k] = v
			}
			r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Vars: vars, Env: []string{}})
			if _, rerr := r.Run(context.Background(), f); rerr != nil {
				t.Fatal(rerr)
			}
			if got := strings.TrimSpace(buf.String()); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// What an *unset* locale is — nothing naming one anywhere — is the dialect's
// answer rather than a rule this operator states, and it moves the case map
// with it.
//
// Measured 2026-09-11 under `env -i`, uppercasing `café` in each shell's own
// spelling: bash 5.3.15 gives `CAFÉ` for `${s^^}`, where ksh93u+ gives `CAFé`
// for `typeset -u` and zsh 5.9.2 gives `CAFé` for `${(U)s}`. Semantics.
// UnsetLocaleIsUnicodeAware (#2020), and the same answer decides a string's
// length and a `\u` escape's room.
func TestAnUnsetLocaleCasesByTheAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"Unicode-aware, as one of them reads it", Yes, "CAFÉ"},
		{"the C locale, as the rest read it", No, "CAFé"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := syntax.Core()
			d.ParamCaseChange = true
			f, err := syntax.Parse(`x=café; echo "${x^^}"`, d)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			sem := permissive()
			sem.UnsetLocaleIsUnicodeAware = tc.answer
			r := newTestRunner(t, &Runner{
				Stdout: &buf, Stderr: &buf, Semantics: &sem,
				Vars: map[string]string{}, Env: []string{},
			})
			if _, rerr := r.Run(context.Background(), f); rerr != nil {
				t.Fatal(rerr)
			}
			if got := strings.TrimSpace(buf.String()); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// And a value that is ASCII throughout never asks it: the two readings agree
// below 0x80, so the core cases `abc` with no dialect chosen rather than
// refusing an unanswered axis.
func TestAnASCIIValueNeverAsksTheUnsetLocaleAxis(t *testing.T) {
	d := syntax.Core()
	d.ParamCaseChange = true
	f, err := syntax.Parse(`x=abc; echo "${x^^}"`, d)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem := permissive()
	// Unspecified on purpose: reaching the axis would report, so a clean
	// answer is evidence it was never asked.
	sem.UnsetLocaleIsUnicodeAware = Unspecified
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem,
		Vars: map[string]string{}, Env: []string{},
	})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatal(rerr)
	}
	if got := strings.TrimSpace(buf.String()); got != "ABC" {
		t.Errorf("got %q, want %q with no question asked", got, "ABC")
	}
}

// TestEverySiteThatChangesCaseAsksTheSameLocale is the whole of #2027: four
// sites change case, and all four narrow to ASCII in the C locale.
//
// It is one table over four spellings rather than four tests, because the
// defect was **drift between the sites**, not any one of them being wrong on
// its own. #367 brought the two operators under the locale policy and left
// the attribute and the `:u`/`:l` modifier beside them calling
// strings.ToUpper directly, and nothing failed for it — each site had its own
// test and each test agreed with its own site. A test per site is exactly the
// shape that cannot see this class of bug.
//
// Measured 2026-09-11 under LC_ALL=C, uppercasing `café`: bash 5.3.15 gives
// `CAFé` for `declare -u`, ksh93u+ gives `CAFé` for `typeset -u`, and zsh
// 5.9.2 gives `CAFé` for both `typeset -u` and `${s:u}`. The panel agrees, so
// this is a correction and not an axis.
func TestEverySiteThatChangesCaseAsksTheSameLocale(t *testing.T) {
	sites := []struct {
		name   string
		src    string
		enable func(*syntax.Dialect)
		before func(*Runner)
	}{{
		name:   "the operator",
		src:    `x=café; echo "${x^^}"`,
		enable: func(d *syntax.Dialect) { d.ParamCaseChange = true },
	}, {
		name:   "the expansion flag",
		src:    `x=café; echo "${(U)x}"`,
		enable: func(d *syntax.Dialect) { d.ParamExpansionFlags = true },
	}, {
		name: "the attribute, folded at assignment",
		src:  `typeset -u x=café; echo "$x"`,
		before: func(r *Runner) {
			sem := *r.Semantics
			sem.DeclareOptions = "aAilprux"
			r.Semantics = &sem
		},
	}, {
		name: "the modifier",
		src:  `x=café; echo "${x:u}"`,
		before: func(r *Runner) {
			sem := *r.Semantics
			sem.SubstringRangeReadsModifiers = Yes
			r.Semantics = &sem
		},
	}}
	locales := []struct {
		name string
		vars map[string]string
		want string
	}{
		{"C narrows to ASCII", map[string]string{"LC_ALL": "C"}, "CAFé"},
		{"POSIX is the same narrowing", map[string]string{"LANG": "POSIX"}, "CAFé"},
		{"UTF-8 cases beyond it", map[string]string{"LC_ALL": "en_US.UTF-8"}, "CAFÉ"},
	}
	for _, site := range sites {
		for _, loc := range locales {
			t.Run(site.name+", "+loc.name, func(t *testing.T) {
				d := syntax.Core()
				if site.enable != nil {
					site.enable(&d)
				}
				f, err := syntax.Parse(site.src, d)
				if err != nil {
					t.Fatalf("parse %q: %v", site.src, err)
				}
				var buf bytes.Buffer
				sem := permissive()
				vars := map[string]string{}
				for k, v := range loc.vars {
					vars[k] = v
				}
				r := newTestRunner(t, &Runner{
					Stdout: &buf, Stderr: &buf, Semantics: &sem,
					Vars: vars, Env: []string{},
				})
				if site.before != nil {
					site.before(r)
				}
				if _, rerr := r.Run(context.Background(), f); rerr != nil {
					t.Fatal(rerr)
				}
				if got := strings.TrimSpace(buf.String()); got != loc.want {
					t.Errorf("%s: got %q, want %q", site.src, got, loc.want)
				}
			})
		}
	}
}

// The lower-case half, so the narrowing is not read as a property of the
// upper-case direction alone — `unicode.ToLower` is the other call each of
// these sites was making bare.
func TestLowerCaseNarrowsAtEverySiteToo(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the operator", `x=CAFÉ; echo "${x,,}"`, "cafÉ"},
		{"the attribute", `typeset -l x=CAFÉ; echo "$x"`, "cafÉ"},
		{"the modifier", `x=CAFÉ; echo "${x:l}"`, "cafÉ"},
		// No array row: measured under LC_ALL=C, `typeset -la a=(CAFÉ)` is
		// `CAFÉ` in zsh 5.9.2 and here, because whether the attribute reaches
		// an element at all is a separate unanswered axis. This is about
		// which case map applies once it does.
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := syntax.Core()
			d.ParamCaseChange = true
			f, err := syntax.Parse(tc.src, d)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			var buf bytes.Buffer
			sem := permissive()
			sem.SubstringRangeReadsModifiers = Yes
			sem.DeclareOptions = "aAilprux"
			r := newTestRunner(t, &Runner{
				Stdout: &buf, Stderr: &buf, Semantics: &sem,
				Vars: map[string]string{"LC_ALL": "C"}, Env: []string{},
			})
			if _, rerr := r.Run(context.Background(), f); rerr != nil {
				t.Fatal(rerr)
			}
			if got := strings.TrimSpace(buf.String()); got != tc.want {
				t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestEverySiteThatFoldsAMatchAsksTheSameLocale is #2644, and it is
// TestEverySiteThatChangesCaseAsksTheSameLocale's other half: the sites that
// treat two letters as one while *matching* narrow to ASCII in the C locale,
// exactly as the sites that convert a value do.
//
// One table over every folding surface rather than a test each, for the
// reason the converting half is one table — the defect was drift **between**
// the sites, and each site had a test that agreed with it. Here the two
// operators of `[[ ]]` had drifted in opposite directions, which is what made
// the pair unable to find each other: `=~` handed the fold to a regex engine
// whose case flag folds Unicode and has no locale to be told about, so it
// matched under `LC_ALL=C` where the panel does not, while `==` folded with
// an ASCII byte swap under every locale, so it missed under UTF-8 where the
// panel matches.
//
// Measured 2026-09-13 with `nocasematch` on, `ÉTÉ` against `été`: bash 5.3.15
// matches at both operators under `en_US.UTF-8` and at neither under
// `LC_ALL=C` or `LC_ALL=POSIX`. zsh 5.9.2 answers the same way at `=~`, which
// is the only one of the two its own option reaches (#2622). So the locale
// half is a correction and not an axis.
func TestEverySiteThatFoldsAMatchAsksTheSameLocale(t *testing.T) {
	sites := []struct {
		name   string
		src    string
		option MatchOption
		before func(*testing.T, *Runner)
	}{{
		name:   "the pattern operator",
		src:    `[[ ÉTÉ == été ]] && echo fold || echo exact`,
		option: MatchFoldsCase,
	}, {
		// One case in the bracket, so the row needs the fold: a member
		// holding both cases matches either way and would pass with the
		// fold gone.
		name:   "a bracket in the pattern",
		src:    `[[ ÉTÉ == [é]T[é] ]] && echo fold || echo exact`,
		option: MatchFoldsCase,
	}, {
		name:   "the case statement",
		src:    `case ÉTÉ in été) echo fold;; *) echo exact;; esac`,
		option: MatchFoldsCase,
	}, {
		name:   "the substitution operator",
		src:    `x=ÉTÉ; r=${x//été/fold}; [ "$r" = fold ] && echo fold || echo exact`,
		option: MatchFoldsCase,
	}, {
		name:   "the regex operator",
		src:    `[[ ÉTÉ =~ ^été$ ]] && echo fold || echo exact`,
		option: RegexFoldsCase,
	}, {
		name: "pathname expansion",
		// A metacharacter, because a word without one is not a pattern at
		// all and never reaches the matcher — the row would pass with the
		// fold gone. Measured on bash 5.3.15 with `nocaseglob` on and a file
		// named `ÉTÉ`: `echo été*` prints the file under `en_US.UTF-8` and
		// the unmatched word itself under `LC_ALL=C`.
		src:    `r=$(echo été*); [ "$r" = ÉTÉ ] && echo fold || echo exact`,
		option: GlobFoldsCase,
		before: func(t *testing.T, r *Runner) {
			t.Helper()
			dir := t.TempDir()
			// Written and read back under the name this test is about: a
			// file system that stores a different normalization than the one
			// it was given would make the row measure the file system, so it
			// is checked rather than assumed.
			if err := os.WriteFile(filepath.Join(dir, "ÉTÉ"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			names, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(names) != 1 || names[0].Name() != "ÉTÉ" {
				t.Skipf("the file system renamed %q, so a glob over it says nothing about the fold", "ÉTÉ")
			}
			r.Dir = dir
		},
	}}
	locales := []struct {
		name string
		vars map[string]string
		want string
	}{
		{"C narrows to ASCII", map[string]string{"LC_ALL": "C"}, "exact"},
		{"POSIX is the same narrowing", map[string]string{"LANG": "POSIX"}, "exact"},
		{"UTF-8 folds beyond it", map[string]string{"LC_ALL": "en_US.UTF-8"}, "fold"},
	}
	for _, site := range sites {
		for _, loc := range locales {
			t.Run(site.name+", "+loc.name, func(t *testing.T) {
				f, err := syntax.Parse(site.src, syntax.Core())
				if err != nil {
					t.Fatalf("parse %q: %v", site.src, err)
				}
				var buf bytes.Buffer
				sem := permissive()
				vars := map[string]string{}
				for k, v := range loc.vars {
					vars[k] = v
				}
				r := newTestRunner(t, &Runner{
					Stdout: &buf, Stderr: &buf, Semantics: &sem,
					Vars: vars, Env: []string{},
				})
				r.SetMatchOption(site.option, true)
				if site.before != nil {
					site.before(t, r)
				}
				if _, rerr := r.Run(context.Background(), f); rerr != nil {
					t.Fatal(rerr)
				}
				if got := strings.TrimSpace(buf.String()); got != loc.want {
					t.Errorf("%s: got %q, want %q", site.src, got, loc.want)
				}
			})
		}
	}
}

// The fold with the option **off** is exact at every one of those sites under
// every locale, so the rows above are evidence about the option and the
// locale together rather than about a matcher that had started folding on its
// own. Written out rather than inferred: a `==` that folded unconditionally
// passes the UTF-8 third of the table and nothing here would notice.
func TestNoSiteFoldsAMatchWithTheOptionOff(t *testing.T) {
	for _, src := range []string{
		`[[ ÉTÉ == été ]] && echo fold || echo exact`,
		`case ÉTÉ in été) echo fold;; *) echo exact;; esac`,
		`x=ÉTÉ; r=${x//été/fold}; [ "$r" = fold ] && echo fold || echo exact`,
		`[[ ÉTÉ =~ ^été$ ]] && echo fold || echo exact`,
	} {
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		var buf bytes.Buffer
		sem := permissive()
		r := newTestRunner(t, &Runner{
			Stdout: &buf, Stderr: &buf, Semantics: &sem,
			Vars: map[string]string{"LC_ALL": "en_US.UTF-8"}, Env: []string{},
		})
		if _, rerr := r.Run(context.Background(), f); rerr != nil {
			t.Fatal(rerr)
		}
		if got := strings.TrimSpace(buf.String()); got != "exact" {
			t.Errorf("%s: got %q with no option on, want %q", src, got, "exact")
		}
	}
}

// What `=~` captured is a span of the subject **as the script wrote it**,
// which the narrowing must not disturb: it matches against a rewritten
// subject under a C locale, so an offset the engine reported is not an offset
// into the script's own text until it has been mapped back. Measured on bash
// 5.3.15 under `LC_ALL=C` with `nocasematch` on — `BASH_REMATCH` holds the
// upper-case letters the subject held, not the pattern's.
func TestTheNarrowedRegexFoldStillCapturesTheSubjectsOwnText(t *testing.T) {
	const src = `[[ éABCé =~ (a)(b)(c) ]] && echo "[${BASH_REMATCH[0]}][${BASH_REMATCH[1]}][${BASH_REMATCH[3]}]"`
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	// A named record raises two questions of its own; this test is about
	// the *text* the captures hold, so both are answered the dense way.
	sem.RegexMatchSurvivesAFailedMatch = No
	sem.RegexMatchOmitsGroupsThatDidNotMatch = No
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem,
		Vars: map[string]string{"LC_ALL": "C"}, Env: []string{},
	})
	r.SetMatchOption(RegexFoldsCase, true)
	r.SetRegexMatch("BASH_REMATCH")
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatal(rerr)
	}
	if got := strings.TrimSpace(buf.String()); got != "[ABC][A][C]" {
		t.Errorf("%s: got %q, want %q", src, got, "[ABC][A][C]")
	}
}

// A wide fold is the lower-case map applied to both sides, not a swap applied
// to one — which is what the characters whose case mapping is not a pair
// measure, and the only cells that can tell the two readings apart.
//
// Measured 2026-09-13 on bash 5.3.15 under `en_US.UTF-8` with `nocasematch`
// on. Both directions of each pair, because a swap of one side is not
// symmetric and a table testing one direction would not notice.
func TestAWideFoldIsTheLowerCaseMapAndNotASwap(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// U+212A KELVIN SIGN lower-cases to the ASCII `k`, so it folds with
		// both cases of it. A swap through ToUpper would leave it alone.
		{"the Kelvin sign against k", "[[ \u212A == k ]] && echo fold || echo exact", "fold"},
		{"k against the Kelvin sign", "[[ k == \u212A ]] && echo fold || echo exact", "fold"},
		// U+017F LATIN SMALL LETTER LONG S is already lower case, so nothing
		// lower-cases onto it. A swap through ToUpper would send it to `S`
		// and match, which bash does not.
		{"the long s against s", "[[ \u017F == s ]] && echo fold || echo exact", "exact"},
		{"s against the long s", "[[ s == \u017F ]] && echo fold || echo exact", "exact"},
		// A range folds and folds wide; the two ends are ranked by code
		// point, so what has to land inside is the *lower case*.
		{"the Kelvin sign in an ASCII range", "[[ \u212A == [a-z] ]] && echo fold || echo exact", "fold"},
		{"É in an accented range", `[[ É == [à-ÿ] ]] && echo fold || echo exact`, "fold"},
		{"É in an ASCII range", `[[ É == [a-z] ]] && echo fold || echo exact`, "exact"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			var buf bytes.Buffer
			sem := permissive()
			r := newTestRunner(t, &Runner{
				Stdout: &buf, Stderr: &buf, Semantics: &sem,
				Vars: map[string]string{"LC_ALL": "en_US.UTF-8"}, Env: []string{},
			})
			r.SetMatchOption(MatchFoldsCase, true)
			if _, rerr := r.Run(context.Background(), f); rerr != nil {
				t.Fatal(rerr)
			}
			if got := strings.TrimSpace(buf.String()); got != tc.want {
				t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// The narrowed fold writes private-use characters in place of the ones the
// engine must not fold, and a subject that already holds one of those must
// not be caught by a stand-in handed out for a different character.
//
// The block is skipped rather than counted through for that reason, and this
// is the row that says so: with the first character of the block sitting in
// the subject, `é` must still not match `É`, and the block character must
// still match only itself.
func TestANarrowedRegexFoldDoesNotCollideWithAPrivateUseSubject(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The subject is the first character of the block and nothing else,
		// so a stand-in handed out by counting from the start of the block
		// would be exactly it — and `é` would match a character it has
		// nothing to do with.
		{"a stand-in is not a character the subject holds", "[[ \U000F0000 =~ ^é$ ]] && echo hit || echo miss", "miss"},
		// And the block character still matches itself, so the skipping is
		// not simply refusing to rewrite anything.
		{"the block character still matches itself", "[[ \U000F0000 =~ ^\U000F0000$ ]] && echo hit || echo miss", "hit"},
		{"and the narrowing still holds beside it", "[[ \U000F0000É =~ ^\U000F0000é$ ]] && echo fold || echo exact", "exact"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			var buf bytes.Buffer
			sem := permissive()
			r := newTestRunner(t, &Runner{
				Stdout: &buf, Stderr: &buf, Semantics: &sem,
				Vars: map[string]string{"LC_ALL": "C"}, Env: []string{},
			})
			r.SetMatchOption(RegexFoldsCase, true)
			if _, rerr := r.Run(context.Background(), f); rerr != nil {
				t.Fatal(rerr)
			}
			if got := strings.TrimSpace(buf.String()); got != tc.want {
				t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
