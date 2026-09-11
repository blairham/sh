// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Reading a compiled terminal description, and finding the one `$TERM` names.
//
// A description is a small binary file: a header of six 16-bit counts, the
// terminal's own names, then three arrays — a byte per boolean capability, a
// number per numeric one, an offset per string — followed by the table those
// offsets point into. Nothing in the file says which capability a slot
// belongs to; the position is the name, and terminfonames.go is the mapping.
//
// Two magic numbers are in use and they differ in one thing: the older format
// stores numbers as signed 16-bit, the newer as signed 32-bit, so a terminal
// claiming more than 32767 of something can say so. Both are read here
// because both are on disk — a database built by a current `tic` holds a
// mixture.
//
// An optional **extended** section follows, and it is where a modern
// description keeps everything the standard capability set has no slot for:
// `Se` and `Ss` for the cursor shape, `Smulx` for curly underlines, the
// modified-arrow keys `kUP3` through `kRIT7`. It carries its own names, so it
// needs no table here.
//
// # Anything unreadable is a terminal this shell does not know
//
// Every failure in this file is the same answer — no description — because
// that is the answer a script can act on and the one real zsh gives. Measured
// against zsh 5.9.2 under a `$TERM` with no entry anywhere: `${+terminfo}` is
// 1 and `${#terminfo}` is 0, so the parameter exists and holds nothing. A
// truncated file, a foreign magic number and a name the database has never
// heard of are indistinguishable to a caller, and should be: each means this
// shell cannot say what the terminal does.

// The two magic numbers a compiled description begins with. The second is the
// format that widened numbers from 16 to 32 bits; everything else about the
// two is identical, which is why one reader handles both.
const (
	terminfoMagic     = 0o432
	terminfoWideMagic = 0o1036
)

// A capability is absent when its stored value is negative, and the format
// distinguishes two reasons — never set, and cancelled by a description that
// builds on another. Both read as absent here, which is what a script sees:
// zsh answers `${+terminfo[U8]}` with 0 for `xterm-256color`, whose extended
// numeric `U8` is stored cancelled.
const terminfoAbsent = -1

// errNoDescription is every failure this file has, for the reason given
// above: a caller can do nothing different with a truncated file than with a
// missing one.
var errNoDescription = errors.New("no terminal description")

// terminfoSystemDirectories is where a terminfo database lives when nothing
// in the environment says otherwise.
//
// Searched in order and missing ones cost nothing — the read fails and the
// next is tried. The list is longer than any one system uses because the
// shell is one binary: a macOS install has only the first, a Debian one has
// the second and third, and a description found in either is the same file
// format.
var terminfoSystemDirectories = []string{
	"/usr/share/terminfo",
	"/etc/terminfo",
	"/lib/terminfo",
	"/usr/lib/terminfo",
	"/usr/share/lib/terminfo",
	"/usr/local/share/terminfo",
	// Homebrew's ncurses is keg-only, so its database — the one its own
	// programs compile against — is under the keg rather than the prefix.
	// `kitty` and `ghostty` are there and in no system directory on this
	// machine, which is a terminal a person is plausibly running this shell
	// inside.
	"/opt/homebrew/share/terminfo",
	"/opt/homebrew/opt/ncurses/share/terminfo",
	"/usr/local/opt/ncurses/share/terminfo",
}

// terminfoDirectories is the database search path, in the order a
// description is looked for.
//
// `$TERMINFO` first, then the personal database, then `$TERMINFO_DIRS` — in
// which an empty entry stands for the system list, which is how a caller says
// "mine, then the usual ones" — and the system list last whether or not
// `$TERMINFO_DIRS` named it.
func terminfoDirectories(env func(string) string) []string {
	var dirs []string
	if d := env("TERMINFO"); d != "" {
		dirs = append(dirs, d)
	}
	if home := env("HOME"); home != "" {
		dirs = append(dirs, filepath.Join(home, ".terminfo"))
	}
	for _, d := range filepath.SplitList(env("TERMINFO_DIRS")) {
		if d == "" {
			dirs = append(dirs, terminfoSystemDirectories...)
			continue
		}
		dirs = append(dirs, d)
	}
	return append(dirs, terminfoSystemDirectories...)
}

