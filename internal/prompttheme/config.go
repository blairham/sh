// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

import (
	"maps"
	"slices"
	"strconv"
	"strings"
)

// The configuration namespace: one keyed store and one lookup, applied
// uniformly.
//
// A rich prompt is a per-segment color, background, icon, prefix, suffix and
// visibility, times several dozen segments. Carrying that as a Go struct with
// a field per knob would make a preset code rather than data, and would make
// every new segment cost configuration plumbing. A keyed store plus a
// fallback chain costs neither, which is why the spec calls this the
// structural decision the rest of the engine rests on.
//
// Settings are named SH_PROMPT_<KEY>, matching this tree's existing
// SH_BLOCKS_* and SH_WORD_SPLIT. Keys are held here without that prefix and
// upper-cased: DIR_FOREGROUND, LEFT_ELEMENTS, ICONS.

// Prefix is what a setting is spelled with outside this package — in a
// session's variables and at the head of a configuration file's line. It is
// accepted and stripped on the way in, so a line copied out of a session
// works in a file and back again.
const Prefix = "SH_PROMPT_"

// Key returns the canonical spelling of a setting's name: upper case, with
// the SH_PROMPT_ prefix accepted and removed.
func Key(name string) string {
	k := strings.ToUpper(strings.TrimSpace(name))
	return strings.TrimPrefix(k, Prefix)
}

// Value is one setting. A key holds either a scalar or a list, never both,
// and which one it holds is a property of how it was set rather than of how
// it is read: either can be read as the other, because dash has no arrays and
// a list still has to be expressible in a session running it.
type Value struct {
	text   string
	list   []string
	isList bool
}

// Scalar makes a scalar value. The empty string is a real value — an empty
// prefix and a suppressed icon are both configured states — and is distinct
// from the key being absent, which Settings.Has answers.
func Scalar(text string) Value { return Value{text: text} }

// ListOf makes a list value.
func ListOf(elements ...string) Value {
	return Value{list: slices.Clone(elements), isList: true}
}

// IsList reports whether the value was set as a list.
func (v Value) IsList() bool { return v.isList }

// Text reads the value as a scalar. A list read as a scalar is its elements
// joined by a space, which is the spelling a list is written in everywhere
// else in this namespace.
func (v Value) Text() string {
	if v.isList {
		return strings.Join(v.list, " ")
	}
	return v.text
}

// List reads the value as a list.
//
// A scalar is split on whitespace rather than returned as one element. The
// spec's sentence is "a scalar read as a list is a one-element list, since
// dash has no arrays and a list still has to be expressible there" — and only
// splitting makes the reason it gives true. A one-element list does not let a
// dash session name three elements; splitting does, and it is also how the
// configuration file already spells one. The spec is corrected alongside this
// package rather than implemented as written.
func (v Value) List() []string {
	if v.isList {
		return slices.Clone(v.list)
	}
	return strings.Fields(v.text)
}

// Layer is one place settings come from. The engine reads through an ordered
// stack of these, so that a value can say which layer it came from and not
// only what it is.
//
// A layer is not required to be enumerable. The session's own variables are
// reached one name at a time through a Runner's lookup, and that is the whole
// reason this is an interface rather than a map: a stack that could only hold
// maps would have to enumerate a namespace no shell offers to enumerate.
type Layer interface {
	// Name identifies the layer in what prompt show prints.
	Name() string

	// Lookup answers a canonical key. The bool distinguishes a setting that
	// is absent from one that is set to empty.
	Lookup(key string) (Value, bool)
}

// Store is an enumerable Layer: a preset, a configuration file, or a set of
// assignments a test wrote.
type Store struct {
	name     string
	values   map[string]Value
	problems []string
}

// NewStore returns an empty store that will name itself as layer.
func NewStore(layer string) *Store {
	return &Store{name: layer, values: map[string]Value{}}
}

// Name reports the layer's name.
func (s *Store) Name() string { return s.name }

// Set stores a value, replacing whatever the key held. Setting a scalar over
// a list and a list over a scalar are the same operation, because a key holds
// one or the other and never both.
func (s *Store) Set(key string, value Value) { s.values[Key(key)] = value }

