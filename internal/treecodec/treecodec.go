// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package treecodec writes a parsed program out as bytes and reads it back as
// the same tree, for the parse cache (#6424).
//
// The format is this shell's own and nothing else reads it. It is a walk of
// the tree's fields in declaration order, planned by reflection over the
// syntax package's types rather than by generated code, so a field added to
// a node is carried without anybody regenerating anything — the tree's types
// change most weeks, and a generated codec would have to follow every one of
// those changes by hand. What it cannot carry it refuses rather than
// approximates, and a refused tree is simply not cached:
//
//   - a non-nil error anywhere in the tree (File.Refused, a deferred refusal
//     on a span). Those carry unexported state and are what a parse failure
//     looks like, which the cache never keeps anyway;
//   - a node whose concrete type is not in the registry below. A node kind
//     added to syntax without a line here makes its trees uncacheable, which
//     costs speed and never correctness — and TestTheRegistryNamesEveryNode
//     fails until the line is added;
//   - any field kind a tree does not hold today: a map, a func, a channel, or
//     an unexported field that is not the embedded redirection list. Those
//     fail the plan, and with it every encoding, until this package learns
//     them.
//
// # Why reading is not reflection
//
// The point of the cache is to be cheaper than the parser, and the parser is
// already close to the cost of allocating the tree it returns: measured over
// 1.6 MB of the plugin files a real startup sources, parsing made 483,000
// allocations and decoding the same trees through reflect.Value made 435,000
// and took slightly *longer* than the parse. So the reader writes fields
// through their offsets, the way compiled code would, and takes each tree's
// nodes out of one block per type rather than one allocation per node. The
// plan that drives it is still built by reflection, once, from the types as
// they are in this build.
//
// # Strings point into the source
//
// Nearly every string in a tree is a slice of the text it was parsed from —
// a word's value, an operator, a name. The encoder notices that by address
// and writes such a string as an offset and a length into the source, and
// the decoder slices the caller's copy of the same source. So a decoded tree
// holds on to the source, as a parsed one does, and not to the cache entry.
// A string the parser built for itself is written out whole.
//
// Pointer identity is kept. A node reached twice is written once and the
// second reach refers back to it, so the tree read back shares exactly what
// the parsed one shared — a CarriedHeredoc's operators are the same values as
// the ones in the command that holds them.
//
// Nil and empty slices are told apart, because reflect.DeepEqual tells them
// apart, and that comparison is how the cache proves itself.
package treecodec

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"math/bits"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"github.com/blairham/sh/syntax"
)

// registry is every concrete type an interface field of the tree can hold, in
// a fixed order: a type's position is its tag in the encoding. The table is
// folded into Layout, so reordering it changes the layout and an entry
// written under the old order is never read under the new one.
var registry = []any{
	// syntax.Expr
	(*syntax.BinaryExpr)(nil),
	(*syntax.Pipeline)(nil),
	(*syntax.TimeClause)(nil),
	// syntax.Command
	(*syntax.SimpleCmd)(nil),
	(*syntax.Subshell)(nil),
	(*syntax.Group)(nil),
	(*syntax.TryClause)(nil),
	(*syntax.IfClause)(nil),
	(*syntax.LoopClause)(nil),
	(*syntax.ForClause)(nil),
	(*syntax.SelectClause)(nil),
	(*syntax.AnonFunc)(nil),
	(*syntax.RepeatClause)(nil),
	(*syntax.CoprocClause)(nil),
	(*syntax.CaseClause)(nil),
	(*syntax.ForArithClause)(nil),
	(*syntax.ArithCmdClause)(nil),
	(*syntax.FuncDecl)(nil),
	(*syntax.NamespaceClause)(nil),
	(*syntax.TestClause)(nil),
	// syntax.CondExpr
	(*syntax.CondUnary)(nil),
	(*syntax.CondUnknown)(nil),
	(*syntax.CondCompletion)(nil),
	(*syntax.CondBinary)(nil),
	(*syntax.CondLogic)(nil),
	(*syntax.CondNot)(nil),
	(*syntax.CondGroup)(nil),
	// syntax.ArithExpr
	(*syntax.ArithNum)(nil),
	(*syntax.ArithVar)(nil),
	(*syntax.ArithCharCode)(nil),
	(*syntax.ArithOutput)(nil),
	(*syntax.ArithUnary)(nil),
	(*syntax.ArithBinary)(nil),
	(*syntax.ArithCond)(nil),
	(*syntax.ArithIndex)(nil),
	(*syntax.ArithCall)(nil),
	(*syntax.ArithAssign)(nil),
}

