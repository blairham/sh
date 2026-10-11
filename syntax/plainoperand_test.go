// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// An operand made only of plain characters skips the lexer (#6354), and the
// shortcut is only sound while it builds exactly the tree the lexer would: the
// same spans, at the same positions, with the same quoting, on the first line
// and on later ones. So a script of such operands is parsed both ways and the
// two trees compared whole — under POSIX, under Core, and under Core with every
// grammar flag moved in turn, which reaches whatever a dialect could set
// without naming one.
//
// Not parallel: the switch is package-wide, and a top-level test that is not
// parallel finishes before any parallel one starts.
func TestPlainOperandsReadAsTheLexerReadsThem(t *testing.T) {
	words := []string{
		"value", "2", "a_b", "a.b-c", "/a,b:c", "+1", "-", "if", "function",
	}
	ops := []string{
		"${v#%s}", "${v##%s}", "${v%%%s}", "${v%%%%%s}", "${v:-%s}", "${v-%s}",
		"${v:+%s}", "${v+%s}", "${v:=%s}", "${v/%s/%s}", "${v//%s}", "${v/%s}",
		"x${v:-%s}y", "${v:-a%s}", "${v:-${w:-%s}}",
	}
	var b strings.Builder
	shortcuts := 0
	for _, op := range ops {
		for _, w := range words {
			word := strings.ReplaceAll(op, "%s", w)
			word = strings.ReplaceAll(word, "%%", "%")
			fmt.Fprintf(&b, ": %s\nif :; then\n\tx=%s\nfi\n", word, word)
			if plainOperand(w) {
				shortcuts++
			}
		}
	}
	if shortcuts == 0 {
		t.Fatal("no operand in the script takes the shortcut")
	}
	src := b.String()

	read := func(d Dialect, shortcut bool) (*File, string) {
		was := plainOperandShortcut
		plainOperandShortcut = shortcut
		defer func() { plainOperandShortcut = was }()
		f, err := Parse(src, d)
		return f, fmt.Sprint(err)
	}
	compare := func(name string, d Dialect) {
		fast, fastErr := read(d, true)
		slow, slowErr := read(d, false)
		switch {
		case fastErr != slowErr:
			t.Errorf("%s: error %s with the shortcut, %s without", name, fastErr, slowErr)
		case !reflect.DeepEqual(fast, slow):
			t.Errorf("%s: the shortcut built a different tree", name)
		}
	}
	compare("posix", POSIX())
	d := Core()
	compare("core", d)
	v := reflect.ValueOf(&d).Elem()
	for i := range v.NumField() {
		f := v.Field(i)
		name := "core." + v.Type().Field(i).Name
		switch f.Kind() {
		case reflect.Bool:
			f.SetBool(!f.Bool())
			compare(name, d)
			f.SetBool(!f.Bool())
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Int, reflect.Int8, reflect.Int32:
			was := reflect.ValueOf(f.Interface())
			for n := 1; n <= 2; n++ {
				if f.CanUint() {
					f.SetUint(uint64(n))
				} else {
					f.SetInt(int64(n))
				}
				compare(fmt.Sprintf("%s=%d", name, n), d)
			}
			f.Set(was)
		}
	}
}
