// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheShippedAddZleHookWidget drives share/sh/functions/add-zle-hook-widget
// against bytes recorded from zsh 5.9.2 on 2026-10-02 under `-f -c`, with
// FPATH naming a directory of function files: the `types` style the first
// registration sets, the `n:name` entries and their numbering across a
// removal, the widget already on the special name kept as `0:user:…`, the
// dispatcher put in its place, `-d`, `-D`, `-L`, the autoload of a name that
// is no widget yet, and the usage (#5393). Nothing here reads zsh's function
// file; the probe calls it.
func TestTheShippedAddZleHookWidget(t *testing.T) {
	out, _ := runShipped(t, `autoload -Uz add-zle-hook-widget; myinit() { :; }; zle -N zle-line-init myinit
foo() {:}; zle -N foo; add-zle-hook-widget line-init foo; zstyle -L; zle -lL zle-line-init
add-zle-hook-widget line-init foo; add-zle-hook-widget zle-line-init baz; add-zle-hook-widget -d line-init foo
add-zle-hook-widget line-init qux; zstyle -L zle-line-init; add-zle-hook-widget -D line-init "q*"
add-zle-hook-widget -L; add-zle-hook-widget -Uz keymap-select kk; functions kk; zle -lL kk
add-zle-hook-widget nosuch x 2>&1; print st=$?; add-zle-hook-widget a 2>&1; print st=$?`)
	const usage = "Usage: add-zle-hook-widget hook widgetname\nValid hooks are:\n" +
		"  isearch-exit isearch-update line-pre-redraw line-init line-finish history-line-set keymap-select\n"
	want := "zstyle zle-hook types isearch-exit isearch-update line-pre-redraw line-init line-finish history-line-set keymap-select\n" +
		"zstyle zle-line-init widgets 0:user:myinit 1:foo\n" +
		"zle -N zle-line-init azhw:zle-line-init\n" +
		"zstyle zle-line-init widgets 0:user:myinit 2:baz 3:qux\n" +
		"zstyle zle-line-init widgets 0:user:myinit 2:baz\n" +
		"kk () {\n\t# undefined\n\tbuiltin autoload -XUz\n}\n" +
		"zle -N kk\n" +
		usage + "st=1\n" + usage + "st=1\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}