// Registry is the registry's types, for the test that checks it against the
// syntax package's own node kinds.
func Registry() []reflect.Type {
	out := make([]reflect.Type, len(registry))
	for i, x := range registry {
		out[i] = reflect.TypeOf(x)
	}
	return out
}

// ErrUncacheable is what Encode reports for a tree it will not write. It is
// not a failure of the caller's: the tree is simply parsed again next time.
var ErrUncacheable = errors.New("treecodec: tree cannot be cached")

// ErrCorrupt is what Decoder reports for bytes that are not a tree this
// package wrote.
var ErrCorrupt = errors.New("treecodec: corrupt encoding")

// op is what one field or element is, as far as the walk is concerned.
type op uint8

const (
	opBool op = iota
	opInt
	opUint
	opString
	opStruct
	opPointer
	opSlice
	opInterface
	opMustBeNil // an error value, or a pointer to syntax.Error: nil or refused
	opPos       // syntax.Pos, the commonest struct in a tree, read without a walk
)

// plan is how one type is walked. Built once per type, on first use.
type plan struct {
	id     int
	t      reflect.Type
	op     op
	size   uintptr // t.Size(); for opInt and opUint, which store to write
	fields []field // opStruct
	elem   *plan   // opPointer (the struct), opSlice (the element)

	// itabs is, for an interface, the method table word of each registered
	// type that implements it, indexed by tag, and tagOf the other way
	// round; see interfaceWords.
	itabs []unsafe.Pointer
	tagOf map[unsafe.Pointer]int

	// leaf is a type that holds no pointer the walk would follow, so a
	// slice of it is counted by its length alone; inner is a struct's fields
	// that are not leaves, which are all the counting passes visit.
	leaf  bool
	inner []field
}

// field is one step of a struct: where the field sits from the start of the
// outermost struct, and how to walk what is there.
type field struct {
	name   string
	offset uintptr
	plan   *plan
}

type codec struct {
	plans   map[reflect.Type]*plan
	byID    []*plan
	tags    map[reflect.Type]int
	types   []*plan // the registry's types' plans, by tag
	file    *plan
	remarks *plan
	layout  uint64
	err     error
}

var shared = sync.OnceValue(build)

var errType = reflect.TypeFor[error]()

func build() *codec {
	c := &codec{plans: map[reflect.Type]*plan{}, tags: map[reflect.Type]int{}}
	h := fnv.New64a()
	for i, x := range registry {
		t := reflect.TypeOf(x)
		c.tags[t] = i
		c.types = append(c.types, c.planFor(t, h))
	}
	c.file = c.planFor(reflect.TypeFor[*syntax.File](), h)
	c.remarks = c.planFor(reflect.TypeFor[[]syntax.Remark](), h)
	for _, p := range c.byID {
		if p.op == opInterface {
			c.interfaceWords(p)
		}
	}
	for _, p := range c.byID {
		p.leaf = c.isLeaf(p, map[*plan]bool{})
	}
	for _, p := range c.byID {
		for _, f := range p.fields {
			if !f.plan.leaf {
				p.inner = append(p.inner, f)
			}
		}
	}
	c.layout = h.Sum64()
	return c
}

// Layout is a fingerprint of every type the walk reaches and of the tag
// table, so an entry written by a build whose tree had another shape is never
// read as this one's. It is a backstop under the build identity the cache
// keys on, not a replacement for it.
func Layout() (uint64, error) {
	c := shared()
	return c.layout, c.err
}

