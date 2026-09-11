// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/blairham/sh/interp"
)

// The `zsh/stat` module: one system call reported fourteen ways.
//
// Measured 2026-09-09 against zsh 5.9.2 with `zsh -f`. The module is two names
// for one builtin — `zstat` and `stat` — and **only `zstat` is registered
// here**. That is a decision rather than an omission, and the manual makes the
// same one in the same words: "as the name stat is often used by an external
// command it is recommended that only the zstat form of the command is used."
//
// The reason is this shell's own shape. Its builtins are the dialect's and are
// registered before any script runs (see zmodload.go), so a `stat` registered
// at all is a `stat` registered *always* — and `stat -f %z file` in any script
// this shell runs would stop reaching /usr/bin/stat, on a machine where that
// command is the one everybody means. zsh does not have that problem because
// the module really is loaded on demand. Registering the name would buy
// literal agreement with `zmodload zsh/stat; stat f` and cost every script
// that never asked for the module, so `b:stat` is in the feature table as
// what it is — a feature of the module this shell has not got — and
// `zmodload -F zsh/stat b:stat` refuses by that name.
//
// Every real caller writes the narrowed form. Counted on this machine: three
// plugin trees name this module, and all three write `zmodload -F zsh/stat
// b:zstat`, which is the line the manual recommends and the line #1634 was
// filed on.

// statElements are the fields, in the order zsh lists them — which is the
// order `-l` writes, the order a listing writes, and the order an array is
// filled in, so all three read against each other.
//
// `link` is not a field of the system call. It is where the file points when
// the file is a link and `-L` was given, and empty otherwise, which is why a
// whole listing ends in a name with nothing after it.
var statElements = []string{
	"device", "inode", "mode", "nlink", "uid", "gid", "rdev", "size",
	"atime", "mtime", "ctime", "blksize", "blocks", "link",
}

// statFields is what one file's stat produced, in types that do not depend on
// the platform's own. The platform-shaped half is statPath and statFd, and
// there is a file per operating system for them.
type statFields struct {
	device  uint64
	inode   uint64
	mode    uint64
	nlink   uint64
	uid     uint64
	gid     uint64
	rdev    uint64
	size    int64
	atime   int64
	mtime   int64
	ctime   int64
	blksize int64
	blocks  int64
	link    string
}

// statTimeFormat is how `-s` writes the three times, measured against zsh
// 5.9.2 on a single-digit and a two-digit day of the month: `Thu Mar  5
// 9:00:07 EST 2026` and `Thu Sep 10  1:06:11 EDT 2026`. Both the day and the
// hour are space-padded rather than zero-padded, which is what makes the run
// of two spaces move between them.
const statTimeFormat = "%a %b %e %k:%M:%S %Z %Y"

// statColumn is the width the element name is written in when a whole listing
// names its fields — `blksize` is the longest at seven, so every other name is
// padded out to it and one space follows.
const statColumn = 7

func registerStatModule(r *interp.Runner) {
	if !statSupported {
		// A platform whose stat fields this package has not been taught.
		// Registering the name anyway would be the hollow module the whole
		// feature table exists to avoid: `zmodload -F zsh/stat b:zstat`
		// would answer 0 and the next line would call something that cannot
		// work. Left unregistered, the module refuses by the builtin's name.
		return
	}
	r.Register("zstat", zstatBuiltin)
}

// zstatOpts is what the letters asked for.
type zstatOpts struct {
	array    string
	hash     string
	toArray  bool
	toHash   bool
	fd       int
	useFd    bool
	format   string
	gmt      bool
	list     bool
	lstat    bool
	names    bool
	noNames  bool
	octal    bool
	raw      bool
	strings  bool
	types    bool
	noTypes  bool
	element  string
	selected bool
}

