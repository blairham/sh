// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// An autoloaded function's first call runs at the same depth as every call
// after it (#1842).
//
// The generated stub is a function whose body is `builtin autoload -X`, so
// calling the name enters a frame and the builtin then had to *call* the name
// it had just resolved — a second frame with the same name on it. zsh replaces
// the stub instead, so the loaded body runs in the frame the call already
// opened.
//
// Measured on zsh 5.9.2, 2026-09-10, with one file on `$fpath`:
//
//	fns/fstk   print "n=${#funcstack[@]} stack=${funcstack[*]}"
//	fpath=(fns); autoload -Uz fstk; fstk a
//	  first call   n=1 stack=fstk        second call   n=1 stack=fstk
//
// where this shell answered `n=2 stack=fstk fstk` on the first call and agreed
// on every one after — a frame that is there on the cold call and gone
// afterwards, which is the shape of a bug that reproduces only once per
// function per session. `$funcstack` is read by real prompt and completion
// code to find out where it is.
func TestAnAutoloadedFunctionsFirstCallIsNotADeeperOne(t *testing.T) {
	t.Run("the stack on the first call and the second", func(t *testing.T) {
		fp := fpathDir(t, map[string]string{
			"fstk": `print "n=${#funcstack[@]} stack=${funcstack[*]} zero=$0"`,
		})
		src := "fpath=(" + fp + ")\nautoload -Uz fstk\nfstk a\nfstk a\n"
		want := "n=1 stack=fstk zero=fstk\nn=1 stack=fstk zero=fstk\n"
		if out, st := runZsh(t, t.TempDir(), src); out != want || st != 0 {
			t.Errorf("got %q (status %d), want %q", out, st, want)
		}
	})

	t.Run("a diagnostic from the body is located in the function", func(t *testing.T) {
		// The same seam from the other side: with the body running as a
		// nested call, the builtin that made the call was still on the stack
		// and the location read `fstk:builtin:1:` on the first call and
		// `fstk:1:` on every one after. Measured, zsh writes `fstk:1:` both
		// times.
		fp := fpathDir(t, map[string]string{"fstk": "nosuchcmd_zz\n"})
		src := "fpath=(" + fp + ")\nautoload -Uz fstk\nfstk\nprint \"st=$?\"\nfstk\nprint \"st2=$?\"\n"
		want := "fstk:1: command not found: nosuchcmd_zz\nst=127\n" +
			"fstk:1: command not found: nosuchcmd_zz\nst2=127\n"
		if out, _ := runZsh(t, t.TempDir(), src); out != want {
			t.Errorf("got %q, want %q", out, want)
		}
	})

	t.Run("the arguments, the locals and the status are unchanged", func(t *testing.T) {
		// The three things the nested arrangement already got right, asserted
		// so that running in place does not lose one of them: the body sees
		// the call's own positional parameters, a `local` in it unwinds at
		// the end of the call, and the status is the body's.
		fp := fpathDir(t, map[string]string{
			"fstk": "local loc=inner\nprint \"args=<$*>\"\nreturn 7\n",
		})
		src := "fpath=(" + fp + ")\nautoload -Uz fstk\nfstk a b\nprint \"st=$?\"\n" +
			"print \"leak=[${loc-unset}]\"\n"
		want := "args=<a b>\nst=7\nleak=[unset]\n"
		if out, _ := runZsh(t, t.TempDir(), src); out != want {
			t.Errorf("got %q, want %q", out, want)
		}
	})
}

// And a `-X` a script wrote by hand still nests, because that is a different
// construct and the measurement behind it is its own.
//
// The stub there is a function the script really wrote: its body runs, the
// loaded text runs inside it, and the stub carries on afterwards with the
// body's status. Measured on zsh 5.9.2 — `${#funcstack}` counts *more* than
// one there, the body sees the stub's locals, and `AFTER 7` follows.
//
// The two are told apart by whether the name is a stub `autoload` generated
// and nothing has defined since, asked before the resolution takes the stub's
// body away.
func TestAHandWrittenResolutionStillRunsInsideItsStub(t *testing.T) {
	fp := fpathDir(t, map[string]string{
		"myf": "print -r -- \"BODY args=<$*> deep=$(( ${#funcstack[@]} > 1 )) secret=$secret\"\nreturn 7\n",
	})
	src := "myf() { local -a fpath; fpath=( " + fp + " ); local secret=hidden; " +
		"builtin autoload -X -Uz; print \"AFTER $?\" }\nmyf a b\n"
	want := "BODY args=<a b> deep=1 secret=hidden\nAFTER 7\n"
	if out, _ := runZsh(t, t.TempDir(), src); out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
