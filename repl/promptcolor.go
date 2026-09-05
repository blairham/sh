// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "strconv"

// What a prompt's color code writes to the terminal.
//
// The dialect says which half of the screen its code paints and what was
// written in the braces; everything below is the terminal's own arithmetic —
// the select-graphic-rendition parameters, where 30 to 37 are the eight
// foreground colors, 90 to 97 their bright halves, 39 the default, and
// `38;5;n` the 256-color extension. Those numbers belong to the terminal in
// the same way the cell widths in cellwidth.go do, so a second dialect with a
// color code gets them without saying them again.
//
// Measured against zsh 5.9.2 through a pty, one code per prompt: `%F{red}`
// drew `\e[31m`, `%F{2}` drew `\e[32m`, `%F{9}` drew `\e[91m`, `%F{200}` drew
// `\e[38;5;200m` and `%K{blue}` drew `\e[44m`.
func colorSequence(layer PromptColor, arg string) string {
	n, ok := colorNumber(arg)
	if !ok {
		// Anything the table and the number range both refuse is the
		// terminal's default. Measured: `%F{bogus}`, `%F{Red}` — the names are
		// lower case and only lower case — `%F{256}` and `%F{-1}` all drew
		// `\e[39m`, which is the same answer `%f` gives.
		return sgr(defaultColor(layer))
	}
	switch {
	case n < 8:
		return sgr(colorBase(layer) + n)
	case n < 16:
		// The bright half, which is its own run of parameters rather than a
		// modifier on the first eight.
		return sgr(colorBright(layer) + n - 8)
	default:
		return "\x1b[" + strconv.Itoa(colorExtended(layer)) + ";5;" + strconv.Itoa(n) + "m"
	}
}

// colorNumber reads what was written in the braces.
//
// A name, or a number from 0 to 255, and nothing else. The empty argument is 0
// rather than the default, which is measured and is not what it looks like:
// zsh drew `%F` and `%F{}` both as `\e[30m`, the same as `%F{black}`.
func colorNumber(arg string) (int, bool) {
	if arg == "" {
		return 0, true
	}
	if n, ok := colorNames[arg]; ok {
		return n, true
	}
	n, err := strconv.Atoi(arg)
	if err != nil || n < 0 || n > 255 {
		return 0, false
	}
	return n, true
}

// colorNames is the eight the terminal names, in the order it numbers them.
var colorNames = map[string]int{
	"black": 0, "red": 1, "green": 2, "yellow": 3,
	"blue": 4, "magenta": 5, "cyan": 6, "white": 7,
}

func colorBase(layer PromptColor) int {
	if layer == Background {
		return 40
	}
	return 30
}

func colorBright(layer PromptColor) int {
	if layer == Background {
		return 100
	}
	return 90
}

func colorExtended(layer PromptColor) int {
	if layer == Background {
		return 48
	}
	return 38
}

func defaultColor(layer PromptColor) int {
	if layer == Background {
		return 49
	}
	return 39
}

func sgr(n int) string { return "\x1b[" + strconv.Itoa(n) + "m" }
