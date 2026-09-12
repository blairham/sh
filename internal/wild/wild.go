// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package wild sweeps the shell scripts installed on the machine and reports
// which of them this parser cannot read.
//
// It exists because the corpus cannot find this class of bug. Every case in
// the corpus is a snippet someone wrote to pin one behavior down, so the
// corpus proves that what it *asks about* is right and says nothing about
// what nobody thought to ask. Positional parameters were missing entirely
// while the corpus stood at 100%: it invokes everything with `-c` and no
// operands, so `$#` is legitimately 0 in every case it has.
//
// Scripts in the wild ask different questions, in bulk, and they were written
// without knowing this implementation exists. Three real gaps came out of one
// afternoon of running them by hand — which is three more than the corpus
// found, and the reason this is a target rather than a habit.
package wild

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// Result is what one script did.
type Result struct {
	Path string
	// Err is why our parser refused it, or nil.
	Err error
	// NotShell records that the reference shell refused it too, which means
	// the file is not the shell script its shebang claims — a Tcl or Perl
	// program that execs its real interpreter from the first line is the
	// usual case, and those must not be counted against us.
	NotShell bool
	// Line is the source line the failure was on, and Text that line.
	//
	// Kept because a report of causes needs something a person can look at:
	// the sweep can say what forty scripts have in common and cannot say why
	// it matters, and a line of real script is the shortest thing that can.
	Line int
	Text string
}

// Report is a whole sweep.
type Report struct {
	Scanned  int
	Parsed   int
	NotShell int
	// Skipped counts, by reason, the files the sweep refused to open because
	// CLEANROOM.md forbids reading them — another shell's own distribution,
	// another project's test data. They are counted so the report stays
	// honest, and never listed, because printing a path invites the reader
	// to open it. Skipped files are not part of Scanned: without opening
	// them, whether they are shell scripts at all is unknowable.
	Skipped  map[string]int
	Failures []Result
}

// DefaultDirs are where a machine keeps its scripts. Missing ones are skipped,
// so the same list serves a mac and a container.
//
// The bin directories are where a script a person *runs* lives. The rest are
// where a script a person runs *reaches*: /etc holds a system's own, and the
// libexec trees hold the helpers a program keeps for itself — git's are the
// worked example, a few dozen shell scripts nobody ever types the name of and
// every one of which runs on an ordinary working day.
var DefaultDirs = []string{
	"/bin", "/usr/bin", "/usr/sbin", "/usr/local/bin", "/usr/local/sbin",
	"/opt/homebrew/bin", "/opt/homebrew/sbin",
	"/etc", "/usr/libexec", "/opt/homebrew/opt",
}

// DirsVar names the environment variable that adds roots to a sweep, as a
// list separated like PATH.
//
// The roots this is for are the plugin and framework trees a shell sources at
// startup, and they are the population DefaultDirs does not hold: a bin
// directory holds programs a person *runs*, and a framework tree holds the
// code a person's shell reads before it draws a prompt. Every daily-driver
// construct found so far lives in the second, and `make wild` said "0
// failures" on a day one held twenty-two parse failures, because it was never
// looking there.
//
// The environment rather than a constant here, and that is the whole point of
// the variable. There is no portable place a framework tree lives — the path
// is one machine's, one plugin manager's and one person's — and baking this
// machine's layout into a binary is the argument already lost once elsewhere
// in this tree. An absent root is skipped, so a machine that has none, and
// every CI runner, sweeps exactly what it swept before.
const DirsVar = "SH_WILD_DIRS"

// FrameworkDepth is how far below a configured root the sweep descends.
//
// Deeper than DefaultDepth because the shapes differ. A bin directory is flat
// and its helpers sit a few levels down, while a framework tree is a checkout
// per plugin: a plugin manager's layout reaches a plugin's own library
// directory at five or six, and stopping at four reads the top of each
// checkout and none of the code below it.
const FrameworkDepth = 8

// DirsFrom is the roots configured in the environment, in order, with empty
// entries dropped.
//
// The lookup is passed in rather than read from the process, which is the rule
// the rest of the tree follows: a package that reached for os.Getenv would be
// answering for whichever process it happened to be linked into.
// Set-but-empty and unset are the same answer here, so the lookup's second
// result is not consulted: an empty list of roots is no roots either way.
// Elsewhere in the tree that distinction carries meaning — an empty HISTFILE
// is a person saying do not remember this session — and here there is nothing
// for it to say.
func DirsFrom(get func(string) (string, bool)) []string {
	value, _ := get(DirsVar)
	var dirs []string
	for _, dir := range strings.Split(value, string(os.PathListSeparator)) {
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// Absent is the roots that are not directories on this machine.
//
// A missing root is not an error — that is what lets one variable serve a
// laptop and a CI runner — but it is worth *saying*, because the failure a
// silent skip hides is a typo, and a sweep that quietly covered nothing would
// report the same "0 failures" this variable exists to stop being misread.
func Absent(dirs []string) []string {
	var absent []string
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			absent = append(absent, dir)
		}
	}
	return absent
}