// terminfoEntryPaths is where one terminal's description would be inside one
// database directory.
//
// Two spellings, because two are in use: a subdirectory named for the first
// character of the terminal's name, and one named for that character's value
// in hexadecimal. The second exists so that a database can hold `ANSI` and
// `ansi` on a filesystem that cannot tell those directories apart, and it is
// what macOS ships — /usr/share/terminfo/78/xterm-256color, `78` being `x`.
func terminfoEntryPaths(dir, term string) []string {
	return []string{
		filepath.Join(dir, term[:1], term),
		filepath.Join(dir, fmt.Sprintf("%02x", term[0]), term),
	}
}

// terminalNameIsOneComponent says whether a `$TERM` may be looked up at all.
//
// A terminal name is a file name inside a database directory and nothing
// else, so it is one path component: no separator, no `.` or `..`, and
// nothing beginning with a dot. This is what keeps the search over directories
// a script can name from becoming a read of a path a script can name — see
// the exemption in internal/boundary for the whole of that argument.
func terminalNameIsOneComponent(term string) bool {
	if term == "" || term == "." || term == ".." || term[0] == '.' {
		return false
	}
	return !bytes.ContainsAny([]byte(term), `/\`)
}

// readTerminalDescription is the database read.
//
// Named rather than inlined because internal/boundary's guard names the
// function that opens a file, and this is the one: the path is
// `<database>/<x>/<name>` with the name checked to be a single component, and
// the whole argument for why it sits outside the boundary is written there.
func readTerminalDescription(path string) ([]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // see internal/boundary's repl.readTerminalDescription
	if err != nil {
		return nil, errNoDescription
	}
	return data, nil
}

// terminfoReader walks a compiled description, refusing to read past its end.
//
// Every field is read through it, including the counts in the header, so a
// truncated or hostile file runs out of bytes rather than indexing into
// whatever follows.
type terminfoReader struct {
	data []byte
	at   int
}

// next takes n bytes, or says the file ended.
func (r *terminfoReader) next(n int) ([]byte, bool) {
	if n < 0 || r.at+n > len(r.data) {
		return nil, false
	}
	out := r.data[r.at : r.at+n]
	r.at += n
	return out, true
}

// shorts takes n signed 16-bit values, little-endian, which is what the
// format uses for every count and every string offset.
func (r *terminfoReader) shorts(n int) ([]int, bool) {
	raw, ok := r.next(2 * n)
	if !ok {
		return nil, false
	}
	out := make([]int, n)
	for i := range out {
		out[i] = int(int16(uint16(raw[2*i]) | uint16(raw[2*i+1])<<8))
	}
	return out, true
}

// numbers takes n numeric capabilities, in whichever width the magic number
// said.
func (r *terminfoReader) numbers(n int, wide bool) ([]int, bool) {
	if !wide {
		return r.shorts(n)
	}
	raw, ok := r.next(4 * n)
	if !ok {
		return nil, false
	}
	out := make([]int, n)
	for i := range out {
		out[i] = int(int32(uint32(raw[4*i]) | uint32(raw[4*i+1])<<8 |
			uint32(raw[4*i+2])<<16 | uint32(raw[4*i+3])<<24))
	}
	return out, true
}

// align steps to the next even offset, which the format requires before every
// array of numbers: the booleans are bytes and the numbers that follow them
// are not.
func (r *terminfoReader) align() {
	if r.at%2 == 1 {
		r.at++
	}
}

// terminfoStringAt reads the NUL-terminated string an offset points at.
func terminfoStringAt(table []byte, off int) (string, bool) {
	if off < 0 || off >= len(table) {
		return "", false
	}
	end := bytes.IndexByte(table[off:], 0)
	if end < 0 {
		return "", false
	}
	return string(table[off : off+end]), true
}

// terminfoStringIndex is where a string capability sits in the string array.
//
// Two of them are needed by name rather than by position, because two
// booleans are answered from a string rather than from their own slot.
func terminfoStringIndex(name string) int {
	for i, n := range terminfoStringNames {
		if n.terminfo == name {
			return i
		}
	}
	return -1
}

// The two string slots the derived booleans below are read out of.
var (
	terminfoBackspaceIndex = terminfoStringIndex("cub1")
	terminfoNewlineIndex   = terminfoStringIndex("nel")
)

// terminfoDerivedBooleans is the pair of boolean capabilities that are not
// read from the boolean array, and what each is read from instead.
//
// Both are termcap's, both are answered by real zsh from a *string*
// capability, and both are wrong in real descriptions if the stored bit is
// believed. See terminfoHasBackspace and terminfoNewlineIsLinefeed.
var terminfoDerivedBooleans = map[string]struct {
	index int
	want  string
	// storedWins says whether the bit in the boolean array is the answer
	// when the string it is derived from is absent. Measured, and the two
	// differ: `OTbs` falls back to it and `OTNL` does not.
	storedWins bool
}{
	"OTbs": {index: terminfoBackspaceIndex, want: "\b", storedWins: true},
	"OTNL": {index: terminfoNewlineIndex, want: "\n", storedWins: false},
}

// terminfoDerivedBoolean answers one of the two booleans that are not read
// from the boolean array the way every other boolean is.
//
// `OTbs` is termcap's `bs` — "moving the cursor back one column is a
// backspace" — and `OTNL` is termcap's `NL` — "a linefeed moves to the next
// line". Measured against zsh 5.9.2, each is derived from the string
// capability it describes and the stored bit loses. Synthetic descriptions,
// compiled with `tic` and read back, 2026-09-11:
//
//	cub1=\b                    OTbs=yes
//	cub1=\E[D                  OTbs=no
//	OTbs set, cub1=\E[D        OTbs=no    — the stored bit loses
//	OTbs set, no cub1          OTbs=yes   — and wins when nothing overrules it
//	nel=\n                     OTNL=yes
//	OTNL set, no nel           OTNL=no    — which is the difference between
//	                                        the two, and is why storedWins is
//	                                        a field rather than always true
//
// It is not a curiosity. Reading the slot gives the wrong answer on real
// terminals in both directions: `ansi` stores `OTbs` and spells `cub1` as
// `\E[D`, while `linux`, `hpterm`, `sun`, `aixterm`, `cygwin` and `putty` all
// leave the bit clear and spell `cub1` as a backspace.
func terminfoDerivedBoolean(name string, stored byte, table []byte, offsets []int) (string, bool) {
	from, ok := terminfoDerivedBooleans[name]
	if !ok {
		return "", false
	}
	fallback := from.storedWins && stored == 1
	if from.index < 0 || from.index >= len(offsets) {
		return terminfoBoolean(boolByte(fallback)), true
	}
	value, present := terminfoStringAt(table, offsets[from.index])
	if !present {
		return terminfoBoolean(boolByte(fallback)), true
	}
	return terminfoBoolean(boolByte(value == from.want)), true
}

// boolByte is the stored spelling of a decision this file has just made, so
// that one function turns a boolean into `yes` or `no`.
func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// terminfoBoolean is how a stored boolean reads.
//
// `yes` and `no`, which is the wording measured out of zsh 5.9.2 rather than
// this file's choice, and anything that is not a stored 1 is `no` — the
// format's cancelled marker included, because a cancelled capability is one
// the terminal does not have.
func terminfoBoolean(v byte) string {
	if v == 1 {
		return "yes"
	}
	return "no"
}

// parseTerminalDescription reads one compiled description into capabilities.
func parseTerminalDescription(data []byte) ([]TerminalCapability, error) {
	rd := &terminfoReader{data: data}
	header, ok := rd.shorts(6)
	if !ok {
		return nil, errNoDescription
	}
	magic, nameLen, boolCount, numCount, strCount, tableLen :=
		header[0], header[1], header[2], header[3], header[4], header[5]
	wide := magic == terminfoWideMagic
	if magic != terminfoMagic && !wide {
		return nil, errNoDescription
	}
	// The terminal's own names and aliases, which are not a capability: the
	// description was found by name, so nothing here needs them.
	if _, ok := rd.next(nameLen); !ok {
		return nil, errNoDescription
	}
	bools, ok := rd.next(boolCount)
	if !ok {
		return nil, errNoDescription
	}
	rd.align()
	nums, ok := rd.numbers(numCount, wide)
	if !ok {
		return nil, errNoDescription
	}
	offsets, ok := rd.shorts(strCount)
	if !ok {
		return nil, errNoDescription
	}
	table, ok := rd.next(tableLen)
	if !ok {
		return nil, errNoDescription
	}

	caps := make([]TerminalCapability, 0, len(terminfoBooleanNames)+numCount+strCount)
	// Every boolean name answers, whether the description stores it or not,
	// and a name past the stored count is `no`. Measured: zsh's `$terminfo`
	// under `TERM=dumb` is 50 keys of which 44 are booleans, and
	// `xterm-256color` stores 38 of them while all 44 still read.
	for i, name := range terminfoBooleanNames {
		stored := byte(0)
		if i < len(bools) {
			stored = bools[i]
		}
		value := terminfoBoolean(stored)
		if derived, ok := terminfoDerivedBoolean(name.terminfo, stored, table, offsets); ok {
			value = derived
		}
		caps = append(caps, TerminalCapability{
			Terminfo: name.terminfo, Termcap: name.termcap, Value: value,
		})
	}
	for i, v := range nums {
		if i >= len(terminfoNumberNames) || v <= terminfoAbsent {
			continue
		}
		name := terminfoNumberNames[i]
		caps = append(caps, TerminalCapability{
			Terminfo: name.terminfo, Termcap: name.termcap, Value: strconv.Itoa(v),
		})
	}
	for i, off := range offsets {
		if i >= len(terminfoStringNames) {
			continue
		}
		name := terminfoStringNames[i]
		if name.terminfo == "" {
			continue
		}
		value, ok := terminfoStringAt(table, off)
		if !ok {
			continue
		}
		caps = append(caps, TerminalCapability{
			Terminfo: name.terminfo, Termcap: name.termcap, Value: value,
		})
	}
	rd.align()
	return appendExtendedCapabilities(caps, rd, wide), nil
}

// appendExtendedCapabilities reads the section a description keeps its
// non-standard capabilities in, if it has one.
//
// The layout is the main one again — booleans, numbers, string offsets, table
// — with the names carried alongside: the offset array holds one offset per
// extended string *value* followed by one per *name*, in boolean, numeric,
// string order. The name offsets are relative to the end of the values rather
// than to the table, which is the one thing about this section that has to be
// measured rather than assumed, and was.
//
// A description without the section, or with a damaged one, keeps everything
// read so far. There is nothing to report: the standard capabilities are
// entirely usable without `Smulx`.
func appendExtendedCapabilities(caps []TerminalCapability, rd *terminfoReader, wide bool) []TerminalCapability {
	header, ok := rd.shorts(5)
	if !ok {
		return caps
	}
	boolCount, numCount, strCount, tableLen := header[0], header[1], header[2], header[4]
	if boolCount < 0 || numCount < 0 || strCount < 0 {
		return caps
	}
	bools, ok := rd.next(boolCount)
	if !ok {
		return caps
	}
	rd.align()
	nums, ok := rd.numbers(numCount, wide)
	if !ok {
		return caps
	}
	values, ok := rd.shorts(strCount)
	if !ok {
		return caps
	}
	nameOffsets, ok := rd.shorts(boolCount + numCount + strCount)
	if !ok {
		return caps
	}
	table, ok := rd.next(tableLen)
	if !ok {
		return caps
	}
	// Where the names begin: past the last byte any value occupies.
	base := 0
	for _, off := range values {
		value, ok := terminfoStringAt(table, off)
		if !ok {
			continue
		}
		if end := off + len(value) + 1; end > base {
			base = end
		}
	}
	if base > len(table) {
		return caps
	}
	names := table[base:]
	for i, off := range nameOffsets {
		name, ok := terminfoStringAt(names, off)
		if !ok || name == "" {
			continue
		}
		entry := TerminalCapability{Terminfo: name, Termcap: "", Value: ""}
		switch {
		case i < boolCount:
			entry.Value = terminfoBoolean(bools[i])
		case i < boolCount+numCount:
			if nums[i-boolCount] <= terminfoAbsent {
				continue
			}
			entry.Value = strconv.Itoa(nums[i-boolCount])
		default:
			value, ok := terminfoStringAt(table, values[i-boolCount-numCount])
			if !ok {
				continue
			}
			entry.Value = value
		}
		caps = append(caps, entry)
	}
	return caps
}