type hasher interface{ Write([]byte) (int, error) }

func (c *codec) planFor(t reflect.Type, h hasher) *plan {
	if p, ok := c.plans[t]; ok {
		return p
	}
	p := &plan{id: len(c.byID), t: t, size: t.Size()}
	c.plans[t] = p
	c.byID = append(c.byID, p)
	_, _ = fmt.Fprintf(h, "%s:%s:%d;", t.String(), t.Kind(), t.Size())
	if t == errType || t == reflect.TypeFor[*syntax.Error]() {
		p.op = opMustBeNil
		return p
	}
	if t == reflect.TypeFor[syntax.Pos]() {
		// Read as its three fields by name, so a fourth would be dropped
		// without a word: refuse the plan instead, until opPos learns it.
		p.op = opPos
		if t.NumField() != 3 || t.Field(0).Name != "Offset" || t.Field(1).Name != "Line" || t.Field(2).Name != "Col" {
			c.fail("a syntax.Pos of another shape (%d fields)", t.NumField())
		}
		return p
	}
	switch t.Kind() {
	case reflect.Bool:
		p.op = opBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		p.op = opInt
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		p.op = opUint
	case reflect.String:
		p.op = opString
	case reflect.Pointer:
		if t.Elem().Kind() != reflect.Struct {
			c.fail("a pointer to %s", t.Elem())
			return p
		}
		p.op = opPointer
		p.elem = c.planFor(t.Elem(), h)
	case reflect.Slice:
		p.op = opSlice
		p.elem = c.planFor(t.Elem(), h)
	case reflect.Interface:
		p.op = opInterface
	case reflect.Struct:
		p.op = opStruct
		c.structFields(p, t, 0, h)
	default:
		c.fail("a field of kind %s (%s)", t.Kind(), t)
	}
	return p
}

// structFields lists t's fields in order, stepping into the one embedded
// struct whose type is unexported — the redirection list every compound
// command carries — so that its exported field is walked as the promoted
// field it is to every reader outside syntax.
func (c *codec) structFields(p *plan, t reflect.Type, base uintptr, h hasher) {
	for i := range t.NumField() {
		f := t.Field(i)
		_, _ = fmt.Fprintf(h, "%s.%s@%d;", t.String(), f.Name, base+f.Offset)
		if !f.IsExported() {
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				c.structFields(p, f.Type, base+f.Offset, h)
				continue
			}
			if t == reflect.TypeFor[syntax.Error]() {
				// Never walked: an error in a tree refuses the tree.
				continue
			}
			c.fail("the unexported field %s.%s", t, f.Name)
			continue
		}
		p.fields = append(p.fields, field{name: f.Name, offset: base + f.Offset, plan: c.planFor(f.Type, h)})
	}
}

// interfaceWords finds, for each registered type that implements the
// interface p is, the first word of an interface value holding it — the
// method table — so that the decoder can write an interface field as two
// words without going through reflect.Value.
//
// That is a statement about how this toolchain lays out an interface value,
// so it is checked here rather than assumed: a value written that way is read
// back through reflection and must come back as the same type and the same
// pointer. If it ever does not, the codec refuses everything — the cache is
// off and the shell parses, which is the safe failure.
func (c *codec) interfaceWords(p *plan) {
	p.itabs = make([]unsafe.Pointer, len(c.types))
	p.tagOf = map[unsafe.Pointer]int{}
	for tag, cp := range c.types {
		if !cp.t.Implements(p.t) {
			continue
		}
		v := reflect.New(p.t).Elem()
		v.Set(reflect.Zero(cp.t))
		words := (*[2]unsafe.Pointer)(v.Addr().UnsafePointer())
		p.itabs[tag] = words[0]
		p.tagOf[words[0]] = tag

		probe := reflect.New(cp.t.Elem())
		slot := reflect.New(p.t)
		dst := (*[2]unsafe.Pointer)(slot.UnsafePointer())
		dst[0], dst[1] = p.itabs[tag], probe.UnsafePointer()
		got := slot.Elem()
		if got.IsNil() || got.Elem().Type() != cp.t || got.Elem().UnsafePointer() != probe.UnsafePointer() {
			c.fail("an interface value laid out as this toolchain lays one out (%s holding %s)", p.t, cp.t)
		}
	}
}

