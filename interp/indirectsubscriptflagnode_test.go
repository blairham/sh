// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// The node the rewrite hands on, asserted where the rewrite operates.
//
// The behavioral rows are in indirectsubscriptflag_test.go and they are the
// ones that matter; this is the half of the rewrite they cannot see. Dropping
// `Indirect` is **measured equivalent** in the one dialect that answers the
// axis Yes today — the twenty-two rows there agree either way, and so does
// every shape the reference's `emulate ksh` can be asked, because that mode
// has no name references for an indirection path to reach (`typeset -n` is
// `bad option: -n` there). It is written all the same, because a node carrying
// both readings claims to be two things at once and the next reader of
// `ParamExpr.Indirect` would believe it.
func TestTheRewrittenNodeIsTheFlagAndNotAnIndirection(t *testing.T) {
	t.Parallel()
	for _, src := range []string{`${!v}`, `${!a[0]}`, `${!a[@]}`} {
		t.Run(src, func(t *testing.T) {
			d := syntax.Core()
			d.ParamIndirection = true
			d.ArraySubscript = true
			f, err := syntax.Parse("echo \""+src+"\"\n", d)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			span := f.Stmts[0].Expr.(*syntax.Pipeline).
				Cmds[0].(*syntax.SimpleCmd).Args[1].Spans[0]
			if span.Param == nil || !span.Param.Indirect {
				t.Fatalf("the source did not parse to an indirection: %+v", span.Param)
			}
			sem := Semantics{IndirectionIsTheSubscriptFlag: Yes}
			// testrunner:bare — the subject is one node and nothing runs.
			r := &Runner{Semantics: &sem, Dialect: &d}

			got := r.indirectionReadAsTheSubscriptFlag(span)
			switch {
			case got.Param.Indirect:
				t.Error("the rewritten node is still an indirection")
			case !got.Param.HasFlags || got.Param.Flags != "k":
				t.Errorf("flags %q hasFlags %v, want the `k` flag",
					got.Param.Flags, got.Param.HasFlags)
			}
			// And the node in the **tree** is untouched, which is the same
			// claim the one-tree-two-answers row makes from outside.
			if !span.Param.Indirect || span.Param.HasFlags {
				t.Errorf("the tree's node was written through: %+v", span.Param)
			}
			// With the axis the other way nothing is rewritten at all.
			sem.IndirectionIsTheSubscriptFlag = No
			if got := r.indirectionReadAsTheSubscriptFlag(span); got.Param != span.Param {
				t.Error("the axis answered No and the node was rewritten anyway")
			}
		})
	}
}