// BashScope and ZshScope are the shebangs each dialect is answerable for.
//
// `sh` belongs to the bash scope on the machines this runs on rather than by
// argument: a `#!/bin/sh` script is written to the common denominator, which
// is the core, and grading it against bash asks the question the file was
// written to answer. Nothing claims `#!/bin/sh` *is* bash.
var (
	BashScope = map[string]bool{"sh": true, "bash": true}
	ZshScope  = map[string]bool{"zsh": true}
)

// DefaultDepth is how far below a named directory the sweep descends.
//
// Deep enough to reach a helper directory a package keeps its scripts in —
// /opt/homebrew/opt/git/libexec/git-core is four — and shallow enough that
// pointing the sweep at a home directory does not walk a source tree for a
// minute. A caller that wants the whole subtree says so.
const DefaultDepth = 4

// Scope is what one sweep looks at: where to look, which shebangs count, and
// how far down to go.
//
// It is a struct rather than three parameters because the three move together.
// A zsh sweep is not a bash sweep with a different dialect — it looks for
// different files, and reading the same set twice would grade zsh on scripts
// that never claimed to be zsh.
type Scope struct {
	// Dirs are the roots. Missing ones are skipped.
	Dirs []string
	// Shells are the interpreter base names that count. Empty means BashScope.
	Shells map[string]bool
	// Depth is how far below each root to descend; 0 means DefaultDepth and a
	// negative number means the roots themselves and nothing under them.
	Depth int
}

func (s Scope) shells() map[string]bool {
	if len(s.Shells) == 0 {
		return BashScope
	}
	return s.Shells
}

func (s Scope) depth() int {
	if s.Depth == 0 {
		return DefaultDepth
	}
	if s.Depth < 0 {
		return 0
	}
	return s.Depth
}

// Sweep parses every shell script it finds and grades this parser against the
// reference for the ones it refuses.
//
// The reference is what makes the result trustworthy: a file whose first line
// says `#!/bin/sh` is not necessarily shell, and refusing one that the
// reference also refuses says nothing about us.
func Sweep(ctx context.Context, scope Scope, dialect syntax.Dialect, reference string) Report {
	var rep Report
	paths, skipped := Find(scope)
	rep.Skipped = skipped
	for _, path := range paths {
		rep.Scanned++
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if _, perr := syntax.Parse(string(src), dialect); perr == nil {
			rep.Parsed++
			continue
		} else if !accepts(ctx, reference, path) {
			rep.NotShell++
			continue
		} else {
			res := Result{Path: path, Err: perr}
			var e *syntax.Error
			if asParseError(perr, &e) {
				res.Line = int(e.Pos.Line)
				res.Text = LineAt(string(src), int(e.Pos.Line))
			}
			rep.Failures = append(rep.Failures, res)
		}
	}
	sort.Slice(rep.Failures, func(i, j int) bool { return rep.Failures[i].Path < rep.Failures[j].Path })
	return rep
}

// Find is every readable regular file in the scope whose first line names one
// of its shells, plus a count, by reason, of the files it declined to open
// because CLEANROOM.md forbids reading them.
//
// The denial happens before the shebang is read — reading the shebang is
// reading the file — so a skipped count is of regular files, not of shell
// scripts: whether a file nobody may open is a script is unknowable, and
// counting all of them is the honest answer.
//
// A denied *directory* is not descended into either, and its files are counted
// where the walk stops rather than one by one: the point of the denial is that
// nothing under it is opened, and a count of a tree nobody may read is a count
// of files, which is what os.ReadDir can say without opening one.
func Find(scope Scope) (paths []string, skipped map[string]int) {
	skipped = map[string]int{}
	// The tree the sweep runs from is this project's own, so its testdata is
	// never mistaken for someone else's.
	own, _ := os.Getwd()
	shells := scope.shells()
	// Symbolic links are followed, files and directories alike, because on a
	// machine with a package manager almost everything worth sweeping is one:
	// /opt/homebrew/bin is a directory of links to files, and
	// /opt/homebrew/opt is a directory of links to directories. Refusing to
	// follow the second kind left the whole tree unread — git keeps its two
	// dozen shell helpers behind exactly that link.
	//
	// What that costs is a walk that can meet the same file twice or go round
	// forever, so the sweep remembers where it has been by the *resolved*
	// path. That is the identity that matters anyway: two links to one script
	// are one script, and counting it twice would put a phantom in every total.
	visited := map[string]bool{}
	var walk func(dir string, left int)
	walk = func(dir string, left int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			path := filepath.Join(dir, e.Name())
			// Stat rather than the entry's own type, so that a link is judged
			// by what it points at.
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			real, err := filepath.EvalSymlinks(path)
			if err != nil {
				continue
			}
			if visited[real] {
				continue
			}
			visited[real] = true
			if info.IsDir() {
				if reason := either(DeniedDir, path, real, own); reason != "" {
					skipped[reason] += countRegular(real)
					continue
				}
				if left > 0 {
					walk(path, left-1)
				}
				continue
			}
			if !info.Mode().IsRegular() {
				continue
			}
			if reason := either(Denied, path, real, own); reason != "" {
				skipped[reason]++
				continue
			}
			if shells[shellOf(path)] {
				paths = append(paths, path)
			}
		}
	}
	for _, dir := range scope.Dirs {
		// A root is judged by the same rule as anything the walk reaches.
		// Naming a denied tree on the command line is still naming a denied
		// tree, and the sweep may not read it because it was asked to.
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			continue
		}
		if reason := either(DeniedDir, dir, real, own); reason != "" {
			skipped[reason] += countRegular(real)
			continue
		}
		walk(dir, scope.depth())
	}
	sort.Strings(paths)
	return paths, skipped
}

