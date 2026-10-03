// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `zmodload -a`, measured 2026-10-03 against zsh 5.9.2 under `-f`. Each want
// is the transcript the real shell wrote for the same snippet, with two
// differences that are stated rather than hidden: a module that will not load
// is `not implemented yet` here where zsh quotes its dlopen error, and zsh's
// own listing starts with its build's two dozen autoloadable builtins, which
// this one does not have — see zmodloadautoload.go.
//
// A failed call is wrapped in braces before it is silenced. zsh attempts the
// load when it *looks the name up*, before the command's own redirections are
// applied, so `mb 2>/dev/null` still writes the complaint there; this shell
// loads when the builtin runs, inside them.

func runZmodloadAutoload(t *testing.T, src, want string, status int) {
	t.Helper()
	out, st := runZsh(t, t.TempDir(), src)
	if out != want || st != status {
		t.Errorf("%s\n = %q (status %d)\nwant %q (status %d)", src, out, st, want, status)
	}
}

// A module that will not load is refused at the call, and the promise is spent: the next call is command not found.
func TestZmodloadAutoloadIsAPromiseAndACallSpendsIt(t *testing.T) {
	runZmodloadAutoload(t, `zmodload -a zsh/nosuch mb
print -r -- "st=$?"
{ mb; } 2>/dev/null
print -r -- "c=$?"
mb 2>&1
print -r -- "c2=$?"`,
		"st=0\n"+
			"c=1\n"+
			"zsh:5: command not found: mb\n"+
			"c2=127\n", 0)
}

// A module that loads and does not define the name is two lines and the end of the script, with the module left unloaded and the promise standing — a subshell gives up alone.
func TestZmodloadAutoloadOfANameTheModuleLacksEndsTheScript(t *testing.T) {
	runZmodloadAutoload(t, `zmodload -a zsh/zutil zfoo
print -r -- "st=$?"
( zfoo ) 2>&1
print -r -- "sub=$?"
zmodload -e zsh/zutil
print -r -- "e=$?"
zmodload -a
zfoo 2>&1
print -r -- never`,
		"st=0\n"+
			"zsh:3: module `zsh/zutil' has no such feature: `b:zfoo': autoload cancelled\n"+
			"zsh:3: autoloading module zsh/zutil failed to define builtin: zfoo\n"+
			"sub=1\n"+
			"e=1\n"+
			"zfoo (zsh/zutil)\n"+
			"zsh:8: module `zsh/zutil' has no such feature: `b:zfoo': autoload cancelled\n"+
			"zsh:8: autoloading module zsh/zutil failed to define builtin: zfoo\n", 1)
}

// A module's own builtin is a promise until the module loads, so it can be registered; the call loads the module and runs the real builtin, after which the name is defined and a second registration is refused.
func TestZmodloadAutoloadOverAModulesOwnBuiltin(t *testing.T) {
	runZmodloadAutoload(t, `zmodload -a zsh/zutil zformat
print -r -- "st=$?"
zformat -f R "%a-%b" a:1 b:2
print -r -- "[$R] st=$?"
zmodload -e zsh/zutil
print -r -- "e=$? [${(M)${(f)"$(zmodload -a)"}:#zformat*}]"
zmodload -a zsh/nosuch zformat 2>&1
print -r -- "again=$?"`,
		"st=0\n"+
			"[1-2] st=0\n"+
			"e=0 []\n"+
			"zsh:zmodload:7: failed to add builtin `zformat'\n"+
			"again=1\n", 0)
}

// The refusals: a builtin that is really there, a name with a slash (which is what a module named with no builtin after it registers), and `-b` on its own.
func TestZmodloadAutoloadRefusals(t *testing.T) {
	runZmodloadAutoload(t, `zmodload -a zsh/nosuch echo 2>&1
print -r -- "echo=$?"
zmodload -a zsh/nosuch 2>&1
print -r -- "default=$?"
zmodload -a m a/b c 2>&1
print -r -- "slash=$?"
zmodload -b m x 2>&1
print -r -- "b=$?"
zmodload -a`,
		"zsh:zmodload:1: failed to add builtin `echo'\n"+
			"echo=1\n"+
			"zsh:zmodload:3: zsh/nosuch: `/' is illegal in a builtin\n"+
			"default=1\n"+
			"zsh:zmodload:5: a/b: `/' is illegal in a builtin\n"+
			"slash=1\n"+
			"zsh:zmodload:7: -b, -c, -f, and -p must be combined with -a or -u\n"+
			"b=1\n"+
			"c (m)\n", 0)
}

// The two listings, `-L` with operands registering, `$modules`, and `-ua` and `-ub` taking promises back.
func TestZmodloadAutoloadListingsAndRemoval(t *testing.T) {
	runZmodloadAutoload(t, `zmodload -a zsh/nosuch mc mb
zmodload -ab foo
zmodload -La m1 b1
print -r -- "La=$?"
zmodload -a
zmodload -La
print -r -- "[$modules[zsh/nosuch]] [$modules[foo]] [${+modules[m1]}]"
whence -w mb
zmodload -ua mb
print -r -- "ua=$?"
whence -w mb
zmodload -ub zz mc 2>&1
print -r -- "ub=$?"
zmodload -ua 2>&1
print -r -- "none=$?"
zmodload -u 2>&1
print -r -- "u=$?"
zmodload -La`,
		"La=0\n"+
			"b1 (m1)\n"+
			"foo\n"+
			"mb (zsh/nosuch)\n"+
			"mc (zsh/nosuch)\n"+
			"zmodload -ab m1 b1\n"+
			"zmodload -ab foo\n"+
			"zmodload -ab zsh/nosuch mb\n"+
			"zmodload -ab zsh/nosuch mc\n"+
			"[autoloaded] [autoloaded] [1]\n"+
			"mb: builtin\n"+
			"ua=0\n"+
			"mb: none\n"+
			"zsh:zmodload:12: zz: no such builtin\n"+
			"ub=1\n"+
			"zsh:zmodload:14: what do you want to unload?\n"+
			"none=1\n"+
			"zsh:zmodload:16: what do you want to unload?\n"+
			"u=1\n"+
			"zmodload -ab m1 b1\n"+
			"zmodload -ab foo\n", 0)
}

// A module loading defines every promised name it has, whichever module the
// promise named: the promise to zsh/nosuch is replaced by zsh/zutil's own
// builtin, so registering it again is refused and the name runs. A rule that
// settled only promises naming the module being loaded would leave the
// promise standing here and answer 0 to the second line.
func TestZmodloadAutoloadIsSettledByAnyLoadOfTheDefiningModule(t *testing.T) {
	runZmodloadAutoload(t, `zmodload -a zsh/nosuch zparseopts
print -r -- "st=$?"
zmodload zsh/zutil
zmodload -a zsh/nosuch zparseopts 2>&1
print -r -- "again=$?"
zparseopts -D -E -A opts -- x
print -r -- "ran=$?"`,
		"st=0\n"+
			"zsh:zmodload:4: failed to add builtin `zparseopts'\n"+
			"again=1\n"+
			"ran=0\n", 0)
}