// stringly reports whether the elements that have a written form are to be
// shown in it. `-F` and `-g` are about *how* a time is written and so imply
// the letter that asks for it to be written at all — measured, `zstat -F %s
// +mtime` is the seconds and not the number.
func (o zstatOpts) stringly() bool { return o.strings || o.format != "" || o.gmt }

func zstatBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	opts, files, code := zstatOptions(r, args)
	if code != 0 {
		return code
	}
	switch {
	case opts.toArray && opts.toHash:
		r.Diagnosef("both array and hash requested\n")
		return 1
	case opts.list:
		// `-l` answers and returns; the manual says arguments and every
		// letter but `-A` are ignored, and that is measured — `zstat -l -H h`
		// writes the names to standard output.
		if opts.toArray {
			r.SetArray(opts.array, statElements)
			return 0
		}
		zstatPrintf(r, "%s\n", strings.Join(statElements, " "))
		return 0
	case opts.useFd && len(files) > 0:
		r.Diagnosef("no files allowed with -f\n")
		return 1
	case !opts.useFd && len(files) == 0:
		r.Diagnosef("no files given\n")
		return 1
	case opts.toHash && len(files) > 1:
		r.Diagnosef("only one file allowed with -H\n")
		return 1
	}
	return zstatReport(r, ctx, opts, files)
}

// zstatOptions reads the letters and the one `+element`.
//
// The element is read here rather than after the letters because that is where
// a real caller writes it: `zstat -A stat +mtime -- $1` and `zstat +mtime -A
// arr "$1"` are two lines from two plugin trees and they put it on either side.
// **Only the first is an element**; measured, `zstat +mtime +size f` complains
// that `+size` is not a file, so the second one has ended the options and is an
// operand.
func zstatOptions(r *interp.Runner, args []string) (opts zstatOpts, files []string, code int) {
	i := 0
	for ; i < len(args); i++ {
		word := args[i]
		switch {
		case word == "--":
			i++
			return opts, args[i:], 0
		case strings.HasPrefix(word, "+") && !opts.selected:
			name := word[1:]
			if name == "" {
				// A bare `+` selects nothing and is not a file: measured,
				// `zstat + f` writes the whole listing for `f` alone.
				continue
			}
			elem, why := zstatElement(name)
			if why != "" {
				r.Diagnosef("%s: %s\n", name, why)
				return opts, nil, 1
			}
			opts.element, opts.selected = elem, true
			if elem == "link" {
				// Measured, and the manual says so: selecting `link` turns on
				// the lstat that makes it anything but empty.
				opts.lstat = true
			}
		case len(word) > 1 && strings.HasPrefix(word, "-"):
			if code := zstatLetters(r, &opts, word, args, &i); code != 0 {
				return opts, nil, code
			}
		default:
			return opts, args[i:], 0
		}
	}
	return opts, nil, 0
}

// zstatLetters reads one option word, which may end in a letter that takes an
// argument — `-A array`, `-Aarray` and `-A` with nothing after it are all
// measured, and the third is a complaint naming what was wanted rather than
// the letter.
func zstatLetters(r *interp.Runner, opts *zstatOpts, word string, args []string, i *int) int {
	for j := 1; j < len(word); j++ {
		letter := word[j]
		switch letter {
		case 'A', 'H', 'f', 'F':
			value, ok := zstatLetterValue(word, args, i, j)
			if !ok {
				r.Diagnosef("%s\n", zstatWanted(letter))
				return 1
			}
			switch letter {
			case 'A':
				opts.array, opts.toArray = value, true
			case 'H':
				opts.hash, opts.toHash = value, true
			case 'F':
				opts.format = value
			case 'f':
				fd, err := strconv.Atoi(value)
				if err != nil {
					r.Diagnosef("%s: bad file descriptor\n", value)
					return 1
				}
				opts.fd, opts.useFd = fd, true
			}
			return 0
		case 'g':
			opts.gmt = true
		case 'l':
			opts.list = true
		case 'L':
			opts.lstat = true
		case 'n':
			opts.names = true
		case 'N':
			opts.noNames = true
		case 'o':
			opts.octal = true
		case 'r':
			opts.raw = true
		case 's':
			opts.strings = true
		case 't':
			opts.types = true
		case 'T':
			opts.noTypes = true
		default:
			r.Diagnosef("bad option: -%c\n", letter)
			return 1
		}
	}
	return 0
}

