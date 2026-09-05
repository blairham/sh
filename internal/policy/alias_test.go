// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package, because the table is per platform and the logic is not: a test
// that could only use the platform's own table would assert nothing at all on a
// system that has none, and would be a test that passes by being empty on
// exactly half the machines it runs on.
package policy

import (
	"runtime"
	"slices"
	"strings"
	"testing"
)

// testAliases stands in for a platform's table, with macOS's shape and a
// neighbor that must not be touched.
var testAliases = []alias{
	{from: "/tmp", to: "/private/tmp"},
	{from: "/private/tmp", to: "/tmp"},
	{from: "/var", to: "/private/var"},
	{from: "/private/var", to: "/var"},
}

// A pattern under a two-name place gains the other name, and one that is not
// gains nothing.
//
// The neighbor cases are the ones worth writing down. `/tmpfoo` starts with
// `/tmp` and is a different directory, and a rule about it that quietly grew a
// second pattern would be a rule covering somewhere nobody named — which is the
// opposite failure from the one this closes, and a worse one, since it refuses
// what a person meant to allow.
func TestExpandAliasTakesWholeComponents(t *testing.T) {
	for _, tc := range []struct{ name, pattern, want string }{
		{"the directory itself", "/tmp", "/private/tmp"},
		{"everything under it", "/tmp/**", "/private/tmp/**"},
		{"one file", "/tmp/secrets/key", "/private/tmp/secrets/key"},
		{"a glob in it", "/tmp/*.key", "/private/tmp/*.key"},
		{"the other direction", "/private/tmp/**", "/tmp/**"},
		{"another entry", "/var/log/**", "/private/var/log/**"},
		{"a neighbor whose name starts the same way", "/tmpfoo/**", ""},
		{"a neighbor of the target", "/private/tmpfoo", ""},
		{"somewhere else entirely", "/srv/build/**", ""},
		{"the root", "/", ""},
		{"a name that only contains it", "/opt/tmp/**", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := expandAlias(tc.pattern, testAliases); got != tc.want {
				t.Errorf("expandAlias(%q) = %q, want %q", tc.pattern, got, tc.want)
			}
		})
	}
}

// An empty table expands nothing, which is what every system but one has.
func TestExpandAliasWithNoTableExpandsNothing(t *testing.T) {
	for _, p := range []string{"/tmp/**", "/private/tmp/**", "/var"} {
		if got := expandAlias(p, nil); got != "" {
			t.Errorf("expandAlias(%q) with no table = %q, want nothing", p, got)
		}
	}
}

// Expansion is one hop and never chains.
//
// The table has both directions in it, so a second pass over the result would
// walk straight back to where it started. Nothing is gained by chaining — a
// table of two-name places has no chains — and looping is the only thing it
// could do.
func TestExpandAliasIsOneHop(t *testing.T) {
	first := expandAlias("/tmp/x", testAliases)
	if first != "/private/tmp/x" {
		t.Fatalf("first hop = %q", first)
	}
	if back := expandAlias(first, testAliases); back != "/tmp/x" {
		t.Fatalf("the table is not symmetric: %q went to %q", first, back)
	}
	// Which is exactly why the parser applies it once. Applying it again is
	// what this test exists to make visible, not something the parser does.
}

// The platform's own table is a set of two-name places, in both directions.
//
// A one-way entry would close half the hole and leave the other half looking
// closed, which is the state this whole change is about.
func TestThePlatformTableIsSymmetric(t *testing.T) {
	for _, a := range platformAliases {
		if !slices.ContainsFunc(platformAliases, func(b alias) bool {
			return b.from == a.to && b.to == a.from
		}) {
			t.Errorf("%q -> %q has no entry going back", a.from, a.to)
		}
		if !strings.HasPrefix(a.from, "/") || !strings.HasPrefix(a.to, "/") {
			t.Errorf("%q -> %q: both names must be absolute", a.from, a.to)
		}
		if a.from == a.to {
			t.Errorf("%q aliases itself", a.from)
		}
	}
}

// macOS has the three the operating system installs, and nowhere else does.
//
// Written as a platform assertion rather than skipped, because "the table is
// empty" is the claim being made everywhere else and a table that quietly grew
// an entry on Linux would make one policy file mean two things.
func TestThePlatformTableIsTheOnesTheSystemShips(t *testing.T) {
	if runtime.GOOS != "darwin" {
		if len(platformAliases) != 0 {
			t.Errorf("%s has %d aliases, want none: a distribution's arrangement is not a platform's",
				runtime.GOOS, len(platformAliases))
		}
		return
	}
	for _, want := range []string{"/tmp", "/var", "/etc"} {
		if expandAlias(want, platformAliases) != "/private"+want {
			t.Errorf("%s does not expand to /private%s", want, want)
		}
	}
	// And nothing beyond them. The bar is "unconditional on this platform",
	// and an entry that is merely usual would make a policy machine-dependent.
	if len(platformAliases) != 6 {
		t.Errorf("the table holds %d entries, want the three places in both directions",
			len(platformAliases))
	}
}
