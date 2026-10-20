// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
)

// A dotted name reads here, which is what makes `${.sh.version}` writable at
// all — and what made every script carrying one unrunnable before, since the
// refusal was the *parse*'s and took the whole file with it (#2620).
//
// Measured on ksh93u+ 2012-08-01, 2026-09-13, `-c`. The values are ours where
// a value is ours to give; the shapes are the shell's.
func TestADottedNameReads(t *testing.T) {
	for _, c := range []struct {
		src, want string
		status    int
	}{
		// The namespace, which is the row the issue is named for. The value
		// is `$KSH_VERSION` under the name this shell's own namespace gives
		// it — measured on ksh93u+, the two are the same sentence there —
		// and the shape is asserted rather than the text so that the version
		// is written down in one place. The prelude that sets `KSH_VERSION`
		// is not read by this harness, which is why the two are not simply
		// compared.
		{`case "${.sh.version}" in Version\ *) echo version;; esac`, "version\n", 0},
		// An unset dotted name is the empty string at status 0, exactly as
		// an unset plain one is. `${.sh.pid}` and its nine siblings are all
		// this row on the measured build.
		{`printf "[%s]\n" "${.sh.pid}"`, "[]\n", 0},
		{`printf "[%s]\n" "${.foo}"`, "[]\n", 0},
		{`printf "[%s]\n" "${.}"`, "[]\n", 0},
		{`x=1; printf "[%s]\n" "${x.y}"`, "[]\n", 0},
		// An assignment, which is the other half of the same lexical rule.
		{`.foo=1; echo "${.foo}"`, "1\n", 0},
		{`a=1; a.b=2; echo "${a.b}"`, "2\n", 0},
		// The name positions that are neither an expansion nor a bare
		// assignment. Each was `invalid variable name` before.
		{`typeset .x=3; echo "${.x}"`, "3\n", 0},
		{`for .x in 1 2; do echo "${.x}"; done`, "1\n2\n", 0},
		{`.foo=1; unset .foo; printf "[%s]\n" "${.foo}"`, "[]\n", 0},
		{`.foo=1; readonly .foo; echo ok`, "ok\n", 0},
		{`printf "x\n" | read .y; echo "${.y}"`, "x\n", 0},
		// The one builtin that refuses it, which is measured and is why the
		// rule is a set of builtins rather than one answer for declarations.
		{`.foo=1; export .foo`, "ksh: export: .foo: is not an identifier\n", 1},
		// An operator still reads off the end of a dotted name, so the name
		// scan stops where a name stops rather than swallowing the rest.
		{`case "${.sh.version#Version }" in Version*) echo kept;; ?*) echo trimmed;; esac`, "trimmed\n", 0},
		{`.foo=abc; echo "${#.foo}"`, "3\n", 0},
		{`.foo=abc; echo "${.foo:-x}"`, "abc\n", 0},
		// A `$` in front of a bare dotted name is *not* an expansion, which
		// is measured: the dot is a rule about names and not about what may
		// follow a dollar.
		{`.foo=1; echo $.foo`, "$.foo\n", 0},
	} {
		out, st := kshOut(t, c.src)
		if out != c.want || st != c.status {
			t.Errorf("%s\n got %q at %d\nwant %q at %d", c.src, out, st, c.want, c.status)
		}
	}
}

// And the flag does not leak. bash and zsh call `${.sh.version}` a bad
// substitution at the *run* — measured on bash 5.3.15, that binary as `sh`,
// bash 3.2.57, zsh 5.9.2 and BusyBox ash, every one of which runs the command
// before it and then complains — so a dialect that started parsing the dot
// would be accepting what its own shell refuses.
func TestADottedNameIsKshsAlone(t *testing.T) {
	for _, d := range []struct {
		name   string
		dotted bool
	}{
		{"ksh", ksh.Dialect().DottedName},
		{"bash", bash.Dialect().DottedName},
		{"zsh", zsh.Dialect().DottedName},
	} {
		if d.name == "ksh" && !d.dotted {
			t.Errorf("ksh: DottedName is off, want it on")
		}
		if d.name != "ksh" && d.dotted {
			t.Errorf("%s: DottedName is on, want it off", d.name)
		}
	}
}
