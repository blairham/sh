// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A function file may hold the function's *body*, or it may hold a
// `name() { … }` definition of the function it is named after. This shell
// runs both; it used to run the first and silently do nothing for the second.
//
// Measured on zsh 5.9.2, 2026-09-10, with files on `$fpath`:
//
//	pfn   print "PLAIN ran [$*]"        pfn a b   PLAIN ran [a b]
//	kfn   kfn() { print "K [$*]" }      kfn a b   K [a b]
//
// The second was nothing at all here, at status 0: the file's text became the
// body, so the first call ran a *definition*, redefined the function and
// returned. The second call then worked, which is what made it silent —
// nothing is missing afterwards and no diagnostic is written, so a test that
// called the function twice, or that only inspected `functions` afterwards,
// would have passed (#1704).
//
// Every case here therefore asserts what the **first** call printed.
func TestAFunctionFileMayHoldTheDefinitionOfItsOwnName(t *testing.T) {
	dir := autoloadKshDir(t)
	for _, tc := range []struct{ name, src, want string }{
		{
			"the body spelling",
			"pfn a b",
			"PLAIN ran [a b]\n",
		},
		{
			"the definition spelling, on the first call",
			"kfn a b",
			"KSH ran [a b]\n",
		},
		{
			"and again on the second, from the same definition",
			"kfn a b\nkfn c",
			"KSH ran [a b]\nKSH ran [c]\n",
		},
		{
			"the `function` keyword counts too",
			"kwfn z",
			"KW ran [z]\n",
		},
		{
			"a comment above it counts",
			"cmtfn q",
			"CMT ran [q]\n",
		},
		{
			"a trailing separator counts",
			"semifn q",
			"SEMI ran [q]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, "fpath=("+dir+")\nautoload -Uz "+autoloadKshNames+"\n"+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// It is the **whole file** that has to be the definition, and these are the
// files that settle it. Each of them defines the name it is called and each
// of them is *not* called on the first pass, so neither "the load defined
// this name" nor "the last command was a definition" is the rule.
func TestAFileThatDoesMoreThanDefineItsOwnNameIsJustABody(t *testing.T) {
	dir := autoloadKshDir(t)
	for _, tc := range []struct{ name, src, want string }{
		{
			"a definition and a statement after it",
			"mixfn a b\nmixfn c",
			"MIX-BODY [a b]\nMIX-DEF ran [c]\n",
		},
		{
			"another function defined before it",
			"twofn a\ntwofn b",
			"TWO ran [b]\n",
		},
		{
			"the definition wrapped in a group",
			"grpfn a\ngrpfn b",
			"GRP ran [b]\n",
		},
		{
			"a file defining some other name",
			"othfn a\nothfn b",
			"OTH-BODY [a]\nOTH-BODY [b]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, "fpath=("+dir+")\nautoload -Uz "+autoloadKshNames+"\n"+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The shape is decided at **load** time and not at call time, which is what
// `+X` measures: the function is defined with the *inner* body before
// anything has called it.
func TestLoadingWithoutRunningDefinesTheInnerBody(t *testing.T) {
	dir := autoloadKshDir(t)
	out, st := runZsh(t, dir,
		"fpath=("+dir+")\nautoload -Uz "+autoloadKshNames+"\nautoload +X kfn\nfunctions kfn")
	if st != 0 || strings.Contains(out, "kfn() {") || !strings.Contains(out, "KSH ran") {
		t.Errorf("functions kfn = %q (status %d), want the inner body and no nested definition", out, st)
	}
}

// autoloadKshNames are the files autoloadKshDir writes, as one operand list.
const autoloadKshNames = "pfn kfn kwfn cmtfn semifn mixfn twofn grpfn othfn"

// autoloadKshDir is a directory of function files, one per shape.
func autoloadKshDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"pfn":    `print "PLAIN ran [$*]"`,
		"kfn":    `kfn() { print "KSH ran [$*]" }`,
		"kwfn":   `function kwfn { print "KW ran [$*]" }`,
		"cmtfn":  "# what this function is for\ncmtfn() { print \"CMT ran [$*]\" }",
		"semifn": `semifn() { print "SEMI ran [$*]" };`,
		"mixfn":  "mixfn() { print \"MIX-DEF ran [$*]\" }\nprint \"MIX-BODY [$*]\"",
		"twofn":  "helper() { print HELPER }\ntwofn() { print \"TWO ran [$*]\" }",
		"grpfn":  `{ grpfn() { print "GRP ran [$*]" } }`,
		"othfn":  "notothfn() { print NOT }\nprint \"OTH-BODY [$*]\"",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
