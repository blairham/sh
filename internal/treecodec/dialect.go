// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package treecodec

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	"github.com/blairham/sh/syntax"
)

// DialectFingerprint hashes every answer in d that decides how a text
// parses, so that a tree cached under one grammar is never handed to a parse
// under another.
//
// The four answers a dialect carries as functions — where a character ends,
// whether a letter past ASCII may be part of a name, how long a character
// is, and what a `$'…'` delimiter means — are not hashed: they are the
// running shell's answers to questions about particular bytes, and the cache
// records each question it saw asked and the answer it got instead. Whether
// each one is nil *is* hashed, because a reader that finds nil there takes
// that as an answer of its own.
//
// Built by reflection over the struct as it is in this build, so a grammar
// setting added to syntax.Dialect is part of the fingerprint without anybody
// remembering to add it. A field of a kind this cannot hash fails the plan
// and ok is false — the cache is then off rather than blind to the field.
func DialectFingerprint(d *syntax.Dialect) (sum uint64, ok bool) {
	p := dialectPlan()
	if p.err != nil {
		return 0, false
	}
	h := uint64(fnvOffset)
	base := unsafe.Pointer(d)
	for i := range p.fields {
		f := &p.fields[i]
		at := unsafe.Add(base, f.offset)
		switch f.kind {
		case reflect.Bool:
			h = mix(h, uint64(*(*uint8)(at)))
		case reflect.Uint8:
			h = mix(h, uint64(*(*uint8)(at)))
		case reflect.Int:
			h = mix(h, uint64(*(*int)(at)))
		case reflect.String:
			h = mixString(h, *(*string)(at))
		case reflect.Func:
			if *(*unsafe.Pointer)(at) == nil {
				h = mix(h, 0)
			} else {
				h = mix(h, 1)
			}
		case reflect.Map:
			// A sum of per-entry hashes, because a map has no order to hash
			// in; the length goes in as well so that two entries cannot
			// cancel to nothing.
			m := reflect.NewAt(f.typ, at).Elem()
			var acc uint64
			it := m.MapRange()
			for it.Next() {
				e := uint64(fnvOffset)
				e = mixValue(e, it.Key())
				e = mixValue(e, it.Value())
				acc += e
			}
			h = mix(mix(h, uint64(m.Len())), acc)
			if m.IsNil() {
				h = mix(h, 0xfeed)
			}
		}
	}
	return h, true
}

type dialectField struct {
	offset uintptr
	kind   reflect.Kind
	typ    reflect.Type
}

type dialectFields struct {
	fields []dialectField
	err    error
}

var dialectPlan = sync.OnceValue(func() dialectFields {
	var out dialectFields
	t := reflect.TypeFor[syntax.Dialect]()
	for i := range t.NumField() {
		f := t.Field(i)
		k := f.Type.Kind()
		switch k {
		case reflect.Bool, reflect.String, reflect.Func, reflect.Uint8, reflect.Int:
		case reflect.Map:
			if !scalar(f.Type.Key().Kind()) || !scalar(f.Type.Elem().Kind()) {
				out.err = fmt.Errorf("treecodec: cannot fingerprint %s.%s (%s)", t, f.Name, f.Type)
			}
		default:
			out.err = fmt.Errorf("treecodec: cannot fingerprint %s.%s (%s)", t, f.Name, f.Type)
		}
		out.fields = append(out.fields, dialectField{offset: f.Offset, kind: k, typ: f.Type})
	}
	return out
})

func scalar(k reflect.Kind) bool {
	switch k {
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

const (
	fnvOffset = 14695981039346656037
	fnvPrime  = 1099511628211
)

func mix(h, x uint64) uint64 {
	for range 8 {
		h ^= x & 0xff
		h *= fnvPrime
		x >>= 8
	}
	return h
}

func mixString(h uint64, s string) uint64 {
	h = mix(h, uint64(len(s)))
	for i := range len(s) {
		h ^= uint64(s[i])
		h *= fnvPrime
	}
	return h
}

func mixValue(h uint64, v reflect.Value) uint64 {
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			return mix(h, 1)
		}
		return mix(h, 0)
	case reflect.String:
		return mixString(h, v.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return mix(h, uint64(v.Int()))
	default:
		return mix(h, v.Uint())
	}
}