// isLeaf reports whether a value of p's type reaches nothing the count pass
// would count.
func (c *codec) isLeaf(p *plan, on map[*plan]bool) bool {
	switch p.op {
	case opPointer, opSlice, opInterface, opMustBeNil:
		return false
	case opStruct:
		if on[p] {
			return false
		}
		on[p] = true
		for _, f := range p.fields {
			if !c.isLeaf(f.plan, on) {
				return false
			}
		}
	}
	return true
}

func (c *codec) fail(format string, args ...any) {
	if c.err == nil {
		c.err = fmt.Errorf("treecodec: cannot carry "+format, args...)
	}
}

// Encoder writes a sequence of trees parsed from one source into one buffer.
// Each tree's pointers are numbered on their own.
//
// It reads the tree the way the decoder writes one, through each field's
// offset rather than through reflect.Value, because a cache that costs more
// to fill than the parse it saves makes the first start after any change
// slower — and that start is one the person sees too.
type Encoder struct {
	c      *codec
	src    string
	lo, hi uintptr // the source's addresses, for strings that point into it
	buf    []byte
	counts []int // per plan id: nodes to allocate, or slice elements
	err    error

	// side is every pointer the tree's side lists hold — File's fields
	// other than Stmts — with the id it was written under, once it was.
	// Sharing is confined to those lists: a span copied into Substitutions
	// carries the same operand pointers as the word it came out of, and a
	// CarriedHeredoc holds the very operators its command holds. The
	// statements themselves share nothing, which
	// TestTheStatementsShareNothing holds the parser to, so a pointer that is
	// not in this set is reached once and needs no bookkeeping.
	side map[ptrKey]int
	tree []byte
	// sideTypes is which plans the side set holds pointers of, so that a
	// pointer of any other type is not looked up at all.
	sideTypes []bool
	next      int
}

// ptrKey is a pointer and the plan of the type it points to: two types can
// start at one address when one is the other's first field.
type ptrKey struct {
	p  uintptr
	id int
}

// NewEncoder starts an encoding of trees parsed from src.
func NewEncoder(src string) *Encoder {
	c := shared()
	e := &Encoder{c: c, src: src, err: c.err}
	if len(src) > 0 {
		e.lo = uintptr(unsafe.Pointer(unsafe.StringData(src)))
		e.hi = e.lo + uintptr(len(src))
	}
	return e
}

// Bytes is everything encoded since the encoder was made or last reset. It
// is the encoder's own buffer, valid until the next Reset.
func (e *Encoder) Bytes() []byte { return e.buf }

// Reset empties the buffer and keeps its capacity, so one encoder can write
// many small trees without growing a buffer for each.
func (e *Encoder) Reset() { e.buf = e.buf[:0] }

// Err is the first refusal, or nil.
func (e *Encoder) Err() error { return e.err }

// File appends one tree.
func (e *Encoder) File(f *syntax.File) error {
	return e.root(e.c.file, unsafe.Pointer(&f))
}

// Remarks appends a list of the parser's remarks, which are kept beside a
// tree and walked the same way, so a field added to a remark is carried too.
func (e *Encoder) Remarks(rs []syntax.Remark) error {
	return e.root(e.c.remarks, unsafe.Pointer(&rs))
}