// zstatLetterValue is the argument of a letter that takes one: the rest of the
// word it is in, or the word after it.
func zstatLetterValue(word string, args []string, i *int, j int) (string, bool) {
	if j+1 < len(word) {
		return word[j+1:], true
	}
	if *i+1 >= len(args) {
		return "", false
	}
	*i++
	return args[*i], true
}

// zstatWanted is what a letter with nothing after it says it is short of.
// Measured one letter at a time, and they are three different sentences rather
// than one about the letter.
func zstatWanted(letter byte) string {
	switch letter {
	case 'f':
		return "missing file descriptor"
	case 'F':
		return "missing time format"
	}
	return "missing parameter name"
}

// zstatElement resolves a `+name` to one element, and says why when it cannot.
//
// A name may be shortened to any unique leading part — measured, `+mt` is
// `mtime` and `+m` is ambiguous between `mode` and `mtime`.
func zstatElement(name string) (elem, why string) {
	var found []string
	for _, e := range statElements {
		if e == name {
			return e, ""
		}
		if strings.HasPrefix(e, name) {
			found = append(found, e)
		}
	}
	switch len(found) {
	case 0:
		return "", "no such stat element"
	case 1:
		return found[0], ""
	}
	return "", "ambiguous stat element"
}

// zstatReport stats each file and writes what was asked for.
//
// A failure stops nothing that has already been written and everything that
// has not: measured, `zstat +mtime f nosuch` writes `f`'s line and complains
// about the other, while `zstat -A a +mtime f nosuch` leaves `a` untouched.
// The two are one rule seen twice — output happens as it goes and an
// assignment happens at the end — so a script reading the array never sees
// half an answer.
func zstatReport(r *interp.Runner, ctx context.Context, opts zstatOpts, files []string) int {
	showNames := opts.names || (!opts.noNames && !opts.toArray && !opts.toHash && len(files) > 1)
	if opts.noNames {
		showNames = false
	}
	showTypes := opts.types || (!opts.noTypes && !opts.toArray && !opts.toHash && !opts.selected)
	if opts.noTypes {
		showTypes = false
	}
	elements := statElements
	if opts.selected {
		elements = []string{opts.element}
	}

	var collected []string
	hash := map[string]string{}
	status := 0
	names := files
	if opts.useFd {
		names = []string{""}
	}
	for n, name := range names {
		fields, err := zstatOne(r, ctx, opts, name)
		if err != nil {
			r.Diagnosef("%s\n", err.Error())
			status = 1
			continue
		}
		switch {
		case opts.toArray:
			if showNames {
				collected = append(collected, name)
			}
			for _, e := range elements {
				collected = append(collected, zstatEntry(opts, fields, e, showTypes, false))
			}
		case opts.toHash:
			if showNames {
				hash["name"] = name
			}
			for _, e := range elements {
				hash[e] = zstatEntry(opts, fields, e, showTypes, !opts.selected)
			}
		default:
			if n > 0 && !opts.selected {
				// The blank line between two whole listings, which is there
				// even when the names above them are not: measured with `-N`.
				zstatPrintf(r, "\n")
			}
			if showNames && !opts.selected {
				zstatPrintf(r, "%s:\n", name)
			}
			for _, e := range elements {
				line := zstatEntry(opts, fields, e, showTypes, !opts.selected)
				if showNames && opts.selected {
					line = name + " " + line
				}
				zstatPrintf(r, "%s\n", line)
			}
		}
	}
	if status != 0 {
		return status
	}
	switch {
	case opts.toArray:
		r.SetArray(opts.array, collected)
	case opts.toHash:
		r.SetAssoc(opts.hash, hash)
	}
	return 0
}