// SetText is Set with a scalar.
func (s *Store) SetText(key, text string) { s.Set(key, Scalar(text)) }

// SetList is Set with a list.
func (s *Store) SetList(key string, elements ...string) { s.Set(key, ListOf(elements...)) }

// Lookup answers a canonical key.
func (s *Store) Lookup(key string) (Value, bool) {
	v, ok := s.values[Key(key)]
	return v, ok
}

// Keys returns every key the store holds, sorted.
func (s *Store) Keys() []string {
	return slices.Sorted(maps.Keys(s.values))
}

// Note records something that was read and could not be honored — a line
// that is not an assignment, a setting stored faithfully and not acted on.
// The engine reports these rather than dropping them: a person configured
// something, the tool accepted it, and the prompt drew something else is the
// silent-wrong-answer class this repository treats as its worst.
func (s *Store) Note(problem string) { s.problems = append(s.problems, problem) }

// Problems returns what Note recorded, in the order it was recorded.
func (s *Store) Problems() []string { return slices.Clone(s.problems) }

// varLayer reads settings out of somewhere that holds shell variables.
type varLayer struct {
	name string
	get  func(string) (string, bool)
}

// NewVars returns a Layer that reads SH_PROMPT_<KEY> through get.
//
// Through the shell's own variables rather than the process environment,
// which is the rule the block store and HISTFILE already follow and for the
// same reason: a session can set one at the prompt and mean it. It is also
// what makes configuration work identically in all five dialects, since every
// dialect has variables even where it has no hooks and no arrays.
func NewVars(layer string, get func(string) (string, bool)) Layer {
	return varLayer{name: layer, get: get}
}

func (v varLayer) Name() string { return v.name }

func (v varLayer) Lookup(key string) (Value, bool) {
	if v.get == nil {
		return Value{}, false
	}
	text, ok := v.get(Prefix + Key(key))
	if !ok {
		return Value{}, false
	}
	return Scalar(text), true
}

// Settings is the ordered stack of layers the engine reads through.
//
// Later wins over earlier, per key, shallow — the preset, then the
// configuration file, then what the session set. A list set by a later layer
// replaces the earlier list outright rather than merging, because "my
// elements are these" cannot mean anything else, and a stack that resolves
// per key rather than merging maps gets that property by construction.
type Settings struct {
	layers []Layer
}

// NewSettings returns a stack, earliest layer first.
func NewSettings(layers ...Layer) *Settings {
	return &Settings{layers: slices.Clone(layers)}
}

// Layers returns the stack, earliest first.
func (s *Settings) Layers() []Layer { return slices.Clone(s.layers) }

// Lookup resolves a key and names the layer that answered.
func (s *Settings) Lookup(key string) (value Value, layer string, ok bool) {
	k := Key(key)
	for i := len(s.layers) - 1; i >= 0; i-- {
		if v, found := s.layers[i].Lookup(k); found {
			return v, s.layers[i].Name(), true
		}
	}
	return Value{}, "", false
}

// Has reports whether the key is set at all, which is distinct from being set
// to empty.
func (s *Settings) Has(key string) bool {
	_, _, ok := s.Lookup(key)
	return ok
}

// Origin names the layer a key's value came from.
func (s *Settings) Origin(key string) (string, bool) {
	_, layer, ok := s.Lookup(key)
	return layer, ok
}

// Str reads a scalar setting, or def when absent.
func (s *Settings) Str(key, def string) string {
	if v, _, ok := s.Lookup(key); ok {
		return v.Text()
	}
	return def
}

// List reads a list setting. Absent reads as nil, which is distinct from a
// setting explicitly emptied — that reads as no elements.
func (s *Settings) List(key string) []string {
	if v, _, ok := s.Lookup(key); ok {
		return v.List()
	}
	return nil
}

// Bool reads a truth setting. true/false, 1/0, yes/no and on/off are all
// accepted, and anything else degrades to def.
func (s *Settings) Bool(key string, def bool) bool {
	v, _, ok := s.Lookup(key)
	if !ok {
		return def
	}
	return parseBool(v.Text(), def)
}