func (e *Encoder) root(p *plan, at unsafe.Pointer) error {
	if e.err != nil {
		return e.err
	}
	if e.counts == nil {
		e.counts = make([]int, len(e.c.byID))
	} else {
		clear(e.counts)
	}
	e.side, e.sideTypes, e.next = nil, nil, 0
	if p == e.c.file {
		e.sideLists(*(*unsafe.Pointer)(at))
	}
	// One pass over the tree, into a buffer of its own, counting as it
	// goes; the counts then go in front of it.
	held := e.buf
	e.buf = e.tree[:0]
	e.write(p, at)
	e.tree, e.buf = e.buf, held
	if e.err != nil {
		return e.err
	}
	n := 0
	for _, k := range e.counts {
		if k > 0 {
			n++
		}
	}
	// What to allocate, ahead of the tree, so the decoder can take the
	// whole tree's nodes of one type out of one block.
	e.buf = binary.AppendUvarint(e.buf, uint64(n))
	for id, k := range e.counts {
		if k > 0 {
			e.buf = binary.AppendUvarint(e.buf, uint64(id))
			e.buf = binary.AppendUvarint(e.buf, uint64(k))
		}
	}
	e.buf = append(e.buf, e.tree...)
	return nil
}

// sideLists collects the pointers File's side lists hold. Each is written
// as one to remember the first time it is reached, wherever that is, and as
// a reference back to it every time after.
func (e *Encoder) sideLists(file unsafe.Pointer) {
	if file == nil {
		return
	}
	fp := e.c.file.elem
	for i := range fp.fields {
		if fp.fields[i].name == "Stmts" {
			continue
		}
		e.collect(fp.fields[i].plan, unsafe.Add(file, fp.fields[i].offset))
	}
	if len(e.side) > 0 {
		e.sideTypes = make([]bool, len(e.c.byID))
		for k := range e.side {
			e.sideTypes[k.id] = true
		}
	}
}

func (e *Encoder) collect(p *plan, at unsafe.Pointer) {
	switch p.op {
	case opPointer:
		ptr := *(*unsafe.Pointer)(at)
		if ptr == nil {
			return
		}
		k := ptrKey{uintptr(ptr), p.id}
		if _, ok := e.side[k]; ok {
			return
		}
		if e.side == nil {
			e.side = map[ptrKey]int{}
		}
		e.side[k] = 0
		e.collect(p.elem, ptr)
	case opStruct:
		for i := range p.inner {
			e.collect(p.inner[i].plan, unsafe.Add(at, p.inner[i].offset))
		}
	case opSlice:
		if p.elem.leaf {
			return
		}
		h := (*sliceHeader)(at)
		for i := range uintptr(h.len) {
			e.collect(p.elem, unsafe.Add(h.data, i*p.elem.size))
		}
	case opInterface:
		words := (*[2]unsafe.Pointer)(at)
		if words[0] == nil {
			return
		}
		if tag, ok := p.tagOf[words[0]]; ok {
			e.collect(e.c.types[tag], unsafe.Pointer(&words[1]))
		}
	}
}

// Pointer tags: nil, new, new and remembered, and a reference back to a
// remembered one by its id plus tagRef.
const (
	tagNil = iota
	tagNew
	tagNewKept
	tagRef
)

// String tags: a slice of the source, or the bytes themselves.
const (
	strInSource = iota
	strInline
)

// isZero reports whether size bytes at at are all zero. A field whose bytes
// are all zero holds its zero value; one that holds its zero value in other
// bytes — an empty string with a pointer — is merely written out, which the
// reader turns back into the same zero value.
func isZero(at unsafe.Pointer, size uintptr) bool {
	for size >= 8 {
		if *(*uint64)(at) != 0 {
			return false
		}
		at = unsafe.Add(at, 8)
		size -= 8
	}
	for size > 0 {
		if *(*uint8)(at) != 0 {
			return false
		}
		at = unsafe.Add(at, 1)
		size--
	}
	return true
}

