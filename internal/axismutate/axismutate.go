// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package axismutate moves one field of a settings struct to a value it does
// not hold, so that something else can ask whether anything noticed.
//
// It exists for the axis sweep (#2031). An axis in interp.Semantics records a
// *measured disagreement between real shells*, so an axis that can be flipped
// with nothing failing is not an untested code path — it is a fact nobody
// checked, wearing the clothes of one that was. Finding those means flipping
// every field in turn and re-running the instruments, and flipping a field
// from outside the package that declares it means reflection.
//
// The package deliberately knows nothing about interp. It takes a pointer to
// any struct, walks a dotted path of field names, and writes a value parsed
// according to the field's kind. That keeps it importable from interp itself
// — the interpreter's own hook needs it, and an import back the other way
// would be a cycle — and it keeps the sweep's vocabulary (what a field is,
// which values are legal) in one place instead of two.
//
// Nothing here is reachable from a normal build. The hooks that call it sit
// behind the `shaxissweep` build tag; see interp/axissweep_on.go.
package axismutate

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// EnvVar names the environment variable the hooks read.
//
// An environment variable rather than a flag because the thing being mutated
// is a *library's* settings struct, and the processes that have to carry the
// mutation are a shell binary, a `go test` binary and anything else the sweep
// decides to point at. A flag would have to be added to each of them.
const EnvVar = "SH_AXIS_MUTATION"

// Spec is one mutation, written `Dotted.Field=value`.
//
// The value is read according to the field's kind: `true`/`false` for a bool,
// a decimal for any integer kind including a named one, and everything after
// the first `=` verbatim for a string. Named integer types are how nearly
// every non-boolean axis is spelled, and their constants are enumerated from
// the source by the sweep rather than listed here — see internal/axissweep.
type Spec struct {
	Path  string
	Value string
}

// ParseSpec reads one `Path=value`.
func ParseSpec(s string) (Spec, error) {
	name, value, ok := strings.Cut(s, "=")
	if !ok || name == "" {
		return Spec{}, fmt.Errorf("axis mutation %q: want Path=value", s)
	}
	return Spec{Path: name, Value: value}, nil
}

// String writes the spec back in the form ParseSpec reads.
func (s Spec) String() string { return s.Path + "=" + s.Value }

// FromEnv is the mutation this process was started with, if any.
func FromEnv() (Spec, bool) {
	raw := os.Getenv(EnvVar)
	if raw == "" {
		return Spec{}, false
	}
	spec, err := ParseSpec(raw)
	if err != nil {
		// A malformed spec is the sweep's own bug and must not read as a
		// shell that behaved oddly, so it stops the process rather than
		// running an unmutated shell under a mutated heading. Unmutated
		// output compared against an unmutated baseline is identical, which
		// is exactly the answer "nothing objected" — the sweep's one
		// unrecoverable misreading.
		fmt.Fprintf(os.Stderr, "%s: %v\n", EnvVar, err)
		os.Exit(2)
	}
	return spec, true
}

// ApplyEnv writes the process's mutation into v, or leaves it alone.
func ApplyEnv(v any) {
	spec, ok := FromEnv()
	if !ok {
		return
	}
	if err := Apply(v, spec); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", EnvVar, err)
		os.Exit(2)
	}
}

// Apply writes spec's value into the field spec names.
//
// v must be a pointer to a struct. The path is field names joined with dots,
// so a field of a nested struct is reachable — StartupFileOptions.Login is an
// axis as much as its neighbors are.
func Apply(v any, spec Spec) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("axis mutation %s: want a non-nil pointer to a struct, got %T", spec.Path, v)
	}
	field := rv.Elem()
	for _, name := range strings.Split(spec.Path, ".") {
		if field.Kind() != reflect.Struct {
			return fmt.Errorf("axis mutation %s: %q is not inside a struct", spec.Path, name)
		}
		next := field.FieldByName(name)
		if !next.IsValid() {
			return fmt.Errorf("axis mutation %s: no field named %q", spec.Path, name)
		}
		if !next.CanSet() {
			return fmt.Errorf("axis mutation %s: field %q cannot be set", spec.Path, name)
		}
		field = next
	}
	return set(field, spec)
}

// set writes one parsed value, by kind.
//
// The kinds here are exactly the ones interp.Semantics uses, and anything
// else is an error rather than a silent no-op. That is the whole discipline
// this package is in service of: a sweep that quietly skips what it does not
// understand reports the field as "nothing objected" and files a bug against
// a field it never touched.
func set(field reflect.Value, spec Spec) error {
	switch field.Kind() {
	case reflect.Bool:
		b, err := strconv.ParseBool(spec.Value)
		if err != nil {
			return fmt.Errorf("axis mutation %s: %q is not a bool", spec.Path, spec.Value)
		}
		field.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(spec.Value, 10, 64)
		if err != nil || field.OverflowInt(n) {
			return fmt.Errorf("axis mutation %s: %q is not a %s", spec.Path, spec.Value, field.Type())
		}
		field.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(spec.Value, 10, 64)
		if err != nil || field.OverflowUint(n) {
			return fmt.Errorf("axis mutation %s: %q is not a %s", spec.Path, spec.Value, field.Type())
		}
		field.SetUint(n)
	case reflect.String:
		field.SetString(spec.Value)
	default:
		return fmt.Errorf("axis mutation %s: nothing here knows how to write a %s", spec.Path, field.Kind())
	}
	return nil
}
