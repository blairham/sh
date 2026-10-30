// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A command behind a precommand modifier, as `set -x` writes it.
//
// The one dialect with the modifier class writes the command the scan left,
// not the words that were written. Measured 2026-10-30 against zsh 5.9.2 under
// `-f -c` and `env -i PATH=/usr/bin:/bin` (#6967):
//
//	command -p true          `true`
//	builtin true             `true`
//	exec /usr/bin/true       `/usr/bin/true`
//	builtin builtin true     `true`
//	builtin command true     `true`
//	command command true     `command true` — `command` stops the scan
//	command builtin true     `builtin true`, and `command not found: builtin`
//	x=1 command true         `x=1 true`
//	x=1 builtin true         `x=1 +zsh:1> true`
//	c=command; $c true       `true`
//	noglob builtin print a*  `print 'a*'`
//
// So the modifier words and their options are left out wherever they came
// from, and the line is what is about to run. A program is written under the
// name it is started with:
//
//	exec -a zz /bin/echo hi  `zz hi`
//	exec -l /bin/echo hi     `-/bin/echo hi`, and `exec -cl` the same
//	exec -la zz /bin/echo hi `zz hi` — the name `-a` gave, with no dash
//	- /bin/echo hi           `-/bin/echo hi`
//	exec -a zz echo hi       `echo hi` — a builtin takes no name
//	- echo hi                `echo hi`
//
// `command -v` and `-V` are lookups rather than modifiers, and keep every word:
// `command -v true` is `command -v true`. And `noglob` was taken away before
// the words were made, so it was never on the line.
//
// The same scan decides the second prompt a prefixed line carries, which is
// asked of the command the scan left and not of the modifier: `x=1 exec
// /usr/bin/true` is `x=1 /usr/bin/true`, one prompt, where `x=1 exec true` is
// `x=1 +zsh:1> true`, and `x=1 builtin command true` is one prompt, as `x=1
// command true` is. See Runner.tracePrefixRepeatsBeforeTheCommand.
//
// Not modeled here. `exec 3>/dev/null`, `exec -a` with no command, `exec -x`
// and a `builtin` naming no builtin write no line at all there; this writes one
// (#7097). A program is traced under the name ARGV0 gives it too (#7120), and
// the name `exec -a` gives reaches the programs a function it names or a
// substitution among its words starts (#7121).

// precommandTrace is what the precommand scan made of a traced command's
// leading words.
type precommandTrace struct {
	// out is how many of the words are modifiers and their options.
	out int
	// name is the name the command is started under, where named says a
	// modifier gave it one. A name may be empty: `exec -a '' /bin/echo hi` is
	// `'' hi`.
	name  string
	named bool
	// throughCommand says `command` ended the scan.
	throughCommand bool
}

// precommandTraceScan reads a traced command's leading precommand modifiers.
//
// words is the command as expanded. Reports nothing left out for a dialect
// that writes the words as written — see
// Diagnostics.TraceLeavesOutPrecommandModifiers.
func (r *Runner) precommandTraceScan(words []string) precommandTrace {
	if !r.diag().TraceLeavesOutPrecommandModifiers || len(r.precommands) == 0 {
		return precommandTrace{}
	}
	var t precommandTrace
	dash := r.dashPrecommand
	i := 0
scan:
	for i < len(words) {
		m, ok := r.precommands[words[i]]
		if !ok {
			break
		}
		switch m {
		case PrecommandTransparent, PrecommandRedirectionForm:
			j := i + 1
			if words[i] == "exec" {
				var o execTraceOption
				j, o = execTraceOptions(words, j)
				if o.named {
					t.name, t.named = o.name, true
				}
				dash = dash || o.dash
			}
			i = j
		case PrecommandStopsTheScan:
			j := i + 1
			for j < len(words) && strings.HasPrefix(words[j], "-") && words[j] != "-" {
				w := words[j]
				j++
				if w == "--" {
					break
				}
				if strings.ContainsAny(w, "vV") {
					// A lookup: the builtin itself is the command.
					return precommandTrace{out: i}
				}
				if strings.Trim(w[1:], "p") != "" {
					// Not an option `command` takes: the scan stopped at the
					// word before, which is the command's name.
					j--
					break
				}
			}
			i, t.throughCommand = j, true
			break scan
		default:
			break scan
		}
	}
	t.out = i
	if i >= len(words) || !t.named && !dash {
		t.name, t.named = "", false
		return t
	}
	cmd := words[i]
	if !t.throughCommand {
		// A builtin or a function takes no name: `exec -a zz echo hi` is
		// `echo hi`. Behind `command` neither is looked for.
		if _, ok := r.funcs[cmd]; ok {
			t.name, t.named = "", false
			return t
		}
		if _, ok := r.lookupBuiltin(cmd); ok {
			t.name, t.named = "", false
			return t
		}
	}
	if !t.named {
		// A name `-a` gave is the whole of it: `exec -la zz /bin/echo hi`
		// is `zz hi`, with no dash.
		t.name, t.named = "-"+cmd, true
	}
	return t
}

// execTraceOption is what `exec`'s options say about the name a program is
// started under.
type execTraceOption struct {
	name  string
	named bool // `-a` gave a name, which may be empty
	dash  bool // `-l` asked for a dash
}

// execTraceOptions reads `exec`'s option words from words[j:], and reports
// where the command's own words start and what the options asked for.
func execTraceOptions(words []string, j int) (int, execTraceOption) {
	var o execTraceOption
	for j < len(words) {
		w := words[j]
		if w == "--" {
			return j + 1, o
		}
		if !strings.HasPrefix(w, "-") || w == "-" {
			return j, o
		}
		j++
		letters := w[1:]
		if k := strings.IndexByte(letters, 'a'); k >= 0 {
			o.dash = o.dash || strings.Contains(letters[:k], "l")
			if k == len(letters)-1 {
				if j < len(words) {
					o.name, o.named = words[j], true
					j++
				}
				continue
			}
			o.name, o.named = letters[k+1:], true
			continue
		}
		o.dash = o.dash || strings.Contains(letters, "l")
	}
	return j, o
}