func (e *Encoder) write(p *plan, at unsafe.Pointer) {
	switch p.op {
	case opBool:
		// Never reached for a field, whose presence bit is its value; this
		// is a bool that is an element of a slice.
		if *(*bool)(at) {
			e.buf = append(e.buf, 1)
		} else {
			e.buf = append(e.buf, 0)
		}
	case opInt:
		var x int64
		switch p.size {
		case 1:
			x = int64(*(*int8)(at))
		case 2:
			x = int64(*(*int16)(at))
		case 4:
			x = int64(*(*int32)(at))
		case 8:
			x = *(*int64)(at)
		}
		e.buf = binary.AppendVarint(e.buf, x)
	case opUint:
		var x uint64
		switch p.size {
		case 1:
			x = uint64(*(*uint8)(at))
		case 2:
			x = uint64(*(*uint16)(at))
		case 4:
			x = uint64(*(*uint32)(at))
		case 8:
			x = *(*uint64)(at)
		}
		e.buf = binary.AppendUvarint(e.buf, x)
	case opString:
		s := *(*string)(at)
		if len(s) == 0 {
			e.buf = append(e.buf, strInSource, 0)
			return
		}
		ptr := uintptr(unsafe.Pointer(unsafe.StringData(s)))
		if ptr >= e.lo && ptr+uintptr(len(s)) <= e.hi {
			e.buf = append(e.buf, strInSource)
			e.buf = binary.AppendUvarint(e.buf, uint64(len(s)))
			e.buf = binary.AppendUvarint(e.buf, uint64(ptr-e.lo))
			return
		}
		e.buf = append(e.buf, strInline)
		e.buf = binary.AppendUvarint(e.buf, uint64(len(s)))
		e.buf = append(e.buf, s...)
	case opStruct:
		// Which fields are not their zero value, sixty-four to a word, and
		// then those fields alone: most of a node's fields are zero — a
		// parameter expansion has fifty-two of them — and a field that is
		// not written is a field the reader does not touch, since the
		// memory it reads into is already zero. A true bool is its bit and
		// nothing more.
		for lo := 0; lo < len(p.fields); lo += 64 {
			hi := min(lo+64, len(p.fields))
			var mask uint64
			for i := lo; i < hi; i++ {
				f := &p.fields[i]
				if !isZero(unsafe.Add(at, f.offset), f.plan.size) {
					mask |= 1 << (i - lo)
				}
			}
			e.buf = binary.AppendUvarint(e.buf, mask)
			for m := mask; m != 0; m &= m - 1 {
				f := &p.fields[lo+bits.TrailingZeros64(m)]
				if f.plan.op != opBool {
					e.write(f.plan, unsafe.Add(at, f.offset))
				}
			}
		}
	case opPos:
		pos := (*syntax.Pos)(at)
		e.buf = binary.AppendVarint(e.buf, int64(pos.Offset))
		e.buf = binary.AppendVarint(e.buf, int64(pos.Line))
		e.buf = binary.AppendVarint(e.buf, int64(pos.Col))
	case opPointer:
		ptr := *(*unsafe.Pointer)(at)
		if ptr == nil {
			e.buf = append(e.buf, tagNil)
			return
		}
		if e.side != nil && e.sideTypes[p.id] {
			k := ptrKey{uintptr(ptr), p.id}
			if id, ok := e.side[k]; ok {
				if id > 0 {
					e.buf = binary.AppendUvarint(e.buf, uint64(tagRef+id-1))
					return
				}
				e.next++
				e.side[k] = e.next
				e.counts[p.id]++
				e.buf = append(e.buf, tagNewKept)
				e.write(p.elem, ptr)
				return
			}
		}
		e.counts[p.id]++
		e.buf = append(e.buf, tagNew)
		e.write(p.elem, ptr)
	case opSlice:
		h := (*sliceHeader)(at)
		if h.data == nil {
			e.buf = append(e.buf, 0)
			return
		}
		e.buf = binary.AppendUvarint(e.buf, uint64(h.len)+1)
		e.counts[p.id] += h.len
		for i := range uintptr(h.len) {
			e.write(p.elem, unsafe.Add(h.data, i*p.elem.size))
		}
	case opInterface:
		words := (*[2]unsafe.Pointer)(at)
		if words[0] == nil {
			e.buf = append(e.buf, 0)
			return
		}
		tag, ok := p.tagOf[words[0]]
		if !ok {
			e.err = fmt.Errorf("%w: an interface holds a type that is not in the registry", ErrUncacheable)
			return
		}
		e.buf = binary.AppendUvarint(e.buf, uint64(tag)+1)
		e.write(e.c.types[tag], unsafe.Pointer(&words[1]))
	case opMustBeNil:
		if *(*unsafe.Pointer)(at) != nil {
			e.err = fmt.Errorf("%w: it holds a refusal (%s)", ErrUncacheable, p.t)
			return
		}
		e.buf = append(e.buf, 0)
	}
}

