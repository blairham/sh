// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package asttree writes a parsed program out as Go source: one composite
// literal that the compiler lays down in the binary's data, so the tree costs
// nothing to build when the program starts.
//
// It exists for the dialect preludes (#6397). A prelude is a constant of the
// binary, and parsing the same six kilobytes of it on every start was about a
// quarter of what this shell's start costs above an empty Go program's. The
// tree is a constant too, so it is written down once, by internal/preludegen,
// and each dialect's tests parse the text again and require the two to be
// equal — which is what makes the literal a cache rather than a second
// source of truth.
//
// The literal is refused rather than approximated wherever Go source cannot
// say what the tree holds: a field this package cannot set from outside
// syntax, a node reached twice, a map, a function. Each of those would make a
// tree that compares equal by value and is not the same tree, so the
// generator stops and says so.
package asttree

import (
	"bytes"
	"fmt"
	"go/format"
	"reflect"
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
)

// Constant is a stretch of the program's text that differs from one build
// to the next and is spelled in the literal as a Go constant expression
// rather than as what this build happened to hold.
//
// The bash prelude is the case: its BASH_VERSINFO names the machine the
// binary was built for, so the text — and every position after that word —
// is one thing on arm64 macOS and another on amd64 Linux. Writing the word
// as `machineName` and each later offset as `N + int32(len(machineName))`
// keeps the literal a compile-time constant on every target, and the
// dialect's test, run on each, is what proves the arithmetic.
type Constant struct {
	// At is the byte offset in the source where the stretch starts.
	At int
	// Text is what the stretch holds in the source the tree was parsed from.
	Text string
	// Expr is the Go constant expression of type string that spells it.
	Expr string
}

// Emit writes a Go source file declaring
//
//	var name = &syntax.File{...}
//
// holding exactly f, in package pkg. src is the text f was parsed from, and
// header is written above the package clause.
func Emit(f *syntax.File, src, pkg, name, header string, consts []Constant) ([]byte, error) {
	for _, c := range consts {
		if c.At < 0 || c.At+len(c.Text) > len(src) || src[c.At:c.At+len(c.Text)] != c.Text {
			return nil, fmt.Errorf("asttree: constant %s is not %q at offset %d", c.Expr, c.Text, c.At)
		}
	}
	e := &emitter{src: src, consts: consts, seen: map[uintptr]bool{}}
	var b bytes.Buffer
	b.WriteString(header)
	fmt.Fprintf(&b, "package %s\n\nimport \"github.com/blairham/sh/syntax\"\n\n", pkg)
	fmt.Fprintf(&b, "var %s = ", name)
	if err := e.value(&b, reflect.ValueOf(f), true); err != nil {
		return nil, err
	}
	b.WriteString("\n")
	out, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("asttree: formatting: %w", err)
	}
	return out, nil
}

type emitter struct {
	src    string
	consts []Constant
	seen   map[uintptr]bool
}