// Int reads a numeric setting, degrading to def when absent or unparsable.
// A malformed value is not a reason to stop drawing: a prompt is not the
// place to report a typo by leaving the line blank.
func (s *Settings) Int(key string, def int) int {
	v, _, ok := s.Lookup(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v.Text()))
	if err != nil {
		return def
	}
	return n
}

// Keys returns every key the enumerable layers hold, sorted and without
// duplicates.
//
// A layer that reads a session's variables one name at a time cannot be
// enumerated, so what this returns is what is written down rather than
// everything that would resolve. Callers that print a configuration say so
// rather than presenting this as the whole of it.
func (s *Settings) Keys() []string {
	var keys []string
	for _, layer := range s.layers {
		store, ok := layer.(interface{ Keys() []string })
		if !ok {
			continue
		}
		keys = append(keys, store.Keys()...)
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

// Param is the segment-scoped lookup, and the reason this is a keyed store.
// Every per-segment setting resolves through the same three steps, most
// specific first, then the caller's default:
//
//	SH_PROMPT_<SEGMENT>_<STATE>_<KEY>    DIR_NOT_WRITABLE_FOREGROUND
//	SH_PROMPT_<SEGMENT>_<KEY>            DIR_FOREGROUND
//	SH_PROMPT_<KEY>                      FOREGROUND
//
// An empty state drops the first step. That chain is why a preset can set one
// FOREGROUND and have every segment inherit it while any segment, or any
// single state of one segment, overrides — and why adding a segment costs no
// new configuration plumbing.
func (s *Settings) Param(segment, state, key, def string) string {
	if v, ok := s.param(segment, state, key); ok {
		return v.Text()
	}
	return def
}

// ParamSet reports whether any step of the chain is set, so a caller can tell
// explicitly empty from absent the way Has does for a plain key.
func (s *Settings) ParamSet(segment, state, key string) bool {
	_, ok := s.param(segment, state, key)
	return ok
}

// ParamList is Param read as a list.
func (s *Settings) ParamList(segment, state, key string) []string {
	if v, ok := s.param(segment, state, key); ok {
		return v.List()
	}
	return nil
}

// ParamBool is Param with the truth parsing of Bool.
func (s *Settings) ParamBool(segment, state, key string, def bool) bool {
	v, ok := s.param(segment, state, key)
	if !ok {
		return def
	}
	return parseBool(v.Text(), def)
}

// ParamInt is Param with the numeric parsing of Int.
func (s *Settings) ParamInt(segment, state, key string, def int) int {
	v, ok := s.param(segment, state, key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v.Text()))
	if err != nil {
		return def
	}
	return n
}

// ParamOrigin names the layer and the key that answered the chain, so that
// prompt show can report where a segment's color actually came from.
func (s *Settings) ParamOrigin(segment, state, key string) (name, layer string, ok bool) {
	for _, candidate := range ParamChain(segment, state, key) {
		if _, from, found := s.Lookup(candidate); found {
			return candidate, from, true
		}
	}
	return "", "", false
}

func (s *Settings) param(segment, state, key string) (Value, bool) {
	for _, candidate := range ParamChain(segment, state, key) {
		if v, _, ok := s.Lookup(candidate); ok {
			return v, true
		}
	}
	return Value{}, false
}

// ParamChain returns the lookup order Param walks, most specific first.
// Hyphens in a segment name become underscores, so a segment may be named
// either way in a configuration and resolve to the same settings.
func ParamChain(segment, state, key string) []string {
	seg := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(segment), "-", "_"))
	st := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(state), "-", "_"))
	k := Key(key)
	switch {
	case seg == "":
		return []string{k}
	case st == "":
		return []string{seg + "_" + k, k}
	default:
		return []string{seg + "_" + st + "_" + k, seg + "_" + k, k}
	}
}

// parseBool reads the truth spellings a configuration may use. An empty value
// is false rather than def: emptying a setting is an answer, and the answer a
// person emptying a flag means is off.
func parseBool(text string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off", "":
		return false
	}
	return def
}
