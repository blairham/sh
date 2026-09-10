// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package walk visits every node of a github.com/blairham/sh syntax tree.
//
// The substrate exports no walker of its own, and two packages here need one
// — comment recovery and the printer's here-document scan — so it lives once,
// in this package, rather than as a helper in each.
package walk

import "github.com/blairham/sh/syntax"

// Nodes calls fn for n and then for every node beneath it, parents before
// children, in source order. Returning false skips the node's children.
//
// Condition expressions ([[ ]] interiors) and arithmetic trees are not
// descended: every consumer here treats their whole extent as opaque.
func Nodes(n syntax.Node, fn func(syntax.Node) bool) {
	if n == nil || !fn(n) {
		return
	}
	switch x := n.(type) {
	case *syntax.File:
		stmts(x.Stmts, fn)
	case *syntax.Stmt:
		Nodes(x.Expr, fn)
	case *syntax.BinaryExpr:
		Nodes(x.X, fn)
		Nodes(x.Y, fn)
	case *syntax.Pipeline:
		for _, c := range x.Cmds {
			Nodes(c, fn)
		}
	case *syntax.TimeClause:
		if x.Pipeline != nil {
			Nodes(x.Pipeline, fn)
		}
	case *syntax.SimpleCmd:
		for _, a := range x.Assigns {
			Nodes(a, fn)
		}
		for _, w := range x.Args {
			Nodes(w, fn)
		}
		redirects(x.Redirs, fn)
	case *syntax.Subshell:
		stmts(x.List, fn)
		redirects(x.Redirs, fn)
	case *syntax.Group:
		stmts(x.List, fn)
		redirects(x.Redirs, fn)
	case *syntax.TryClause:
		stmts(x.Try, fn)
		stmts(x.Always, fn)
		redirects(x.Redirs, fn)
	case *syntax.IfClause:
		stmts(x.Cond, fn)
		stmts(x.Then, fn)
		for _, e := range x.Elifs {
			Nodes(e, fn)
		}
		stmts(x.Else, fn)
		redirects(x.Redirs, fn)
	case *syntax.Elif:
		stmts(x.Cond, fn)
		stmts(x.Then, fn)
	case *syntax.LoopClause:
		stmts(x.Cond, fn)
		stmts(x.Body, fn)
		redirects(x.Redirs, fn)
	case *syntax.ForClause:
		words(x.Items, fn)
		stmts(x.Body, fn)
		redirects(x.Redirs, fn)
	case *syntax.SelectClause:
		words(x.Items, fn)
		stmts(x.Body, fn)
		redirects(x.Redirs, fn)
	case *syntax.AnonFunc:
		Nodes(x.Body, fn)
		words(x.Args, fn)
		redirects(x.Redirs, fn)
	case *syntax.RepeatClause:
		word(x.Count, fn)
		stmts(x.Body, fn)
		redirects(x.Redirs, fn)
	case *syntax.CoprocClause:
		if x.Cmd != nil {
			Nodes(x.Cmd, fn)
		}
	case *syntax.CaseClause:
		word(x.Word, fn)
		for _, it := range x.Items {
			Nodes(it, fn)
		}
		redirects(x.Redirs, fn)
	case *syntax.CaseItem:
		words(x.Patterns, fn)
		stmts(x.Body, fn)
	case *syntax.ForArithClause:
		stmts(x.Body, fn)
		redirects(x.Redirs, fn)
	case *syntax.ArithCmdClause:
		redirects(x.Redirs, fn)
	case *syntax.TestClause:
		redirects(x.Redirs, fn)
	case *syntax.FuncDecl:
		word(x.NameWord, fn)
		for _, n := range x.AlsoNamed {
			word(n.Word, fn)
		}
		if x.Body != nil {
			Nodes(x.Body, fn)
		}
	case *syntax.Assign:
		word(x.Value, fn)
		word(x.Index, fn)
		words(x.Elems, fn)
	case *syntax.Redirect:
		word(x.N, fn)
		word(x.Word, fn)
		word(x.Heredoc, fn)
	case *syntax.Word:
		// A leaf for our purposes: spans carry no further extents.
	}
}

func stmts(list []*syntax.Stmt, fn func(syntax.Node) bool) {
	for _, s := range list {
		Nodes(s, fn)
	}
}

func words(list []*syntax.Word, fn func(syntax.Node) bool) {
	for _, w := range list {
		Nodes(w, fn)
	}
}

// word guards the typed-nil hazard: a nil *Word in a Node interface is not a
// nil interface, so the check at the top of Nodes cannot catch it.
func word(w *syntax.Word, fn func(syntax.Node) bool) {
	if w != nil {
		Nodes(w, fn)
	}
}

func redirects(list []*syntax.Redirect, fn func(syntax.Node) bool) {
	for _, r := range list {
		Nodes(r, fn)
	}
}