// zstatOne is the stat itself: a descriptor when `-f` named one, otherwise a
// path, following links unless `-L` said not to.
func zstatOne(r *interp.Runner, ctx context.Context, opts zstatOpts, name string) (statFields, error) {
	if !opts.useFd {
		at := shellPath(r, name)
		// The gate first, and answered as "not there" when it refuses.
		// interp/fsgate.go opens by saying every probe in the shell comes
		// through the gate because a probe is an oracle, and this one was
		// outside that sentence until #1819: on one path in one script,
		// `[[ -f secret ]]` was refused into ENOENT while `zstat +size
		// secret` answered with the true size and mtime of the same file.
		//
		// Indistinguishable from a missing path on purpose. `zstat nosuch`
		// is already `nosuch: no such file or directory`, so a refusal
		// wearing those words tells a script nothing it could not have
		// learned by naming something that does not exist.
		// The bare errno, which is what statPath hands back for a path that
		// is not there: a *fs.PathError would print the resolved absolute
		// path inside the message and make the refusal recognizable at a
		// glance — and hand the script the very name the policy withheld.
		if !r.AllowProbe(ctx, at) {
			return statFields{}, fmt.Errorf("%s: %w", name, syscall.ENOENT)
		}
		f, err := statPath(at, !opts.lstat)
		if err != nil {
			return f, fmt.Errorf("%s: %w", name, err)
		}
		return f, nil
	}
	sys, ok := r.SystemDescriptor(opts.fd)
	if !ok {
		// The shell's number rather than the kernel's, because the shell's is
		// the one the script wrote. Measured: `zstat -f 33` with nothing open
		// there is `33: bad file descriptor`.
		return statFields{}, fmt.Errorf("%d: bad file descriptor", opts.fd)
	}
	f, err := statFd(sys)
	if err != nil {
		return f, fmt.Errorf("%d: %w", opts.fd, err)
	}
	return f, nil
}

// zstatEntry is one element as it will be written, with the element's own name
// in front of it when the type names are being shown.
//
// The padding is the odd half and it is measured rather than reasoned: a whole
// listing pads the name out to a column, on standard output and in a hash
// alike, and an array never pads. `zstat -A x -t f` is `device 16777230` and
// `zstat -H h -t f` leaves `mode    33188` under its key.
func zstatEntry(opts zstatOpts, f statFields, elem string, showTypes, column bool) string {
	value := zstatValue(opts, f, elem)
	switch {
	case !showTypes:
		return value
	case column:
		return fmt.Sprintf("%-*s %s", statColumn, elem, value)
	}
	return elem + " " + value
}

// zstatValue is one element's value, in the form the letters asked for.
//
// Three of them and only three: the number, the written form, or the number
// with the written form after it in brackets. `-r` is the third whether or not
// `-s` is there — measured, `zstat -r f` writes `33188 (-rw-r--r--)` — so the
// letter is not a modifier of `-s` but the request for both.
func zstatValue(opts zstatOpts, f statFields, elem string) string {
	raw := zstatRaw(opts, f, elem)
	written, has := zstatWritten(opts, f, elem)
	switch {
	case !has:
		return raw
	case opts.raw:
		return raw + " (" + written + ")"
	case opts.stringly():
		return written
	}
	return raw
}