// value writes v as a Go expression. typed is false where the element type
// may be elided — an element of a slice literal of the same type.
func (e *emitter) value(b *bytes.Buffer, v reflect.Value, typed bool) error {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			b.WriteString("nil")
			return nil
		}
		if v.Elem().Kind() != reflect.Struct {
			return fmt.Errorf("asttree: a pointer to %s has no literal", v.Elem().Type())
		}
		if e.seen[v.Pointer()] {
			return fmt.Errorf("asttree: %s is reached twice; a literal would make two of it", v.Type())
		}
		e.seen[v.Pointer()] = true
		if typed {
			b.WriteString("&")
		}
		return e.value(b, v.Elem(), typed)
	case reflect.Interface:
		if v.IsNil() {
			b.WriteString("nil")
			return nil
		}
		return e.value(b, v.Elem(), true)
	case reflect.Struct:
		if p, ok := v.Interface().(syntax.Pos); ok {
			return e.pos(b, p)
		}
		if typed {
			b.WriteString(v.Type().String())
		}
		b.WriteString("{")
		for i := range v.NumField() {
			sf, fv := v.Type().Field(i), v.Field(i)
			if fv.IsZero() {
				continue
			}
			if !sf.IsExported() {
				return fmt.Errorf("asttree: %s.%s is set and cannot be written outside syntax", v.Type(), sf.Name)
			}
			fmt.Fprintf(b, "\n%s: ", sf.Name)
			if err := e.value(b, fv, true); err != nil {
				return err
			}
			b.WriteString(",")
		}
		b.WriteString("\n}")
		return nil
	case reflect.Slice:
		if v.IsNil() {
			b.WriteString("nil")
			return nil
		}
		b.WriteString(v.Type().String())
		b.WriteString("{")
		// An element is elided to `{...}` only where its type is the
		// slice's own; an interface element has to name what it holds.
		elide := v.Type().Elem().Kind() != reflect.Interface
		for i := range v.Len() {
			b.WriteString("\n")
			if err := e.value(b, v.Index(i), !elide); err != nil {
				return err
			}
			b.WriteString(",")
		}
		b.WriteString("\n}")
		return nil
	case reflect.String:
		e.str(b, v.String(), v.Type())
		return nil
	case reflect.Bool:
		b.WriteString(strconv.FormatBool(v.Bool()))
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.WriteString(strconv.FormatInt(v.Int(), 10))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b.WriteString(strconv.FormatUint(v.Uint(), 10))
		return nil
	default:
		if v.IsZero() {
			b.WriteString("nil")
			return nil
		}
		return fmt.Errorf("asttree: a %s has no literal", v.Type())
	}
}

// str writes a string, spelling any constant stretch it carries as the
// constant's expression.
func (e *emitter) str(b *bytes.Buffer, s string, t reflect.Type) {
	var parts []string
	for _, c := range e.consts {
		if !strings.Contains(s, c.Text) {
			continue
		}
		for i, piece := range strings.Split(s, c.Text) {
			if i > 0 {
				parts = append(parts, c.Expr)
			}
			if piece != "" {
				parts = append(parts, strconv.Quote(piece))
			}
		}
		break
	}
	if parts == nil {
		parts = []string{strconv.Quote(s)}
	}
	expr := strings.Join(parts, " + ")
	if t.PkgPath() != "" {
		expr = t.String() + "(" + expr + ")"
	}
	b.WriteString(expr)
}

// pos writes a position, with an offset past a constant stretch — and a
// column past one on the same line — counted from the stretch's own length
// rather than from this build's.
func (e *emitter) pos(b *bytes.Buffer, p syntax.Pos) error {
	if int(p.Offset) > len(e.src) {
		return fmt.Errorf("asttree: a position at offset %d is past the end of the source", p.Offset)
	}
	off, col := fmt.Sprint(p.Offset), fmt.Sprint(p.Col)
	var offAdd, colAdd []string
	n, c := int(p.Offset), int(p.Col)
	for _, k := range e.consts {
		end := k.At + len(k.Text)
		if int(p.Offset) > k.At && int(p.Offset) < end {
			return fmt.Errorf("asttree: a position falls inside the constant %s", k.Expr)
		}
		if int(p.Offset) < end {
			continue
		}
		n -= len(k.Text)
		offAdd = append(offAdd, "int32(len("+k.Expr+"))")
		if strings.LastIndexByte(e.src[:p.Offset], '\n') < k.At {
			c -= len(k.Text)
			colAdd = append(colAdd, "int32(len("+k.Expr+"))")
		}
	}
	if offAdd != nil {
		off = strings.Join(append([]string{fmt.Sprint(n)}, offAdd...), " + ")
	}
	if colAdd != nil {
		col = strings.Join(append([]string{fmt.Sprint(c)}, colAdd...), " + ")
	}
	fmt.Fprintf(b, "syntax.Pos{Offset: %s, Line: %d, Col: %s}", off, p.Line, col)
	return nil
}
