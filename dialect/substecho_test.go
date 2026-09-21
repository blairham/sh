// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/driver"
)

// What follows a `$( … )` body's refusal, from a script file (#3331).
//
// The sentence names the closer since #3296. Two dialects then write a second
// line, and the text it quotes is the **script's** rather than the body's —
// which a runner that parses the body at expansion time never had, so the
// front end and every route that runs text now say what text that is. See
// interp/substecho.go.
//
// Measured 2026-09-16, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> s.sh` with
// stdin from /dev/null, in a fresh directory; BusyBox v1.37.0 in the
// digest-pinned Alpine image internal/oracle reaches. Every want below is
// that measurement with the path replaced.
//
// Through the driver and a real file, because the text is the front end's to
// hand over: a Runner built in a test has no program text and writes the one
// line it always wrote.
func TestASubstitutionRefusalQuotesTheScript(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      map[string]string
	}{
		{
			// Something else on the line, which is what tells the script's
			// line from the body: bash quotes all of it, zsh the word from
			// its start cut at twenty bytes and numbered one line on.
			name: "the line holds more than the substitution",
			src:  "printf 'start\\n'\nq=1; v=$(echo hi; for); z=2\n",
			want: map[string]string{
				"bash": "s.sh: line 2: syntax error near unexpected token `)'\n" +
					"s.sh: line 2: `q=1; v=$(echo hi; for); z=2'\n",
				"zsh": "s.sh:2: parse error near `)'\n" +
					"s.sh:3: parse error near `v=$(echo hi; for); z...'\n",
				"ksh":  "s.sh: line 2: syntax error at line 2: `)' unexpected\n",
				"dash": "s.sh: 2: Syntax error: Bad for loop variable\n",
				"ash":  "s.sh: line 2: syntax error: bad for loop variable\n",
			},
		},
		{
			// A body over two lines, refused on the second. Both messages
			// move to the failure's line in four columns — the prefix used
			// to name the line the command began on — and ksh93 keeps the
			// command's line in front because its sentence names the other.
			// bash quotes the failure's line; zsh the word's own line, to
			// its newline.
			name: "the body runs onto a later line",
			src:  "printf 'start\\n'\n\nq=1; v=$(echo hi\n for); z=2\necho after\n",
			want: map[string]string{
				"bash": "s.sh: line 4: syntax error near unexpected token `)'\n" +
					"s.sh: line 4: ` for); z=2'\n",
				"zsh": "s.sh:4: parse error near `)'\n" +
					"s.sh:5: parse error near `v=$(echo hi'\n",
				"ksh":  "s.sh: line 3: syntax error at line 4: `)' unexpected\n",
				"dash": "s.sh: 4: Syntax error: Bad for loop variable\n",
				"ash":  "s.sh: line 4: syntax error: bad for loop variable\n",
			},
		},
		{
			// A **redirection target**, which is a word like any other and
			// was reached through a second expansion walk that recorded no
			// word at all — so the dialect quoting the word had nothing to
			// quote from and wrote no second line (#3355). Nothing about
			// the message is special; the call was missing.
			//
			// bash is left out of this row rather than measured wrong: it
			// stops the script at the refusal where this engine goes on to
			// call the target ambiguous, which is a separate divergence and
			// not this one. ksh93 places its own sentence at `line 0` here.
			name: "the substitution is a redirection target",
			src:  "printf 'start\\n'\necho hi >$(echo hi; for)\necho after\n",
			want: map[string]string{
				"zsh": "s.sh:2: parse error near `)'\n" +
					"s.sh:3: parse error near `$(echo hi; for)'\n",
				"dash": "s.sh: 2: Syntax error: Bad for loop variable\n",
			},
		},
		{
			// **Inside double quotes**, where that dialect writes a
			// different sentence rather than a different quote. The quote is
			// still open when the input runs out, and it is what the line
			// names (#3355).
			//
			// bash is the control column and is unmoved: it quotes the
			// script's line whatever is open around the substitution, so a
			// rule about open contexts must not reach it.
			name: "the substitution is inside double quotes",
			src:  "printf 'start\\n'\necho \"x $(echo hi; for) y\"\n",
			want: map[string]string{
				"zsh": "s.sh:2: parse error near `)'\n" +
					"s.sh:3: unmatched \"\n",
				"bash": "s.sh: line 2: syntax error near unexpected token `)'\n" +
					"s.sh: line 2: `echo \"x $(echo hi; for) y\"'\n",
				"dash": "s.sh: 2: Syntax error: Bad for loop variable\n",
				"ksh":  "s.sh: line 2: syntax error at line 2: `)' unexpected\n",
			},
		},
		{
			// **Inside an expansion's operand**, where the `${` is what is
			// still open. Same shape, different sentence, and the same
			// control column.
			//
			// One line, and that is a limit rather than a preference: the
			// **first** message is misplaced for this shape — a body inside
			// a `${ }` operand is reported at line 1 of a script whose
			// second line holds it, in every column, on `main` and here
			// alike. That is the span-placement half this issue also names
			// and it is untouched by this change, so the rows that would
			// expose it are written where the two lines coincide. Filed
			// separately; the sentence is what these rows are about.
			name: "the substitution is an expansion's operand",
			src:  "echo ${x:-$(echo hi; for)}\n",
			want: map[string]string{
				"zsh": "s.sh:1: parse error near `)'\n" +
					"s.sh:2: closing brace expected\n",
				"bash": "s.sh: line 1: syntax error near unexpected token `)'\n" +
					"s.sh: line 1: `echo ${x:-$(echo hi; for)}'\n",
				"dash": "s.sh: 1: Syntax error: Bad for loop variable\n",
				"ksh":  "s.sh: line 1: syntax error at line 1: `)' unexpected\n",
			},
		},
		{
			// The discriminator between the two above: with a quote **and**
			// a brace open at one level, the quote is what is named — and it
			// is named from either side, so this is not "the innermost
			// context wins". Written twice because one order alone would
			// pass against a build that always reported the outermost.
			name: "a quote outside the brace outranks it",
			src:  "echo \"${x:-$(for)}\"\n",
			want: map[string]string{
				"zsh": "s.sh:1: parse error near `)'\n" +
					"s.sh:2: unmatched \"\n",
				"bash": "s.sh: line 1: syntax error near unexpected token `)'\n" +
					"s.sh: line 1: `echo \"${x:-$(for)}\"'\n",
			},
		},
		{
			name: "a quote inside the brace outranks it too",
			src:  "echo ${x:-\"$(for)\"}\n",
			want: map[string]string{
				"zsh": "s.sh:1: parse error near `)'\n" +
					"s.sh:2: unmatched \"\n",
				"bash": "s.sh: line 1: syntax error near unexpected token `)'\n" +
					"s.sh: line 1: `echo ${x:-\"$(for)\"}'\n",
			},
		},
		{
			// And the row that says a line is written per **level** and not
			// per open context: two braces at one level write one line. A
			// build that unwound the contexts one at a time would write
			// `closing brace expected` twice here, and the reading this was
			// first proposed under predicted exactly that.
			name: "two braces at one level write one line",
			src:  "echo ${x:-${y:-$(for)}}\n",
			want: map[string]string{
				"zsh": "s.sh:1: parse error near `)'\n" +
					"s.sh:2: closing brace expected\n",
			},
		},
		{
			// A level holding neither writes nothing at all and the level
			// outside it still writes its own: the intermediate `$( )` here
			// is such a level, and the quote is the script's. Without the
			// walk outward this writes nothing, which is what it did.
			name: "an empty level falls through to the one outside it",
			src:  "printf 'start\\n'\necho \"$(echo $(for))\"\n",
			want: map[string]string{
				"zsh": "s.sh:2: parse error near `)'\n" +
					"s.sh:3: unmatched \"\n",
			},
		},
		{
			// An arithmetic expansion's text is re-lexed, so it is a level
			// too — and one with no sentence of its own. The quote outside
			// it is what is named, and the unquoted spelling of the same row
			// writes nothing at all, which is the control: a build that read
			// the span's quoting without opening a level names a quote for a
			// row that holds none.
			name: "an arithmetic level has no sentence of its own",
			src:  "echo \"$(( $(for) + 1 ))\"\n",
			want: map[string]string{
				"zsh": "s.sh:1: parse error near `)'\n" +
					"s.sh:2: unmatched \"\n",
			},
		},
		{
			// The unquoted spelling of the same row, where no level holds a
			// quote or a brace and the sentence is the **outermost** level's:
			// the whole arithmetic expansion, quoted from the word. It wrote
			// nothing at all until the levels were unwound rather than
			// searched, because the innermost level with something open was
			// the only one asked (#3355).
			name: "an unquoted arithmetic level takes the word's own sentence",
			src:  "printf 'start\\n'\necho $(( $(for) + 1 ))\necho after\n",
			want: map[string]string{
				"zsh": "s.sh:2: parse error near `)'\n" +
					"s.sh:3: parse error near `$(( $(for) + 1 ))'\n",
			},
		},
		{
			// A substitution inside a substitution, with nothing open at
			// either level: the script's level writes the sentence and the
			// intermediate body writes nothing, and the sentence quotes the
			// *outer* word — which is neither the failing span nor anything
			// the body's runner ever held.
			//
			// The line is the one after the last line of the program rather
			// than one past the failure, which is the half a three-line
			// script cannot tell apart from the other rule. See
			// Runner.inRunSubstitutionBody.
			name: "a substitution nested in a substitution",
			src:  "printf 'start\\n'\necho A$(echo B$(for)C)D\necho after\necho after2\n",
			want: map[string]string{
				"zsh": "s.sh:2: parse error near `)'\n" +
					"s.sh:5: parse error near `A$(echo B$(for)C)D'\n",
			},
		},
		{
			// Two levels with something open, and they are written innermost
			// outward: the brace is the body's and the quote is the script's.
			// A build that stops at the first level writes one of them.
			name: "a brace inside a quote, one line each",
			src:  "printf 'start\\n'\necho \"x $(echo ${y:-$(for)})\"\n",
			want: map[string]string{
				"zsh": "s.sh:2: parse error near `)'\n" +
					"s.sh:3: closing brace expected\n" +
					"s.sh:3: unmatched \"\n",
			},
		},
		{
			// The same two contexts in the other order, which is the pair
			// that says the order is the levels' and not a ranking between
			// the two sentences.
			name: "a quote inside a brace, the other order",
			src:  "printf 'start\\n'\necho ${x:-$(echo \"$(for)\")}\n",
			want: map[string]string{
				"zsh": "s.sh:2: parse error near `)'\n" +
					"s.sh:3: unmatched \"\n" +
					"s.sh:3: closing brace expected\n",
			},
		},
		{
			// Three levels, each holding a quote of its own: three lines,
			// all at the one line. A walk that wrote the innermost level and
			// stopped wrote one.
			name: "three quoted levels write three lines",
			src:  "printf 'start\\n'\necho \"$(echo \"$(echo \"$(for)\")\")\"\n",
			want: map[string]string{
				"zsh": "s.sh:2: parse error near `)'\n" +
					"s.sh:3: unmatched \"\n" +
					"s.sh:3: unmatched \"\n" +
					"s.sh:3: unmatched \"\n",
			},
		},
		{
			// A level with a brace open and the script's level with nothing:
			// the brace's sentence and then the word's, which is the pair
			// that says a level with nothing open is not the end of the walk.
			name: "a brace level and then the word",
			src:  "printf 'start\\n'\necho $(echo ${y:-$(for)})\n",
			want: map[string]string{
				"zsh": "s.sh:2: parse error near `)'\n" +
					"s.sh:3: closing brace expected\n" +
					"s.sh:3: parse error near `$(echo ${y:-$(for)})...'\n",
			},
		},
		{
			// The word the quote starts at is on a line the failure is not
			// on: the outer body opens on line 1 and the inner one is
			// refused on line 2, and the sentence is still the outer word's
			// own line to its end.
			name: "the outer word begins on an earlier line",
			src:  "x=$(echo a\necho b; v=$(echo hi; for))\n",
			want: map[string]string{
				"zsh": "s.sh:2: parse error near `)'\n" +
					"s.sh:3: parse error near `x=$(echo a'\n",
			},
		},
		{
			// A **process substitution** body is a body of its own in the
			// same way, and it had no level at all: the sentence quotes the
			// word from its `<(`, which is a second opener the check for
			// where the substitution begins has to know (#3355).
			name: "the substitution is inside a process substitution",
			src:  "echo one\necho two\ncat <(v=$(echo hi; for))\n",
			want: map[string]string{
				"zsh": "s.sh:3: parse error near `)'\n" +
					"s.sh:4: parse error near `<(v=$(echo hi; for))...'\n",
			},
		},
		{
			// No newline at the end of the file, so the reader has none to
			// take and zsh's second line is not moved on. The word is exactly
			// twenty bytes, which that dialect marks although nothing is cut.
			name: "the last line has no newline",
			src:  "aaaaaaaaaaaaa=$(for)",
			want: map[string]string{
				"bash": "s.sh: line 1: syntax error near unexpected token `)'\n" +
					"s.sh: line 1: `aaaaaaaaaaaaa=$(for)'\n",
				"zsh": "s.sh:1: parse error near `)'\n" +
					"s.sh:1: parse error near `aaaaaaaaaaaaa=$(for)...'\n",
			},
		},
		{
			// A function read out of a sourced file and called from the
			// script quotes the *file's* line, which is what the text on the
			// function's origin is for: the script's line 4 is `f`. Standard
			// error only — bash refuses the body where the definition is read
			// and so never runs `echo two`, which is the parse-time change
			// Runner.runCommandSubst declines.
			name: "a function from a sourced file",
			src:  "echo one\n. ./lib.sh\necho two\nf\n",
			want: map[string]string{
				"bash": "./lib.sh: line 2: syntax error near unexpected token `)'\n" +
					"./lib.sh: line 2: `  q=1; v=$(echo hi; for); z=2'\n",
			},
		},
		{
			// A **process substitution's** body, which is read with the line
			// in the columns that read a `$( … )` body that way — and was
			// read only by the shell that ran it, so the line was not
			// refused and the two messages were the body's shell's rather
			// than the script's (#3962).
			//
			// bash alone, because it is the only column that both has the
			// spelling and reads a body with its line: dash and BusyBox ash
			// have no `<( … )` at all, and the three that read a body when
			// it runs are the separate divergence recorded in the row below.
			name: "a process substitution's body does not parse",
			src:  "printf 'start\\n'\ncat <(for)\necho after\n",
			want: map[string]string{
				"bash": "s.sh: line 2: syntax error near unexpected token `)'\n" +
					"s.sh: line 2: `cat <(for)'\n",
			},
		},
		{
			// And a `$( … )` **inside** one, which is the shape #3962 was
			// filed from. `false &&` is the control that makes it a
			// statement about reading rather than about running: the
			// substitution is never reached and the line is refused all the
			// same.
			name: "a substitution inside a process substitution, never reached",
			src:  "printf 'start\\n'\nfalse && cat <(v=$(echo hi; for))\necho after\n",
			want: map[string]string{
				"bash": "s.sh: line 2: syntax error near unexpected token `)'\n" +
					"s.sh: line 2: `false && cat <(v=$(echo hi; for))'\n",
			},
		},
		{
			// The older spelling's body is its own text, and it is quoted
			// in place of the script's line.
			name: "a backquoted body",
			src:  "printf 'start\\n'\nq=1; v=`echo hi; for`; z=2\n",
			want: map[string]string{
				"bash": "s.sh: command substitution: line 2: syntax error near unexpected token `newline'\n" +
					"s.sh: command substitution: line 2: `echo hi; for'\n",
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for name, want := range c.want {
				t.Run(name, func(t *testing.T) {
					dir := t.TempDir()
					write := func(file, text string) {
						if err := os.WriteFile(filepath.Join(dir, file), []byte(text), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					write("s.sh", c.src)
					write("lib.sh", "f() {\n  q=1; v=$(echo hi; for); z=2\n}\n")
					t.Chdir(dir)
					sh := echoShells()[name]
					var out, errs strings.Builder
					sh.Stdout, sh.Stderr = &out, &errs
					driver.MainArgs(sh, []string{sh.Name, "s.sh"})
					if got := errs.String(); got != want {
						t.Errorf("wrote\n%s\nwant\n%s", errs.String(), want)
					}
				})
			}
		})
	}
}

// Three routes the text in force is not the script's line for the span, where
// a quote read off the script would look plausible and be wrong. Each script
// is built so that the wrong line *would* pass the check that the text holds
// the substitution, or would be quoted outright without it — so each row
// fails if its route stops saying which text it runs.
//
// The first messages are not asserted: bash 5.3.20 tags the first two
// `exit trap:` and `command substitution:`, and places the third at line 3,
// and this engine does neither yet (#3354). What is pinned is the
// quote alone. Measured 2026-09-16, the same way as the table above.
func TestASubstitutionRefusalQuotesOnlyItsOwnText(t *testing.T) {
	for _, c := range []struct {
		name, src, want, never string
		// shell is the preset, bash where empty.
		shell string
	}{
		{
			// A trap body is text of its own. The script's line 3 carries
			// the body's text in a comment, and it is the line the body is
			// numbered from when it fires there.
			name:  "a trap body",
			src:   "echo one\ntrap 'v=$(echo hi; for)' EXIT\necho two; exit # v=$(echo hi; for)\n",
			want:  "`v=$(echo hi; for)'\n",
			never: "echo two",
		},
		{
			// And in the dialect that reads a trap's action when the trap is
			// set: zsh 5.9.2 refuses it there — `couldn't parse trap command`
			// — and never fires it, so no second message of the word kind is
			// its to write from inside a body that fired.
			name:  "a trap body, in the dialect that quotes the word",
			shell: "zsh",
			src:   "echo one\ntrap 'v=$(echo hi; for)' EXIT\necho two\n",
			never: "near `v=",
		},
		{
			// Nor from inside a function body, where that dialect locates a
			// message by the function: it reads the body at the definition
			// and numbers its second message in the *file* — `s.sh:4` for a
			// body on line 3 — which a body refused when it is called cannot
			// say. `f:2` would be a place zsh never names.
			name:  "a function body, in the dialect that quotes the word",
			shell: "zsh",
			src:   "echo one\nf() {\n  q=1; v=$(echo hi; for); z=2\n}\nf\n",
			never: "near `v=",
		},
		{
			// A `$( … )` inside the older spelling's body quotes that body,
			// which is the text it was read from: bash 5.3.20 writes
			// “ `echo $(for)' “, and the script's line holds `$(for` too.
			name:  "a backquoted body holding a substitution",
			src:   "q=1; v=`echo $(for)`; z=2\n",
			want:  "`echo $(for)'\n",
			never: "q=1",
		},
		{
			// A process substitution's body is numbered from its own first
			// line here, so the refusal lands on the script's line 1, which
			// does not hold it. bash quotes line 3; writing nothing is the
			// honest answer until the body is placed, and quoting line 1 is
			// the wrong one.
			name:  "a process substitution's body",
			src:   "echo one\necho two\ncat <(v=$(echo hi; for))\n",
			never: "`echo one'",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "s.sh"), []byte(c.src), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Chdir(dir)
			sh := bashShell()
			if c.shell != "" {
				sh = echoShells()[c.shell]
			}
			// Locked, because a process substitution's body writes from a
			// goroutine of its own. `cat` has read the body to its end before
			// the script finishes, so what it wrote is there to read.
			var out, errs lockedText
			sh.Stdout, sh.Stderr = &out, &errs
			driver.MainArgs(sh, []string{sh.Name, "s.sh"})
			got := errs.String()
			if !strings.Contains(got, "`)'") {
				// Without the refusal the rows below pass for a shell that
				// never reached the body at all.
				t.Fatalf("wrote\n%s\nwith no refusal of the body in it", got)
			}
			if c.want != "" && !strings.HasSuffix(got, c.want) {
				t.Errorf("wrote\n%s\nwant it to end with the quote %q", got, c.want)
			}
			if strings.Contains(got, c.never) {
				t.Errorf("wrote\n%s\nwhich quotes %q, a line of the script rather than of the text that failed", got, c.never)
			}
		})
	}
}

// lockedText is standard output or error collected from every goroutine the
// shell writes from.
type lockedText struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedText) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedText) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func echoShells() map[string]driver.Shell {
	return map[string]driver.Shell{
		"bash": bashShell(),
		"zsh":  zshShell(),
		"ksh":  kshShell(),
		"dash": dashShell(),
		"ash": {
			Name: "ash", Dialect: ash.Dialect(), Semantics: ash.Semantics(),
			Diagnostics: ash.Diagnostics(), Prelude: ash.Prelude(), Register: ash.Apply,
			PromptStyle: ash.PromptStyle(),
		},
	}
}
