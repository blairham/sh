// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// collatingElementNames is the roster a `[.name.]` or a `[=name=]` is looked
// up in where the dialect reads a body of more than one character as a name —
// see [ACollatingElementMayBeNamed].
//
// **It is measured rather than derived**, a name at a time, and the two are
// not the same list. Swept 2026-09-18 against bash 5.3.20 under `LC_ALL=C`,
// with every name of POSIX's portable character set asked under both
// delimiters against the character it stands for and against a character it
// does not:
//
//   - 86 of the 87 that can be written in a shell word are taken. `NUL` is
//     the 88th and cannot be asked, since no shell word holds the character.
//   - `low-line` is the one refused, and its character is reached under
//     `underscore` — so the roster is not the charmap's aliases, and a table
//     derived from the standard would have carried a name the shell has not.
//   - Eight names outside the portable set are taken: the control
//     abbreviations `BS`, `HT`, `LF`, `VT`, `FF` and `CR`, and `minus` and
//     `dash` for the hyphen. `BEL`, `NL`, `SP`, `XON`, `XOFF`,
//     `left-bracket`, `right-bracket`, `underline` and `vertical-bar` are
//     refused, so the abbreviations are a chosen set and not a pattern to
//     extend by guessing.
//   - The lookup is case-sensitive: `HYPHEN` and `Hyphen` are refused where
//     `hyphen` is taken.
//
// A body outside the roster is a body that is not an element, exactly as a
// body of the wrong length is, and [Semantics.UnknownCharacterClass] says
// what that does to the bracket around it.
//
// Written out rather than generated, unlike the Unicode tables this package
// builds: the roster is 95 lines of measurement and no standard states it, so
// there is nothing to generate it from and the sweep above is the source.
var collatingElementNames = map[string]string{
	"SOH": "\x01", "STX": "\x02", "ETX": "\x03", "EOT": "\x04",
	"ENQ": "\x05", "ACK": "\x06", "alert": "\a",
	"BS": "\b", "backspace": "\b",
	"HT": "\t", "tab": "\t",
	"LF": "\n", "newline": "\n",
	"VT": "\v", "vertical-tab": "\v",
	"FF": "\f", "form-feed": "\f",
	"CR": "\r", "carriage-return": "\r",
	"SO": "\x0e", "SI": "\x0f", "DLE": "\x10",
	"DC1": "\x11", "DC2": "\x12", "DC3": "\x13", "DC4": "\x14",
	"NAK": "\x15", "SYN": "\x16", "ETB": "\x17", "CAN": "\x18",
	"EM": "\x19", "SUB": "\x1a", "ESC": "\x1b",
	"IS4": "\x1c", "FS": "\x1c",
	"IS3": "\x1d", "GS": "\x1d",
	"IS2": "\x1e", "RS": "\x1e",
	"IS1": "\x1f", "US": "\x1f",
	"space":             " ",
	"exclamation-mark":  "!",
	"quotation-mark":    `"`,
	"number-sign":       "#",
	"dollar-sign":       "$",
	"percent-sign":      "%",
	"ampersand":         "&",
	"apostrophe":        "'",
	"left-parenthesis":  "(",
	"right-parenthesis": ")",
	"asterisk":          "*",
	"plus-sign":         "+",
	"comma":             ",",
	"hyphen":            "-", "hyphen-minus": "-", "minus": "-", "dash": "-",
	"period": ".", "full-stop": ".",
	"slash": "/", "solidus": "/",
	"zero": "0", "one": "1", "two": "2", "three": "3", "four": "4",
	"five": "5", "six": "6", "seven": "7", "eight": "8", "nine": "9",
	"colon":               ":",
	"semicolon":           ";",
	"less-than-sign":      "<",
	"equals-sign":         "=",
	"greater-than-sign":   ">",
	"question-mark":       "?",
	"commercial-at":       "@",
	"left-square-bracket": "[",
	"backslash":           `\`, "reverse-solidus": `\`,
	"right-square-bracket": "]",
	"circumflex":           "^", "circumflex-accent": "^",
	"underscore":   "_",
	"grave-accent": "`",
	"left-brace":   "{", "left-curly-bracket": "{",
	"vertical-line": "|",
	"right-brace":   "}", "right-curly-bracket": "}",
	"tilde": "~",
	"DEL":   "\x7f",
}

// collatingElementNamed is the character a name stands for, and whether the
// roster holds the name at all.
//
// A body of one character is not looked up here: it is the element it spells
// in every column that reads one, and a one-character name would be the same
// answer twice. That keeps the roster a table of *names* rather than a second
// copy of the C locale.
func collatingElementNamed(body string) (string, bool) {
	if len(body) < 2 {
		return "", false
	}
	c, ok := collatingElementNames[body]
	return c, ok
}
