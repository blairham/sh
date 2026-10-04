// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// A `[[ … ]]` pattern is traced as the pattern this shell reads it as, and a
// `case` arm's patterns are tried from the last. Measured 2026-10-04 on
// ksh93u+ 2012-08-01 under `set -x` (#5722). See
// interp.Diagnostics.TraceConditionAsKsh93 and
// interp.Semantics.CaseTriesTheLastAlternativeFirst.
func TestConditionsAndCaseArmsTraceAsKsh93Does(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x="a b"; [[ $x == "a b" ]]`, "+ [[ 'a b' == 'a b' ]]"},
		{`[[ abc = "a b" ]]`, "+ [[ abc == 'a b' ]]"},
		{`[[ abc == "a*" ]]`, "+ [[ abc == 'a*' ]]"},
		{`[[ abc == a\ b ]]`, "+ [[ abc == 'a b' ]]"},
		{`[[ abc == "" ]]`, "+ [[ abc == '' ]]"},
		// Part quoted and all text, so still the value.
		{`[[ abc == a" "b ]]`, "+ [[ abc == 'a b' ]]"},
		{`[[ abc == a"*" ]]`, "+ [[ abc == 'a*' ]]"},
		{"[[ abc == a'|'b ]]", "+ [[ abc == 'a|b' ]]"},
		{`[[ abc == a"*"b* ]]`, `+ [[ abc == a\*b* ]]`},
		{`p="a b"; [[ abc == "$p" ]]`, `+ [[ abc == a\ b ]]`},
		{`p="a*"; [[ abc == $p ]]`, "+ [[ abc == a* ]]"},
		{`[[ abc == @(a|b) ]]`, "+ [[ abc == @(a|b) ]]"},
		{`[[ abc =~ ^a.c$ ]]`, `+ [[ abc == ~(E)^a.c\$ ]]`},
		{`[[ abc =~ "a b" ]]`, `+ [[ abc == ~(E)a\ b ]]`},
		{`[[ abc =~ a|b ]]`, "+ [[ abc == ~(E)a|b ]]"},
		{`[[ abc =~ "a|b" ]]`, `+ [[ abc == ~(E)a\|b ]]`},
		{`[[ abc =~ 'a.c' ]]`, "+ [[ abc == ~(E)a.c ]]"},
		{`case a in $(echo a)|$(echo b)) : ;; esac`, "+ echo b\n+ echo a\n+ :"},
		{`case a in $(echo b)|$(echo a)) : ;; esac`, "+ echo a\n+ :"},
	} {
		_, errs, _ := runKshArgs(t, "-c", "set -x; "+c.src)
		var got []string
		for _, l := range strings.Split(strings.TrimSuffix(errs, "\n"), "\n") {
			if !strings.HasPrefix(l, "+ p=") && !strings.HasPrefix(l, "+ x=") {
				got = append(got, l)
			}
		}
		if g := strings.Join(got, "\n"); g != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.src, g, c.want)
		}
	}
}