// zstatRaw is the element as a number, which is the default form.
func zstatRaw(opts zstatOpts, f statFields, elem string) string {
	switch elem {
	case "device":
		return strconv.FormatUint(f.device, 10)
	case "inode":
		return strconv.FormatUint(f.inode, 10)
	case "mode":
		if opts.octal {
			// With the leading zero the letter's own description asks for, so
			// the number reads as the octal it is.
			return "0" + strconv.FormatUint(f.mode, 8)
		}
		return strconv.FormatUint(f.mode, 10)
	case "nlink":
		return strconv.FormatUint(f.nlink, 10)
	case "uid":
		return strconv.FormatUint(f.uid, 10)
	case "gid":
		return strconv.FormatUint(f.gid, 10)
	case "rdev":
		return strconv.FormatUint(f.rdev, 10)
	case "size":
		return strconv.FormatInt(f.size, 10)
	case "atime":
		return strconv.FormatInt(f.atime, 10)
	case "mtime":
		return strconv.FormatInt(f.mtime, 10)
	case "ctime":
		return strconv.FormatInt(f.ctime, 10)
	case "blksize":
		return strconv.FormatInt(f.blksize, 10)
	case "blocks":
		return strconv.FormatInt(f.blocks, 10)
	}
	return f.link
}

// zstatWritten is the element in words, and whether it has such a form at all.
// Six of the fourteen do — the mode, the two owners and the three times — and
// the rest are numbers however they are asked for.
func zstatWritten(opts zstatOpts, f statFields, elem string) (string, bool) {
	switch elem {
	case "mode":
		return statModeString(f.mode), true
	case "uid":
		if u, err := user.LookupId(strconv.FormatUint(f.uid, 10)); err == nil {
			return u.Username, true
		}
		return strconv.FormatUint(f.uid, 10), true
	case "gid":
		if g, err := user.LookupGroupId(strconv.FormatUint(f.gid, 10)); err == nil {
			return g.Name, true
		}
		return strconv.FormatUint(f.gid, 10), true
	case "atime":
		return statTimeString(opts, f.atime), true
	case "mtime":
		return statTimeString(opts, f.mtime), true
	case "ctime":
		return statTimeString(opts, f.ctime), true
	}
	return "", false
}

// statTimeString writes one time, in the zone `-g` chose and the format `-F`
// gave — the same format language `strftime` speaks here, extensions and all,
// which is what makes `-F %s.%N` the nanosecond timestamp the manual promises.
func statTimeString(opts zstatOpts, sec int64) string {
	t := time.Unix(sec, 0)
	if opts.gmt {
		t = t.UTC()
	}
	format := opts.format
	if format == "" {
		format = statTimeFormat
	}
	return zshStrftime(format, t)
}

// statModeString is the mode as `ls -l` writes it: the type in front and three
// triples after it, with the set-user, set-group and sticky bits folded into
// the execute letter of their own triple — upper case where the execute bit
// they replace was not set, which is the distinction `-rw-r--r-T` carries.
func statModeString(mode uint64) string {
	out := []byte(statFileType(mode))
	const (
		setuid = 0o4000
		setgid = 0o2000
		sticky = 0o1000
	)
	special := []uint64{setuid, setgid, sticky}
	extra := []byte{'s', 's', 't'}
	for triple := range 3 {
		shift := 6 - 3*triple
		bits := mode >> shift
		for bit, letter := range []byte{'r', 'w', 'x'} {
			if bits&(1<<(2-uint(bit))) != 0 {
				out = append(out, letter)
				continue
			}
			out = append(out, '-')
		}
		if mode&special[triple] == 0 {
			continue
		}
		last := len(out) - 1
		if out[last] == 'x' {
			out[last] = extra[triple]
			continue
		}
		out[last] = extra[triple] - 'a' + 'A'
	}
	return string(out)
}

// statFileType is the first letter of the mode string.
func statFileType(mode uint64) string {
	const typeMask = 0o170000
	switch mode & typeMask {
	case 0o140000:
		return "s"
	case 0o120000:
		return "l"
	case 0o100000:
		return "-"
	case 0o060000:
		return "b"
	case 0o040000:
		return "d"
	case 0o020000:
		return "c"
	case 0o010000:
		return "p"
	}
	return "?"
}

func zstatPrintf(r *interp.Runner, format string, args ...any) {
	_, _ = fmt.Fprintf(r.Out(), format, args...)
}