// Decoder reads trees back in the order they were written.
type Decoder struct {
	c    *codec
	data []byte
	src  string
	off  int
	kept []unsafe.Pointer
	// slab is, per plan id, the block the tree's nodes or slice elements of
	// that type are being taken from.
	slab []slab
	err  error
}

type slab struct {
	base unsafe.Pointer
	next uintptr // bytes handed out so far
	end  uintptr
}

// NewDecoder reads data, which holds trees parsed from src. src must be the
// same text, byte for byte: the trees' strings are slices of it.
func NewDecoder(data []byte, src string) *Decoder {
	c := shared()
	return &Decoder{c: c, data: data, src: src, err: c.err, slab: make([]slab, len(c.byID))}
}

// Offset is how far into the data the decoder has read.
func (d *Decoder) Offset() int { return d.off }

// File reads the next tree.
func (d *Decoder) File() (*syntax.File, error) {
	var out *syntax.File
	err := d.root(d.c.file, unsafe.Pointer(&out))
	return out, err
}

// Remarks reads a list Encoder.Remarks wrote.
func (d *Decoder) Remarks() ([]syntax.Remark, error) {
	var out []syntax.Remark
	err := d.root(d.c.remarks, unsafe.Pointer(&out))
	return out, err
}

// root reads one value of p's type into at.
func (d *Decoder) root(p *plan, at unsafe.Pointer) (err error) {
	if d.err != nil {
		return d.err
	}
	defer func() {
		// Bytes that index past the end, a slice length no tree could have,
		// a tag that names nothing: each panics in a slice expression or an
		// index, and every one of them means the same thing — this is not
		// something Encode wrote.
		if rec := recover(); rec != nil {
			err = fmt.Errorf("%w: %v", ErrCorrupt, rec)
			d.err = err
		}
	}()
	d.kept = d.kept[:0]
	clear(d.slab)
	for n := d.uvarint(); n > 0; n-- {
		id, k := d.uvarint(), d.uvarint()
		p := d.c.byID[id]
		var t reflect.Type
		switch p.op {
		case opPointer:
			t = p.elem.t
		case opSlice:
			t = p.elem.t
		default:
			panic("an allocation of a type that is not allocated")
		}
		if k > uint64(len(d.data)) {
			panic("more nodes than bytes")
		}
		block := reflect.MakeSlice(reflect.SliceOf(t), int(k), int(k))
		d.slab[id] = slab{base: block.UnsafePointer(), end: uintptr(k) * t.Size()}
	}
	d.read(p, at)
	return nil
}

// take hands out n consecutive values of the type plan id allocates.
func (d *Decoder) take(id int, size uintptr, n int) unsafe.Pointer {
	s := &d.slab[id]
	want := size * uintptr(n)
	if s.next+want > s.end {
		panic("a tree larger than its own header")
	}
	at := unsafe.Add(s.base, s.next)
	s.next += want
	return at
}

func (d *Decoder) uvarint() uint64 {
	x, n := binary.Uvarint(d.data[d.off:])
	if n <= 0 {
		panic("bad uvarint")
	}
	d.off += n
	return x
}

func (d *Decoder) varint() int64 {
	x, n := binary.Varint(d.data[d.off:])
	if n <= 0 {
		panic("bad varint")
	}
	d.off += n
	return x
}

func (d *Decoder) byte() byte {
	b := d.data[d.off]
	d.off++
	return b
}

