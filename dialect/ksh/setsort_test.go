// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `set -s` sorts, measured 2026-09-16 on ksh93 93u+ 2012-08-01 under
// LC_ALL=C: the operands the call is given, the positional parameters already
// there when it is given none, and a `set -A` or `set +A`'s values. `+s` is
// the same request and `$-` never shows the letter. It was refused as not
// implemented before, which ended every one of these lines at status 2.
func TestKshSetSSortsTheOperands(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`set -- c b a; set -s; echo "$*"`, "a b c\n"},
		{`set -s c b a; echo "$*"`, "a b c\n"},
		{`set -s -- c -b a; echo "$*"`, "-b a c\n"},
		{`set -- B a 10 9 ''; set -s; printf '[%s]' "$@"; echo`, "[][10][9][B][a]\n"},
		{`set -- c b a; set +s; echo "$*"`, "a b c\n"},
		{`set -- c b a; set -s; case $- in *s*) echo s;; *) echo none;; esac`, "none\n"},
		{`set -- q p; set -sA a z y; echo "${a[*]} / $*"`, "y z / q p\n"},
		{`set -s -A a q p; echo "${a[*]}"`, "p q\n"},
		{`a=(z 2); set -s +A a b a; echo "${a[*]}"`, "a b\n"},
		{`f() { set -s; echo "$*"; }; set -- x; f c b a; echo "$*"`, "a b c\nx\n"},
		// The letter is a request about one call: the next `set` does not
		// inherit it.
		{`set -s b a; set -- d c; echo "$*"`, "d c\n"},
	} {
		var out, errs strings.Builder
		r := preset.Runner(dialecttest.Base{Stdout: &out, Stderr: &errs, Name: "/bin/ksh"})
		if _, err := r.Run(context.Background(), preset.Parse(t, c.src+"\n")); err != nil {
			t.Fatalf("run %q: %v", c.src, err)
		}
		if out.String() != c.want || errs.Len() != 0 {
			t.Errorf("%s\n got %q, stderr %q\nwant %q", c.src, out.String(), errs.String(), c.want)
		}
	}
}
