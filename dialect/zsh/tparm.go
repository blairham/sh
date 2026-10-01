// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"fmt"
	"strconv"
	"strings"
)

// tparm computes a terminfo capability's parameterized string, which is what
// `echoti NAME ARGS…` writes.
//
// The language is the one terminfo(5) documents under "Parameterized
// Strings", implemented from that page: a stack machine over integers, with
//
//	%%            a percent sign
//	%p1 … %p9     push a parameter          %{nn}  push an integer
//	%'c'          push a character's code   %l     the length of a string
//	%Pa %ga       set and get a variable (a–z per call, A–Z per call here)
//	%+ %- %* %/ %m   arithmetic, popping the right operand first
//	%& %| %^      bitwise      %= %> %<  comparison     %A %O  logical
//	%! %~         logical and bitwise not
//	%i            add one to the first two parameters
//	%? c %t then %e else %;   the conditional, with %e … %t chains
//	%d %o %x %X %s %c, and %[:flags][width[.precision]] before d o x X s
//
// Measured 2026-10-01 against zsh 5.9.2's `echoti` under TERM=xterm-256color,
// xterm, screen and vt100: `setaf` across the three branches of the
// 256-colour conditional (2, 9, 200), `setab`, `cup` with two, one and three
// arguments, `cuu`, `cub`, `ech`, `csr`, `hpa`, and `sgr` with nine
// parameters, byte for byte. A missing parameter is 0 and an extra one is
// ignored, measured (`cup 3` is `\e[4;1H`); a word that is not a number is 0
// (`setaf x` is `\e[30m`) and a negative one is written with its sign.
func tparm(s string, params []int) string {
	var p [9]int
	copy(p[:], params)
	var stack []int
	push := func(v int) { stack = append(stack, v) }
	pop := func() int {
		if len(stack) == 0 {
			return 0
		}
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return v
	}
	var dynamic, static [26]int
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch c := s[i]; c {
		case '%':
			b.WriteByte('%')
		case 'c':
			b.WriteByte(byte(pop()))
		case 'p':
			if i+1 < len(s) && s[i+1] >= '1' && s[i+1] <= '9' {
				i++
				push(p[s[i]-'1'])
			}
		case 'P', 'g':
			if i+1 >= len(s) {
				continue
			}
			i++
			v := s[i]
			switch {
			case v >= 'a' && v <= 'z' && c == 'P':
				dynamic[v-'a'] = pop()
			case v >= 'a' && v <= 'z':
				push(dynamic[v-'a'])
			case v >= 'A' && v <= 'Z' && c == 'P':
				static[v-'A'] = pop()
			case v >= 'A' && v <= 'Z':
				push(static[v-'A'])
			}
		case '\'':
			if i+2 < len(s) && s[i+2] == '\'' {
				push(int(s[i+1]))
				i += 2
			}
		case '{':
			j := strings.IndexByte(s[i:], '}')
			if j < 0 {
				continue
			}
			n, _ := strconv.Atoi(s[i+1 : i+j])
			push(n)
			i += j
		case 'l':
			// The length of a string parameter. Every parameter here is a
			// number, so what is measured is the digits it is written in.
			push(len(strconv.Itoa(pop())))
		case '+', '-', '*', '/', 'm', '&', '|', '^', '=', '>', '<', 'A', 'O':
			y, x := pop(), pop()
			push(tparmBinary(c, x, y))
		case '!':
			push(boolInt(pop() == 0))
		case '~':
			push(^pop())
		case 'i':
			p[0]++
			p[1]++
		case '?', ';':
			// The conditional's opening and closing marks do nothing on
			// their own: `%t` is where the test is taken.
		case 't':
			if pop() == 0 {
				i = tparmSkip(s, i, true)
			}
		case 'e':
			// Reached only after a branch that was taken: skip to the end.
			i = tparmSkip(s, i, false)
		default:
			if j, ok := tparmFormat(s, i, pop, &b); ok {
				i = j
			}
		}
	}
	return b.String()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func tparmBinary(op byte, x, y int) int {
	switch op {
	case '+':
		return x + y
	case '-':
		return x - y
	case '*':
		return x * y
	case '/':
		if y == 0 {
			return 0
		}
		return x / y
	case 'm':
		if y == 0 {
			return 0
		}
		return x % y
	case '&':
		return x & y
	case '|':
		return x | y
	case '^':
		return x ^ y
	case '=':
		return boolInt(x == y)
	case '>':
		return boolInt(x > y)
	case '<':
		return boolInt(x < y)
	case 'A':
		return boolInt(x != 0 && y != 0)
	case 'O':
		return boolInt(x != 0 || y != 0)
	}
	return 0
}

// tparmSkip moves past a branch not taken. From a false `%t` it stops at the
// matching `%e` (so the else part runs, and a chained `%e … %t` tests again)
// or at the matching `%;`; from an `%e` it stops only at the `%;`. It answers
// the index of the code's letter it stopped on.
func tparmSkip(s string, i int, toElse bool) int {
	depth := 0
	for i++; i < len(s); i++ {
		if s[i] != '%' || i+1 >= len(s) {
			continue
		}
		i++
		switch s[i] {
		case '?':
			depth++
		case ';':
			if depth == 0 {
				return i
			}
			depth--
		case 'e':
			if depth == 0 && toElse {
				return i
			}
		}
	}
	return i
}

// tparmFormat writes `%[:flags][width[.precision]]` followed by d, o, x, X or
// s, popping the value it formats, and answers where the code ended.
func tparmFormat(s string, i int, pop func() int, b *strings.Builder) (int, bool) {
	j := i
	if s[j] == ':' {
		j++
	}
	spec := "%"
	for j < len(s) && strings.IndexByte("-+# ", s[j]) >= 0 {
		spec += string(s[j])
		j++
	}
	for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.') {
		spec += string(s[j])
		j++
	}
	if j >= len(s) || strings.IndexByte("doxXs", s[j]) < 0 {
		return i, false
	}
	verb := s[j]
	v := pop()
	if verb == 's' {
		b.WriteString(fmt.Sprintf(spec+"s", strconv.Itoa(v)))
	} else {
		b.WriteString(fmt.Sprintf(spec+string(verb), v))
	}
	return j, true
}
