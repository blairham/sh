// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package diff renders a unified diff between two texts, for the formatter's
// -d mode. Line-based LCS; scripts are small and the exactness matters more
// than the constant.
package diff

import (
	"fmt"
	"strings"
)

// Unified returns a unified diff of a against b with three lines of context,
// headed by the given names. Empty means no difference.
func Unified(nameA, nameB, a, b string) string {
	if a == b {
		return ""
	}
	al, bl := splitLines(a), splitLines(b)
	ops := diffOps(al, bl)
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", nameA, nameB)
	const ctx = 3
	for i := 0; i < len(ops); {
		if ops[i].kind == keep {
			i++
			continue
		}
		// A hunk: from ctx lines before this change through ctx lines after
		// the last change reachable without a gap wider than 2*ctx.
		start := i
		end := i
		for j := i + 1; j < len(ops); j++ {
			if ops[j].kind != keep {
				gap := 0
				for k := end + 1; k < j; k++ {
					gap++
				}
				if gap > 2*ctx {
					break
				}
				end = j
			}
		}
		from := max(0, start-ctx)
		to := min(len(ops), end+1+ctx)
		var aStart, bStart, aCount, bCount int
		aStart, bStart = ops[from].aLine, ops[from].bLine
		var body strings.Builder
		for _, op := range ops[from:to] {
			switch op.kind {
			case keep:
				body.WriteString(" " + op.text + "\n")
				aCount++
				bCount++
			case del:
				body.WriteString("-" + op.text + "\n")
				aCount++
			case ins:
				body.WriteString("+" + op.text + "\n")
				bCount++
			}
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", aStart+1, aCount, bStart+1, bCount)
		out.WriteString(body.String())
		i = to
	}
	return out.String()
}

type kind int

const (
	keep kind = iota
	del
	ins
)

type op struct {
	kind         kind
	text         string
	aLine, bLine int
}

func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// diffOps is the textbook LCS walk: quadratic table, then a backtrack that
// yields keeps, deletions and insertions in order.
func diffOps(a, b []string) []op {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, op{keep, a[i], i, j})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, op{del, a[i], i, j})
			i++
		default:
			ops = append(ops, op{ins, b[j], i, j})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{del, a[i], i, j})
	}
	for ; j < m; j++ {
		ops = append(ops, op{ins, b[j], i, j})
	}
	return ops
}