// either applies a denial rule to both the path the walk arrived by and the
// path it resolves to, and reports the first refusal.
//
// Both, because a link crosses the boundary in either direction:
// /opt/homebrew/bin/bashbug is an innocent-looking name in a directory the
// sweep is meant to read, and it points into a shell's own distribution. The
// rule reads paths and never contents, so asking it twice costs nothing.
func either(rule func(path, own string) string, path, real, own string) string {
	if reason := rule(path, own); reason != "" {
		return reason
	}
	return rule(real, own)
}

// countRegular is how many regular files a denied path stands for: one when it
// is a file, and the whole subtree when it is a directory.
//
// The subtree is counted by listing directories, which never opens a file —
// the one operation the denial forbids. Links are not followed here, unlike in
// the walk: a count is not worth a loop, and a link inside a denied tree
// points at something that is either in the same tree and counted already or
// outside it and not this number's business.
func countRegular(path string) int {
	info, err := os.Lstat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		if info.Mode().IsRegular() {
			return 1
		}
		return 0
	}
	n := 0
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		n += countRegular(filepath.Join(path, e.Name()))
	}
	return n
}

// shellOf reads the shebang and answers with the interpreter's base name, or
// "" for a file that is not a script. A symbolic link is followed, which is
// how most of these are installed.
func shellOf(path string) string {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	var head [128]byte
	n, _ := f.Read(head[:])
	line, _, _ := strings.Cut(string(head[:n]), "\n")
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "#!") {
		if name := autoloaded(line); name != "" {
			return name
		}
		return named(path)
	}
	// `#!/bin/sh`, `#!/bin/bash -e`, `#!/usr/bin/env bash`. The interpreter
	// is the last path component of the first word, or the word after `env`.
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) == 0 {
		return ""
	}
	name := filepath.Base(fields[0])
	if name == "env" && len(fields) > 1 {
		name = filepath.Base(fields[1])
	}
	return name
}

// shellExtensions are the file names that say what a file is when nothing
// inside it does.
//
// The third convention, and the one that carries a framework tree. A plugin's
// code is neither run by the kernel nor autoloaded by the shell — it is
// *sourced*, by a line in somebody's startup file, so it needs no shebang and
// declares nothing, and the name is all there is. Pointing the sweep at a real
// plugin tree without this found 41 files in a tree holding 123 `.zsh`, which
// is a root added and the population still missing: the parse failures that
// prompted the root were in the files with no first line to read.
//
// Deliberately not .bats. A bats file is a test suite in a language that is
// bash with a `@test` header, so it is neither ours to read nor ours to parse.
var shellExtensions = map[string]string{
	".sh": "sh", ".bash": "bash", ".zsh": "zsh", ".zsh-theme": "zsh",
}

// named answers with the shell a file's name claims, or "".
//
// Weaker evidence than a shebang and used only when there is none: a shebang
// is what the kernel obeys, so a `.sh` file that starts `#!/usr/bin/perl` is
// perl and the name is a leftover.
func named(path string) string {
	return shellExtensions[strings.ToLower(filepath.Ext(path))]
}

// autoloadMarkers are the first lines that declare a file to be a zsh function
// rather than a program: a completion says `#compdef`, and a function meant to
// be loaded on first use says `#autoload`.
var autoloadMarkers = []string{"#compdef", "#autoload"}

// autoloaded answers "zsh" for a file whose first line is zsh's own way of
// saying what it is, and "" otherwise.
//
// A shebang is not the only convention for declaring an interpreter, and on a
// machine with zsh installed it is not even the common one. Zsh's function
// files are read by the shell rather than executed by the kernel, so they
// carry no `#!` — which meant a sweep looking only for shebangs found eleven
// zsh scripts on a machine holding several dozen. Every completion a package
// installs is a real zsh program that someone wrote without knowing this
// implementation exists, which is the population this sweep is for.
func autoloaded(line string) string {
	for _, marker := range autoloadMarkers {
		if line == marker || strings.HasPrefix(line, marker+" ") {
			return "zsh"
		}
	}
	return ""
}

// accepts reports whether the reference shell parses the file.
//
// `-n` reads without running, which is the whole reason a reference is usable
// here at all: nothing in the sweep executes anything.
func accepts(ctx context.Context, reference, path string) bool {
	if reference == "" {
		return true
	}
	return exec.CommandContext(ctx, reference, "-n", path).Run() == nil
}
