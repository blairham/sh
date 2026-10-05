// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// history-search-end, incarg, smart-insert-last-word,
// bracketed-paste-url-magic and copy-earlier-word, driven through a session on a pseudo-terminal
// with the shipped files on $fpath. Every expected line was measured
// 2026-10-05 through a pseudo-terminal against /opt/homebrew/bin/zsh (zsh
// 5.9.2) with its own copies, the same keys and the same history.
// docs/spec/functions.md has the tables.
//
// A step ending in a carriage return is a command line, and the next step
// waits for the prompt after it; the others are keys. The line is read back
// as $BUFFER|$CURSOR by a widget on ^G.
func TestTheSmallerContribWidgets(t *testing.T) {
	rcs := map[string]string{
		"hse": `autoload -Uz history-search-end
zle -N history-beginning-search-backward-end history-search-end
zle -N history-beginning-search-forward-end history-search-end
bindkey '^P' history-beginning-search-backward-end
bindkey '^N' history-beginning-search-forward-end
`,
		"inc": "autoload -Uz incarg; zle -N incarg; bindkey '^X+' incarg\n",
		"sil": "autoload -Uz smart-insert-last-word; zle -N insert-last-word smart-insert-last-word; bindkey '\\e.' insert-last-word\n",
		"url": "autoload -Uz bracketed-paste-url-magic; zle -N bracketed-paste bracketed-paste-url-magic\n",
		"cew": "autoload -Uz copy-earlier-word; zle -N copy-earlier-word; bindkey '\\e,' copy-earlier-word\n",
		"ilw": `wa() { zle insert-last-word -- -1 -2 }; zle -N wa; bindkey '^Xa' wa
wb() { zle insert-last-word -- 0 -3 }; zle -N wb; bindkey '^Xb' wb
wc() { zle insert-last-word -- -1 1 1 }; zle -N wc; bindkey '^Xc' wc
hn() { LBUFFER=H$HISTNO }; zle -N hn; bindkey '^Xh' hn
`,
	}
	for _, tc := range []struct {
		name, rc string
		steps    []string
		want     string
	}{
		{"hse1", "hse", []string{": echo apple\r", ": ls x\r", ": echo banana\r", ": ech\x02\x02", "\x10"}, ": echo banana|13"},
		{"hse2", "hse", []string{": echo apple\r", ": ls x\r", ": echo banana\r", ": ech\x02\x02", "\x10\x10"}, ": echo apple|12"},
		{"hse3", "hse", []string{": echo apple\r", ": ls x\r", ": echo banana\r", ": ech\x02\x02", "\x10\x0e"}, ": ech|5"},
		{"hse4", "hse", []string{": echo apple\r", ": ls x\r", ": echo banana\r", ": ech\x02\x02", "\x10\x10\x0e"}, ": echo banana|13"},
		{"hse5", "hse", []string{": echo apple\r", ": ls x\r", ": echo banana\r", ": ech\x02\x02", "\x10\x10\x0e\x0e"}, ": ech|5"},
		{"hse6", "hse", []string{": echo apple\r", ": ls x\r", ": echo banana\r", ": ech\x02\x02", "\x0e"}, ": ech|3"},
		{"hse7", "hse", []string{": echo apple\r", ": ls x\r", ": echo banana\r", ": ech\x02\x02", "\x10\x10\x10\x10"}, ": echo apple|12"},
		{"hse8", "hse", []string{": echo apple\r", "zzz\x02\x02", "\x10"}, "zzz|1"},
		{"hse9", "hse", []string{": echo apple\r", "zzz\x02\x02", "\x0e"}, "zzz|1"},
		{"hse10", "hse", []string{": echo apple\r", ": ls x\r", ": echo banana\r", ": ec", "\x10\x02\x10"}, ": echo banana|12"},
		{"hse11", "hse", []string{": echo apple\r", ": ls x\r", ": echo banana\r", ": ec", "\x1b2\x10"}, ": echo apple|12"},
		{"inc1", "inc", []string{"a 41 b\x01\x06\x06\x06", "\x18+"}, "a 42 b|3"},
		{"inc2", "inc", []string{"a -5 b\x01\x06\x06", "\x18+"}, "a -5 b|2"},
		{"inc3", "inc", []string{"a -5 b\x01\x06\x06\x06", "\x18+"}, "a -6 b|3"},
		{"inc4", "inc", []string{"v=009\x01\x06\x06\x06", "\x18+"}, "v=10|3"},
		{"inc5", "inc", []string{"x 9\x01\x06\x06", "\x18+"}, "x 10|2"},
		{"inc6", "inc", []string{"x 1\x01\x06\x06", "\x1b-\x1b5\x18+"}, "x -4|2"},
		{"inc7", "inc", []string{"incarg=10\r", "n 5\x01\x06\x06", "\x18+"}, "n 15|2"},
		{"inc8", "inc", []string{"a 41 b\x01\x06\x06", "\x1b5\x18+"}, "a 46 b|2"},
		{"inc9", "inc", []string{"a 41 b", "\x18+"}, "a 41 b|6"},
		{"inc10", "inc", []string{"x 09\x01", "\x18+"}, "x 09|0"},
		{"sil1", "sil", []string{": cp /src/a.txt 2> 3\r", ": echo hello 42 >&2\r", "\x1b."}, "hello|5"},
		{"sil2", "sil", []string{": cp /src/a.txt 2> 3\r", ": echo hello 42 >&2\r", "\x1b.\x1b."}, "/src/a.txt|10"},
		{"sil3", "sil", []string{": cp /src/a.txt 2> 3\r", ": echo hello 42 >&2\r", "\x1b.\x1b.\x1b."}, "/src/a.txt|10"},
		{"sil4", "sil", []string{": cp /src/a.txt 2> 3\r", ": echo hello 42 >&2\r", "x \x1b."}, "x hello|7"},
		{"sil5", "sil", []string{": cp /src/a.txt\r", ": 1 2 3\r", "\x1b."}, "3|1"},
		{"sil6", "sil", []string{": cp /src/a.txt\r", ": 1 2 3\r", "\x1b.\x1b."}, "/src/a.txt|10"},
		{"sil7", "sil", []string{"zstyle :insert-last-word auto-previous true\r", ": cp /src/a.txt\r", ": 1 2 3\r", "\x1b."}, "/src/a.txt|10"},
		{"sil8", "sil", []string{"zstyle :insert-last-word match '*[[:digit:]]*'\r", ": echo hello 42 x\r", "\x1b."}, "42|2"},
		{"sil9", "sil", []string{"setopt interactivecomments\r", ": echo hi there # a comment\r", "\x1b."}, "comment|7"},
		{"sil10", "sil", []string{": cp /src/a.txt\r", ": echo a\\\\b\r", "\x1b."}, "a\\\\b|4"},
		{"sil11", "sil", []string{": one\r", ": two\r", "\x1b.x\x1b."}, "twoxtwo|7"},
		{"url0", "url", []string{"curl \x1b[200~http://x/?a=1&b\x1b[201~"}, "curl 'http://x/?a=1&b'|22"},
		{"url1", "url", []string{"curl \x1b[200~https://x.y/p q\x1b[201~"}, "curl 'https://x.y/p q'|22"},
		{"url2", "url", []string{"curl \x1b[200~ftp://h/*\x1b[201~"}, "curl 'ftp://h/*'|16"},
		{"url3", "url", []string{"curl \x1b[200~not a url?\x1b[201~"}, "curl not a url?|15"},
		{"url4", "url", []string{"curl \x1b[200~http://x\x1b[201~"}, "curl http://x|13"},
		{"url5", "url", []string{"curl \x1b[200~file:///tmp/a b\x1b[201~"}, "curl 'file:///tmp/a b'|22"},
		{"url6", "url", []string{"curl \x1b[200~ssh://u@h/?\x1b[201~"}, "curl 'ssh://u@h/?'|18"},
		{"url7", "url", []string{"curl \x1b[200~HTTP://x/?\x1b[201~"}, "curl HTTP://x/?|15"},
		{"url8", "url", []string{"curl \x1b[200~http://x/it's\x1b[201~"}, "curl http://x/it\\'s|19"},
		{"url9", "url", []string{"curl \x1b[200~ http://x/?\x1b[201~"}, "curl  http://x/?|16"},
		{"url10", "url", []string{"curl \x1b[200~http://x/? http://y/?\x1b[201~"}, "curl 'http://x/? http://y/?'|28"},
		{"url11", "url", []string{"\x1b[200~ftps://h/?x\x1b[201~"}, "'ftps://h/?x'|13"},
		{"url12", "url", []string{"\x1b[200~sftp://h/?x\x1b[201~"}, "'sftp://h/?x'|13"},
		{"url13", "url", []string{"\x1b[200~magnet:?xt=a&b\x1b[201~"}, "'magnet:?xt=a&b'|16"},
		{"url14", "url", []string{"\x1b[200~magnet:x?\x1b[201~"}, "'magnet:x?'|11"},
		{"url15", "url", []string{"\x1b[200~http:/x?\x1b[201~"}, "http:/x?|8"},
		{"url16", "url", []string{"\x1b[200~Magnet:?x\x1b[201~"}, "Magnet:?x|9"},
		{"url17", "url", []string{"\x1b[200~http://x/$(id)\x1b[201~"}, "'http://x/$(id)'|16"},
		{"url18", "url", []string{"ab\x02\x1b[200~http://x/?\x1b[201~"}, "a'http://x/?'b|13"},
		{"urlssh", "url", []string{"\x1b[200~ssh://h/?x\x1b[201~"}, "'ssh://h/?x'|12"},
		// copy-earlier-word, and the insert-last-word it is built on (#5987).
		{"cew1", "cew", []string{": one two three\r", ": alpha beta gamma\r", "\x1b.\x1b,"}, "beta|4"},
		{"cew2", "cew", []string{": one two three\r", ": alpha beta gamma\r", "\x1b.\x1b,\x1b,"}, "alpha|5"},
		{"cew3", "cew", []string{": one two three\r", ": alpha beta gamma\r", "\x1b.\x1b,\x1b,\x1b,\x1b,\x1b,"}, ":|1"},
		{"cew4", "cew", []string{": one two three\r", ": alpha beta gamma\r", "p q r \x1b,"}, "p q r r|7"},
		{"cew5", "cew", []string{": one two three\r", ": alpha beta gamma\r", "p q r \x1b,\x1b,"}, "p q r q|7"},
		{"cew6", "cew", []string{": one two three\r", ": alpha beta gamma\r", "p q r \x1b,\x1b,\x1b,\x1b,"}, "p q r |6"},
		{"cew7", "cew", []string{": one two three\r", ": alpha beta gamma\r", "p q r \x1b2\x1b,"}, "p q r q|7"},
		{"cew8", "cew", []string{": one two three\r", ": alpha beta gamma\r", "\x1b.\x1b2\x1b,"}, "alpha|5"},
		{"cew9", "cew", []string{": one two three\r", ": alpha beta gamma\r", "\x1b.\x1b.\x1b,"}, "two|3"},
		{"cew10", "cew", []string{": one two three\r", ": alpha beta gamma\r", "p q r \x1b,\x1b."}, "p q r gamma|11"},
		{"cew11", "cew", []string{": one two three\r", ": alpha beta gamma\r", "\x1b.x\x1b,"}, "gammaxgammax|12"},
		{"ilw1", "ilw", []string{": one two three\r", ": alpha beta gamma\r", "\x18a"}, "beta|4"},
		{"ilw2", "ilw", []string{": one two three\r", ": alpha beta gamma\r", "\x1b.\x18b"}, "alpha|5"},
		{"ilw3", "ilw", []string{": one two three\r", ": alpha beta gamma\r", "\x1b2\x1b."}, "beta|4"},
		{"ilw4", "ilw", []string{": one two three\r", ": alpha beta gamma\r", "p q r \x18c"}, "p q r :|7"},
		{"ilw5", "ilw", []string{": one two three\r", ": alpha beta gamma\r", "\x1b.\x1b2\x1b."}, "two|3"},
		{"histno", "ilw", []string{": one two three\r", ": alpha beta gamma\r", "\x18h"}, "H3|2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen, home := contribSession(t, rcs[tc.rc]+
				`record() { print -r -- "$BUFFER|$CURSOR" >> $HOME/recorded }`+"\n")
			for _, step := range tc.steps {
				if line, ok := strings.CutSuffix(step, "\r"); ok {
					jobNoticeType(t, control, screen, line)
					continue
				}
				contribSend(t, control, step)
			}
			if got := contribRecorded(t, control, screen, home, 1); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
