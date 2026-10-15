// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package treecodec

import (
	"reflect"
	"unsafe"

	"github.com/blairham/sh/syntax"
)

// Settle decodes every deferred body in a tree this package read back and
// puts it where the parser puts a body, so that the tree is the very value
// the parser made — reflect.DeepEqual and all — rather than one that only
// reads the same through syntax.FuncDecl.Body.
//
// It is for tests, which compare trees field by field and cannot see a body
// that has not been read. It writes to the tree, so nothing else may be
// reading it at the time; a shell never calls it.
func Settle(tree any) {
	settle(reflect.ValueOf(tree), map[unsafe.Pointer]bool{})
}

var deferredField = func() int {
	f, ok := funcDeclType.FieldByName("deferred")
	if !ok {
		panic("treecodec: syntax.FuncDecl has no deferred body field")
	}
	return f.Index[0]
}()

func settle(v reflect.Value, seen map[unsafe.Pointer]bool) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		p := v.UnsafePointer()
		if seen[p] {
			return
		}
		seen[p] = true
		if fd, ok := v.Interface().(*syntax.FuncDecl); ok {
			body := fd.Body()
			fd.BodyCommand = body
			f := v.Elem().Field(deferredField)
			reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().SetZero()
		}
		settle(v.Elem(), seen)
	case reflect.Interface:
		if !v.IsNil() {
			settle(v.Elem(), seen)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() || v.Type().Field(i).Anonymous {
				settle(v.Field(i), seen)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			settle(v.Index(i), seen)
		}
	}
}