// read decodes one value of p's type into the memory at at, which holds a
// zero value of that type.
func (d *Decoder) read(p *plan, at unsafe.Pointer) {
	switch p.op {
	case opBool:
		switch d.byte() {
		case 0:
		case 1:
			*(*bool)(at) = true
		default:
			panic("bad bool")
		}
	case opInt:
		x := d.varint()
		switch p.size {
		case 1:
			*(*int8)(at) = int8(x)
		case 2:
			*(*int16)(at) = int16(x)
		case 4:
			*(*int32)(at) = int32(x)
		case 8:
			*(*int64)(at) = x
		}
	case opUint:
		x := d.uvarint()
		switch p.size {
		case 1:
			*(*uint8)(at) = uint8(x)
		case 2:
			*(*uint16)(at) = uint16(x)
		case 4:
			*(*uint32)(at) = uint32(x)
		case 8:
			*(*uint64)(at) = x
		}
	case opString:
		tag := d.byte()
		n := int(d.uvarint())
		switch tag {
		case strInSource:
			if n == 0 {
				return
			}
			o := int(d.uvarint())
			*(*string)(at) = d.src[o : o+n]
		case strInline:
			// A copy rather than a slice of the entry, so the tree does not
			// keep the whole entry alive for the sake of a few strings.
			*(*string)(at) = strings.Clone(string(d.data[d.off : d.off+n]))
			d.off += n
		default:
			panic("bad string tag")
		}
	case opStruct:
		for lo := 0; lo < len(p.fields); lo += 64 {
			for mask := d.uvarint(); mask != 0; mask &= mask - 1 {
				f := &p.fields[lo+bits.TrailingZeros64(mask)]
				if f.plan.op == opBool {
					*(*bool)(unsafe.Add(at, f.offset)) = true
					continue
				}
				d.read(f.plan, unsafe.Add(at, f.offset))
			}
		}
	case opPos:
		pos := (*syntax.Pos)(at)
		pos.Offset = int32(d.varint())
		pos.Line = int32(d.varint())
		pos.Col = int32(d.varint())
	case opPointer:
		switch tag := d.uvarint(); tag {
		case tagNil:
		case tagNew, tagNewKept:
			n := d.take(p.id, p.elem.size, 1)
			if tag == tagNewKept {
				d.kept = append(d.kept, n)
			}
			d.read(p.elem, n)
			*(*unsafe.Pointer)(at) = n
		default:
			*(*unsafe.Pointer)(at) = d.kept[tag-tagRef]
		}
	case opSlice:
		n := d.uvarint()
		if n == 0 {
			return
		}
		n--
		if n == 0 {
			// Empty and not nil: any non-nil pointer will do, and the
			// zero-size base Go itself uses for one is not reachable from
			// here, so a block of one element's worth stands in.
			*(*sliceHeader)(at) = sliceHeader{data: unsafe.Pointer(&zeroBase)}
			return
		}
		base := d.take(p.id, p.elem.size, int(n))
		for i := range uintptr(n) {
			d.read(p.elem, unsafe.Add(base, i*p.elem.size))
		}
		*(*sliceHeader)(at) = sliceHeader{data: base, len: int(n), cap: int(n)}
	case opInterface:
		tag := d.uvarint()
		if tag == 0 {
			return
		}
		cp := d.c.types[tag-1]
		itab := p.itabs[tag-1]
		if itab == nil {
			panic("an interface holding a type that does not implement it")
		}
		var ptr unsafe.Pointer
		d.read(cp, unsafe.Pointer(&ptr))
		words := (*[2]unsafe.Pointer)(at)
		words[0], words[1] = itab, ptr
	case opMustBeNil:
		if d.byte() != 0 {
			panic("a refusal in a cached tree")
		}
	}
}

// sliceHeader is a slice's three words, written in one store.
type sliceHeader struct {
	data     unsafe.Pointer
	len, cap int
}

// zeroBase is where an empty, non-nil slice points.
var zeroBase uintptr
