// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// TestEvalTextIsAUnitTheTraceArraysReport is #5159's V06parameter front
// `Eval tracing`. Text handed to `eval` is a unit of its own in zsh's four
// trace arrays: `(eval)` in `$funcstack`, with an entry in `$functrace`,
// `$funcfiletrace` and `$funcsourcetrace`. A call made from inside the text
// is located there twice, as `(eval):<line of the text>` and as the file's own
// line. A function the text defines is located at the file's line.
//
// Each script is run from its own directory, so the names are the relative
// ones the reference printed. Every expectation is the output of
// `/opt/homebrew/bin/zsh -f` (zsh 5.9.2) over the same file, captured
// 2026-10-02.
func TestEvalTextIsAUnitTheTraceArraysReport(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"t1", "zmodload zsh/parameter\nf() {\n  print -r -- \"ft=[$functrace] fft=[$funcfiletrace] fst=[$funcsourcetrace] fs=[$funcstack]\"\n}\nf\neval 'f'\neval '\nprint -r -- \"e-ft=[$functrace] e-fft=[$funcfiletrace] e-fst=[$funcsourcetrace] e-fs=[$funcstack]\"\nf'\neval 'g() { print -r -- \"g-fft=[$funcfiletrace] g-fst=[$funcsourcetrace]\"; }'\ng\nh() { eval 'print -r -- \"h-ft=[$functrace] h-fft=[$funcfiletrace] h-fst=[$funcsourcetrace] h-fs=[$funcstack]\"'; }\nh\n", "ft=[./t1.zsh:5] fft=[./t1.zsh:5] fst=[./t1.zsh:2] fs=[f]\nft=[(eval):1 ./t1.zsh:6] fft=[./t1.zsh:6 ./t1.zsh:6] fst=[./t1.zsh:2 ./t1.zsh:6] fs=[f (eval)]\ne-ft=[./t1.zsh:7] e-fft=[./t1.zsh:7] e-fst=[./t1.zsh:7] e-fs=[(eval)]\nft=[(eval):3 ./t1.zsh:7] fft=[./t1.zsh:9 ./t1.zsh:7] fst=[./t1.zsh:2 ./t1.zsh:7] fs=[f (eval)]\ng-fft=[./t1.zsh:11] g-fst=[./t1.zsh:11]\nh-ft=[h:0 ./t1.zsh:13] h-fft=[./t1.zsh:12 ./t1.zsh:13] h-fst=[./t1.zsh:12 ./t1.zsh:12] h-fs=[(eval) h]\n"},
		{"t2", "zmodload zsh/parameter\np() { print -r -- \"$1 fs=[$funcstack] ft=[$functrace] fft=[$funcfiletrace] fst=[$funcsourcetrace]\"; }\neval 'eval \"p nested\"'\nunsetopt evallineno\neval 'p offopt'\nsetopt evallineno\nk() {\n  eval '\n    p ink'\n}\nk\neval '\nq() {\n  p inq\n}'\nq\n. ./inc.zsh\n", "nested fs=[p (eval) (eval)] ft=[(eval):1 (eval):1 ./t2.zsh:3] fft=[./t2.zsh:3 ./t2.zsh:3 ./t2.zsh:3] fst=[./t2.zsh:2 ./t2.zsh:3 ./t2.zsh:3]\noffopt fs=[p] ft=[./t2.zsh:5] fft=[./t2.zsh:5] fst=[./t2.zsh:2]\nink fs=[p (eval) k] ft=[(eval):2 k:1 ./t2.zsh:11] fft=[./t2.zsh:9 ./t2.zsh:8 ./t2.zsh:11] fst=[./t2.zsh:2 ./t2.zsh:8 ./t2.zsh:7]\ninq fs=[p q] ft=[q:1 ./t2.zsh:16] fft=[./t2.zsh:15 ./t2.zsh:16] fst=[./t2.zsh:2 ./t2.zsh:14]\ninsourced fs=[p (eval) ./inc.zsh] ft=[(eval):1 ./inc.zsh:2 ./t2.zsh:17] fft=[./inc.zsh:2 ./inc.zsh:2 ./t2.zsh:17] fst=[./t2.zsh:2 ./inc.zsh:2 ./inc.zsh:0]\n"},
		{"t5", "zmodload zsh/parameter\np() { print -r -- \"$1 fft=[$funcfiletrace] fst=[$funcsourcetrace]\"; }\neval 'eval \"g() { p g; }\"'\ng\neval '\no() {\n  i() { p i; }\n  i\n}'\no\n", "g fft=[./t5.zsh:4 ./t5.zsh:4] fst=[./t5.zsh:2 ./t5.zsh:4]\ni fft=[./t5.zsh:8 ./t5.zsh:9 ./t5.zsh:10] fst=[./t5.zsh:2 ./t5.zsh:8 ./t5.zsh:7]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			for name, body := range map[string]string{c.name + ".zsh": c.src, "inc.zsh": "\neval \"p insourced\"\n"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			out, errs, code := runZsh(t, "-f", "./"+c.name+".zsh")
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("got status %d, stderr %q\n%s\nwant\n%s", code, errs, out, c.want)
			}
		})
	}
}

// And the `-c` route, where the top level is no file at all: an `eval` there
// is located at `$0`, as a function entered from there is, while the function
// defined there is in the shell's fixed name. Invoked as `./myzsh` so that the
// two names are different strings — measured through a symlink of that name,
// which writes `ft=[(eval):1 ./myzsh:2] fft=[./myzsh:2 ./myzsh:2] fst=[zsh:1
// ./myzsh:2]`.
func TestEvalTextAtTheTopOfACommandStringIsLocatedAtTheShell(t *testing.T) {
	src := "p() { print -r -- \"ft=[$functrace] fft=[$funcfiletrace] fst=[$funcsourcetrace]\"; }\neval p\n"
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	driver.MainArgs(sh, []string{"./myzsh", "-f", "-c", src})
	if want := "ft=[(eval):1 ./myzsh:2] fft=[./myzsh:2 ./myzsh:2] fst=[zsh:1 ./myzsh:2]\n"; out.String() != want || errs.String() != "" {
		t.Errorf("got %q %q, want %q", out.String(), errs.String(), want)
	}
}
