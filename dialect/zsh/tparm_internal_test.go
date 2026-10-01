// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// tparm over the descriptions `echoti` reads, each row what zsh 5.9.2's
// `echoti` wrote under that TERM (#5150). The inputs are the terminals' own
// capability strings, as `infocmp -1` prints them.
func TestTparmComputesTheMeasuredSequences(t *testing.T) {
	const (
		setaf256 = "\x1b[%?%p1%{8}%<%t3%p1%d%e%p1%{16}%<%t9%p1%{8}%-%d%e38;5;%p1%d%;m"
		setab256 = "\x1b[%?%p1%{8}%<%t4%p1%d%e%p1%{16}%<%t10%p1%{8}%-%d%e48;5;%p1%d%;m"
		cup      = "\x1b[%i%p1%d;%p2%dH"
		sgrXterm = "%?%p9%t\x1b(0%e\x1b(B%;\x1b[0%?%p6%t;1%;%?%p5%t;2%;%?%p2%t;4%;%?%p1%p3%|%t;7%;%?%p4%t;5%;%?%p7%t;8%;m"
		sgrVT100 = "\x1b[0%?%p1%p6%|%t;1%;%?%p2%t;4%;%?%p1%p3%|%t;7%;%?%p4%t;5%;m%?%p9%t\x0e%e\x0f%;"
	)
	for _, c := range []struct {
		name, cap string
		params    []int
		want      string
	}{
		{"setaf 2, the first branch", setaf256, []int{2}, "\x1b[32m"},
		{"setaf 9, the second", setaf256, []int{9}, "\x1b[91m"},
		{"setaf 200, the third", setaf256, []int{200}, "\x1b[38;5;200m"},
		{"setab 12", setab256, []int{12}, "\x1b[104m"},
		{"setaf -1", "\x1b[3%p1%dm", []int{-1}, "\x1b[3-1m"},
		{"cup 3 4", cup, []int{3, 4}, "\x1b[4;5H"},
		{"cup 3, a missing parameter is 0", cup, []int{3}, "\x1b[4;1H"},
		{"cup 1 2 3, an extra one is ignored", cup, []int{1, 2, 3}, "\x1b[2;3H"},
		{"sgr standout", sgrXterm, []int{1, 0, 0, 0, 0, 0, 0, 0, 0}, "\x1b(B\x1b[0;7m"},
		{"sgr reverse and blink", sgrXterm, []int{0, 0, 1, 1, 0, 0, 0, 0, 0}, "\x1b(B\x1b[0;7;5m"},
		{"vt100 sgr standout", sgrVT100, []int{1, 0, 0, 0, 0, 0, 0, 0, 0}, "\x1b[0;1;7m\x0f"},
		{"no parameter codes at all", "\x1b[A", []int{3}, "\x1b[A"},
		// The rows below are the language as terminfo(5) describes it rather
		// than sequences measured through a terminal: no description on the
		// measuring machine uses these codes.
		{"a percent sign", "%%%p1%d", []int{7}, "%7"},
		{"a character constant and %c", "%'A'%c", nil, "A"},
		{"arithmetic, right operand popped first", "%p1%p2%-%d", []int{9, 4}, "5"},
		{"a formatted width", "%p1%:-3d|", []int{5}, "5  |"},
		{"variables", "%p1%Pa%ga%ga%+%d", []int{6}, "12"},
		{"a skipped branch holding a conditional of its own", "%?%p1%t%?%p2%tA%eB%;%eC%;", []int{0, 1}, "C"},
	} {
		if got := tparm(c.cap, c.params); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// Padding is a delay for the writer and never reaches the terminal.
func TestWithoutPadding(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b[4;5H$<5>":  "\x1b[4;5H",
		"\x0f$<2>":       "\x0f",
		"a$<10.5*/>b":    "ab",
		"a$<x>b":         "a$<x>b",
		"cost $5 and $<": "cost $5 and $<",
	} {
		if got := withoutPadding(in); got != want {
			t.Errorf("withoutPadding(%q) = %q, want %q", in, got, want)
		}
	}
}
