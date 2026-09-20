// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The panel has seven columns and the dialect set has five, and for a long
// time the axis doc comments were written as though both were four.
//
// It is not a style complaint. A comment that says what four shells do is also
// a claim about what our fifth dialect should answer, and a reader who
// consults one gets a group with a column missing from it. That is how #3226
// happened: EchoExpandsHexEscapes read "bash and zsh do, dash and ksh93 print
// it as written", BusyBox ash — which expands it — was in neither list, and
// the value was left at a default saying it does not, while the corpus row
// beside it already held the right answer. Reading the same comments against
// the shell turned up four more live defects: #3237, #3238, #3239, #3245.
//
// The drift is mechanical, so the guard is a ratchet rather than a rule: these
// phrases may only become rarer. A new one cannot be added, and the count goes
// down as the remaining comments are measured and rewritten. #3228 is the
// campaign; this is what keeps the next column addition from starting it
// again.
//
// Deliberately a count and not a ban. A number of the matches are honest —
// "all four combinations", "a type of its own with four answers", "the four
// columns without the construct" — and a rule that could not tell those from a
// miscount would be satisfied by rewording rather than by measuring. A ceiling
// that only falls needs no such judgement, and the budgets below are the whole
// audit trail.
//
// # One budget per file, and the third file
//
// interp/diagnostics.go was outside this guard until #3228 was re-read
// against it, and it held **54** of these lines — more than the two guarded
// files together. Its Answer axes are the same kind of claim in the same
// words: "CdStatus is what that reports. dash says 2 and the other three say
// 1." A ratchet that covers half a population is not a ratchet; new prose
// simply lands in the half it does not read.
//
// That sentence is the first one the file's own pass corrected: dash and
// BusyBox ash both say 2, and dialect/ash already held 2 throughout. Nine of
// its lines were measured across all seven columns on 2026-09-19 and
// rewritten, which is what takes the budget from 54 to 45 — every dialect
// value checked was right, so what the pass produced is a corrected record
// rather than a defect. Two findings were not about ash at all: the same bash
// binary called `sh` is silent where bash announces an empty hash table, and
// drops the `alias ` prefix from a listing, so two fields documented as
// "bash" are bash-as-bash.
//
// A second pass the same day measured eight more across all seven columns and
// takes the budget from 45 to 37. Every dialect value it checked was right
// again, so it too is a corrected record; what it changed is what a reader is
// told. Three of the eight were wrong about more than a count:
//
//   - ReturnOutsideAFunction said "the other three obey it and end the
//     script". Four columns obey it, and bash — the shell the sentence was
//     about — does not: it complains and runs the next command at status 0,
//     so it was in neither group while appearing to be the subject of both.
//   - TrapBadSignal said status 1 was "the only part of this any two of them
//     agree on". BusyBox ash writes bash's wording and ksh93 writes dash's,
//     so the wordings pair up across the family lines and the sentence was
//     false in exactly the direction the missing column supplies.
//   - UnboundPositional said "true of three of the four", and the count was
//     right by accident: the three are dash, zsh and ash, and ksh93 does not
//     refuse an unset positional under `set -u` at all. A column with nothing
//     to word had been filed as a column that words it the same way.
//
// LowercaseReason is the one that had to be measured twice, and it is the
// reminder that a probe has to discriminate: ash's other reasons read
// lowercase and look exactly like that flag, and only a reason it does not
// substitute — Permission denied — tells the two apart. It capitalizes, so
// zsh really is alone.
//
// # The panel is seven columns and one of them is an invocation
//
// Two of this pass's eight had to be corrected *after* they were written,
// because the first probe set `argv[0]` on `env` rather than on the shell it
// was about to exec — so what it called bash-as-`sh` was an ordinary bash and
// the column read as a duplicate of the one beside it. Running the same binary
// through a link named `sh` puts it in POSIX mode, and two of the eight split:
//
//   - ReturnOutsideAFunction: bash and bash 3.2 complain and run the next
//     command at 0; the same binary called `sh` complains and ends the script
//     at 2. A third behavior neither group holds.
//   - TrapPrintsSignalPrefix: bash and bash 3.2 write `SIGINT`; the same
//     binary called `sh` writes `INT`, with everybody else.
//
// That is the same class as the previous pass's `HashEmptyTable` and
// `AliasListPrefix`, and it is worth stating in its own right: **the column a
// four-shell sentence leaves out is usually BusyBox ash and is not always**.
// #3228's own framing says "almost always", and a reader who takes that as
// "always" will check ash, find it agrees, and file the sentence as correct
// while a bash column two rows over disagrees. A probe that cannot tell
// bash-as-`sh` from bash cannot see any of these, and it looks exactly like a
// probe that can.
//
// # The pass that found a live defect rather than a corrected record
//
// A third pass, 2026-09-19, is the first against semantics.go rather than
// diagnostics.go, and takes that file's budget from 41 to 33. Seven of the
// eight were the usual shape, a missing fifth dialect in a sentence whose
// dialect values were all right: `ReadTrailingEscapedSeparator` gained ash
// and zsh rows and a
// third bash cell, `ArithSubscriptSkippedWhenNameUnset` found that **two**
// columns have no subscript in arithmetic rather than one, `unset "b[0]"` on
// an unset name is quiet in five columns and fatal in the two that read the
// brackets as part of the name, `StdinOptionNamesTheOperands` and
// `PlusSignedCommandStringIsDollarZero` each gained ash on bash's side, and
// `StartupFileOptions.Login` was one dialect short in both of its counts —
// BusyBox ash takes `-l` *and* `--login`, and takes them as a login shell,
// measured with a scratch `/etc/profile` and `$HOME/.profile` it then reads.
//
// `LocalOutsideAFunctionIsAnError` is this pass's count-right-by-accident, the
// same shape as `UnboundPositional` above: "three of the four" is true of the
// four dialects that **have** `local` — bash, dash and ash say so and zsh does
// not — while ksh93, the column a reader would put fourth, has no `local` at
// all and answers `not found` at 127.
//
// The eighth is why this comment has a section. `DotWithNoOperandIsAnError`
// said the panel "splits four ways on `.` alone". It splits **six** ways, and
// the row that was missing is a **bash** one: bash-as-`sh` writes bash's two
// lines, exits 2 and **ends the script**, where bash and bash 3.2 run on.
// `set -o posix` does the same to plain bash, so it is the POSIX rule about a
// special builtin's usage error rather than anything about `argv[0]` — and
// this shell carries on under both, which is #3818. Every previous pass on
// this budget produced a corrected record and no defect; this one produced a
// defect precisely where the count was smallest, in the column the phrase set
// was written to find.
//
// # The fourth pass, and the two readings that were wrong about a bash column
//
// 2026-09-19, six more across all seven columns, taking diagnostics.go from
// 37 to 31. Two of the six are not count corrections:
//
//   - ScriptLocation said "ksh93 is the only shell in the panel where they
//     differ" and "true of the other three". **Both halves are wrong about
//     the same column**: BusyBox ash differs too — `ash -c` writes a parse
//     failure with no line at all where the same two lines in a file are
//     `line 2`, and a runtime failure counts the command string's own lines
//     rather than the file's. dialect/ash has held LocationLineWord since the
//     column was written, so the value was right and the sentence counted one
//     short.
//   - ParamNullOrNotSet's fallback said `parameter not set` "is what plain
//     ${x?} says in all four". **bash 3.2.57 says `parameter null or not
//     set` for the plain form too**, using one sentence where 5.3 uses two —
//     so the exception is a bash column rather than the missing dialect, and
//     a reader taking the sentence at its word would have gone looking in the
//     dialect vector, where it is not. BusyBox ash is the missing dialect and
//     writes dash's wording.
//
// ForArithHeaderNoPart is the one where no count of four was available at
// all: two columns never reach the question (dash and ash have no C-style
// `for`), one has no answer to give (ksh93u+ 2012 **faults** on `for ((;))`
// where `for ((;2))` is an ordinary syntax error), bash says the same either
// way in all three of its columns, and zsh is the dissenter the field exists
// for. CommandStringParsedWhole, SelfName and the `${@:=abc}` status were
// count corrections with the membership measured: ash is in bash's group for
// the first two, and it is **with dash** on the third, exiting 2 where the
// other five exit 1.
//
// # The fifth pass, and the two sentences that were one dialect short
//
// 2026-09-20, five more measured across all seven columns with `env -i
// PATH=/usr/bin:/bin LC_ALL=C <shell> case.sh` over a script file, taking
// diagnostics.go from 22 to 17. Every dialect value checked was right again,
// so this pass too is a corrected record rather than a defect — but two of
// the five were wrong about **membership** and not only about the count:
//
//   - SyntaxUnexpectedWord and SyntaxRedirectUnexpected each filed dash as
//     the lone dissenter. BusyBox ash makes the same two distinctions with
//     the words the other way round: `syntax error: unexpected word
//     (expecting "do")` against dash's `Syntax error: word unexpected
//     (expecting "do")`, and `syntax error: unexpected redirection` against
//     `Syntax error: redirection unexpected`. dialect/ash has held both
//     wordings since the column was written, so a reader consulting the
//     sentence would have gone looking for a missing value that is there.
//
// ArithFailureStatus is this pass's count-wrong-about-bash: the sentence
// credited the non-zero answer to bash alone, and **ksh93 sets it too** —
// `echo $((1 @))` is status 1 there, where ksh93's general syntax-error
// status is 3. So the column the count left out is the one that most needs
// the field, and the sentence named it as one of the columns that do not.
//
// FunctionListingKeywordHeader and ForArithHeaderEcho are the ordinary shape:
// the two dialects the count left out never reach either field at all — dash
// and BusyBox ash have no `typeset` and no C-style `for` — which is a
// different answer from agreeing with the group they had been filed under.
//
// # The sixth pass, the first on semantics.go since the third, and three
// kinds of line
//
// 2026-09-20, the same day as the fifth and on the other file: seven lines,
// taking semantics.go from 14 to 7. They fall into
// three kinds, and saying which is which is the point of the section, because
// only one of them is a measurement:
//
//   - **Measured, and the count was a dialect short.**
//     InteractiveMonitorNeedsATerminal said a shell with no terminal does not
//     report a monitor in three of four. Measured with `<shell> -i case.sh`
//     and no terminal on any descriptor, the script reading its own `$-`:
//     dash, bash 5.3, bash-as-`sh`, bash 3.2, zsh 5.9.2 and BusyBox ash
//     report no `m` and **ksh93u+ reports one** — six columns against one, and
//     dialect/ksh has held No since the field was written. Its note in the
//     POSIX preset carried the same count and is corrected with it.
//     ExportTakesTheAttributeOff's count was right and its members were not
//     named: `export -n x` is `Illegal option -n` at 2 in dash, `-n: unknown
//     option` with a usage line at 2 in ksh93u+, and `bad option: -n` at 1 in
//     zsh with the next command run. HeredocBody said `all four columns of
//     the panel print the body`; re-measured, `sh -c 'cat <&3' 3<<X` writes
//     both lines in **all seven**.
//
//   - **The surrounding prose already named the columns**, so the number was
//     redundant and wrong at once. KillSendsASignalNumberItCannotName's
//     `#3139 was the other three` is followed in the same sentence by `ksh93,
//     zsh and ash`, and the `command -v` note's `in all four dialects` is
//     followed by `bash, zsh, dash and ash` and `ksh93`. Both are now written
//     with the names alone. No measurement is claimed for either, because
//     none was needed and claiming one would be the worse error.
//
//   - **Not a claim about the panel at all.** StartupFileOptions' `bash has
//     three of the four` counts the *four fields of the struct*, which is the
//     same false-positive class that took syntax/dialect.go from eleven to
//     zero. Reworded so it stops reading as a panel count, rather than added
//     to fourShellNotAPanel: a blocklist entry hides the sentence from the
//     guard and leaves it reading the same way to a person.
//
// # The seventh pass, which measured nothing and says so
//
// 2026-09-20, five lines that needed no shell run at all, taking
// diagnostics.go from 17 to 15 and semantics.go from 7 to 4. Every one of
// them is a count the file had **already** corrected or already spelled out
// in names, and the only reason they were still here is that the correction
// was written in a way the ratchet reads:
//
//   - **Two were the corrections themselves, quoting the miscount back.**
//     ReturnOutsideAFunction and UnboundPositional each open their corrected
//     record by repeating the old sentence in quotation marks, so the phrase
//     the audit is counting survived the fix that removed it. Both are
//     rewritten to describe the old count rather than reproduce it — which
//     is the general rule and is now written into the first of them: a
//     reader skimming for the answer finds a quoted miscount as readily as
//     the correction under it.
//   - **Two were a count the same sentence then spelled out.** The
//     `set -o --` row of SetODeclinesADashWord names its three refusers on
//     the two rows above and below it, and LongOptionNamesASetOption's
//     closing line points at a group its own paragraph opens by naming. Both
//     now carry the names. No measurement is claimed for either.
//   - **One was a counted subset written with the wrong joining word.**
//     SetValidatesOptionLettersFirst's probe note explains why it puts
//     `command` in front of the builtin, and the four columns it counts are
//     chosen by a criterion it states in the same breath. It said `columns
//     where`, and fourShellSubset reads `columns that` — so the sentence was
//     already the shape this audit wants and was being counted as the shape
//     it does not. Reworded rather than added to the subset pattern: one
//     conjunction is a narrower change than widening what the guard forgives
//     for every line in the tree, and `columns where` appears on five other
//     lines that this pass did not measure.
//
// Worth stating because the three kinds look identical from the count and
// are three different things to a reader: a fix that left its own evidence
// behind, a number standing in front of the names it stands for, and prose
// that was right and read wrong.
//
// # The eighth pass, and the first live defect this campaign has produced
//
// 2026-09-20, six lines of diagnostics.go measured across all seven columns
// with `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> case.sh` over a script
// file, taking that file from 15 to 9. Four are corrected records — the
// dialect value was right and the sentence was not — and **two are not**:
//
//   - **DuplicationSourceNotOpen is a defect, and the probe that hid it is
//     the finding.** BusyBox ash writes `dup2(19,0): Bad file descriptor`
//     for `cat <&19`: the syscall, the source *and the target*, where this
//     field carries the source and the reason. dialect/ash leaves it empty,
//     so this shell writes bash's sentence there — #3909. The earlier
//     measurement was taken at descriptor **10**, and 10 is the one number
//     that cannot answer the question: in a script file BusyBox answers
//     `10: Bad file descriptor` there, bash's shape exactly, because 10 is
//     where it keeps the script itself. Every other number, and 10 itself
//     under `-c`, answers `dup2(N,M)`. A non-discriminating probe read the
//     column as agreeing with bash for as long as the field has existed.
//   - **ReadonlyVariableInDeclaration turned up a second one on the way
//     past.** Two columns name the builtin and they name it in different
//     places: dash in the sentence, which is this field, and BusyBox ash in
//     its *location* — `export: line 2: q: is read only`. dialect/ash holds
//     the right wording and the right BuiltinLocation, and `cd` reaches it
//     byte for byte, but the readonly refusal raised from inside a
//     declaration builtin does not — #3908.
//
// The four corrected records are the ordinary shape and two of them are
// worth naming for the column they left out. FileSubstitutionReadError
// listed the silent columns as bash 5.3, bash 3.2 and ksh93 and left out
// **bash-as-`sh`** — a bash column again, the third time this campaign has
// found the missing one there rather than in ash. And it is the field where
// the status cannot decide the question: dash and BusyBox ash have no
// `$(<file)` at all, but `v=$(<f)` is a command substitution holding nothing
// but a redirection, so it runs, succeeds and produces the empty string — a
// probe reading the status sees agreement and a probe reading the value sees
// two columns that never had the construct.
//
// ArithBadOperator and UnmatchedNearMaxBytes were counts one dialect short,
// and reasonText's was six columns short of the seven that capitalize a
// reason. That last one had to be measured on `Permission denied` and could
// not have been measured on a missing file: dash writes `No such file` and
// BusyBox ash writes `no such file`, each its own sentence rather than the
// errno, so ash reads as a second lowercasing column on that probe alone.
// That is the same discrimination LowercaseReason itself needed two passes
// ago, rediscovered from the other side.
//
// # The ninth pass, and the two routes the corpus cannot reach
//
// 2026-09-20, five lines of diagnostics.go taking it from 9 to 4. Every
// dialect value checked was right, so this pass is a corrected record — but
// it is the first one whose probes are not `-c` and not a script file, and
// that is the part worth keeping:
//
//   - **The prompt route wanted `-i` with the program on a pipe.** Three
//     lines — PromptCountsTheSessionsLines, the Prompt wordings' preamble
//     and ForPrompt's own — each said three of four write no line at a
//     prompt. Six of the seven columns write none, and dash is alone:
//     `dash: 2: Syntax error` and `dash: 1: … not found` against a bare
//     `bash:`, `sh:`, `ksh:`, `zsh:` and BusyBox's `/bin/sh:` with no number
//     anywhere, on a parse failure and a missing command alike.
//   - **The job-control route wanted a pseudo-terminal**, and BusyBox ash
//     wanted a container with one — `docker run -t`. JobDoneNotice's four
//     columns that word a finished job word it once and only ksh93 needs a
//     second word, which is measured by `jobs` *after* the job has ended:
//     `Done` in bash's three columns, dash and ash, and `Running` in ksh93,
//     which is the whole reason the field exists. zsh is not a fifth answer
//     there — it has forgotten the job by the time the listing runs, so it
//     has nothing to say rather than a different thing to say.
//
// JobResumedInForeground is the one where a column produced **no row at
// all**, and saying so is the point: `sleep 0.4 &` then `fg` is `sleep 0.4`
// in bash 5.3, bash-as-`sh`, dash and BusyBox ash and `sleep 0.4 ` in
// ksh93u+, while **bash 3.2 declines job control on that arrangement** and
// answers `fg: no job control`. A column that cannot be asked is not a
// column that agreed, and rolling it into the count is how a four-shell
// sentence gets written in the first place.
//
// Per file rather than one total, because a single number lets a file that
// gets worse hide behind a file that gets better — and these three are worked
// on separately, so that trade would be made by accident rather than chosen.
var fourShellPhraseBudget = map[string]int{
	"semantics.go":   4,
	"diagnostics.go": 4,
	filepath.Join("..", "syntax", "dialect.go"): 0,
}

// fourShellPanel is the phrase set from the audit that filed #3228. Each one
// names a panel of four where the panel is seven.
//
// **The noun after the number decides it**, and getting that wrong was most
// of this check. The first spelling matched `all four` and `the other three`
// bare, which caught sentence after sentence making no claim about the panel:
// `all four combinations` of a name count and a body spelling, `the other
// three flags showing through`, `ksh93 takes all four` of the quoting
// constructs, `a header written over four lines with all four of them`, and
// four separate `four answers` that are the values of an enum. In
// syntax/dialect.go it was **eleven matches out of eleven** — the file's whole
// budget was false, and driving it to zero would have meant rewording prose
// that is already right.
//
// So the number is read with what it counts. A panel noun or an elided one
// counts; a blocklisted noun does not.
var fourShellPanel = regexp.MustCompile(
	`\bthe other three\b|\bthree of the four\b|\b(?:all |the )four (?:columns|shells|dialects|of them)\b`)

// fourShellNotAPanel is what the number turned out to be counting instead.
//
// Every entry was found in the tree rather than imagined: each is a real
// sentence the bare phrase matched and should not have.
var fourShellNotAPanel = regexp.MustCompile(
	`\b(?:the other three|four) (?:flags|rules|combinations|lines|rows|answers|` +
		`constructs|spellings|letters|words|characters)\b|` +
		`\bwritten over four lines\b|\bthe other three would\b`)

// fourShellSubset is a panel phrase that names *which* four, which is a
// counted subset of the seven rather than a claim that the panel is four.
//
// `the four columns without the construct`, `all four shells that have =~`,
// `the four shells that read through a double quote` — each says how its four
// were chosen, so no reader is left with a four-shell picture of the panel.
// These are the sentences this audit wants written, not the ones it wants
// found.
var fourShellSubset = regexp.MustCompile(
	`\b(?:columns|shells|dialects|of them) (?:that|without|with|which|having|holding)\b`)

func TestNoNewFourShellPanelInAnAxisDoc(t *testing.T) {
	for _, path := range slices.Sorted(maps.Keys(fourShellPhraseBudget)) {
		budget := fourShellPhraseBudget[path]
		n := fourShellPanelLines(t, path)
		t.Logf("%s: %d of %d", path, n, budget)
		switch {
		case n > budget:
			t.Errorf("%s: %d doc lines describe a four-shell panel, where the budget is %d.\n"+
				"The panel is seven columns and the dialect set is five. Name the "+
				"columns, or measure the one the sentence leaves out — it is "+
				"almost always BusyBox ash, and it is the column this project has "+
				"had wrong five separate times (#3228).", path, n, budget)
		case n < budget:
			t.Errorf("%s: %d doc lines describe a four-shell panel, where the budget is %d.\n"+
				"Lower this file's entry in fourShellPhraseBudget to %d: the ratchet "+
				"only holds while the number it holds is the real one.", path, n, budget, n)
		}
	}
}

// fourShellPanelLines counts the comment lines in one file that describe a
// panel of four.
//
// A file that cannot be read is a failure rather than a zero. A ratchet whose
// count silently becomes nought on a rename is a green check for a rule that
// stopped applying, which is the same shape as the population it was not
// reading.
func fourShellPanelLines(t *testing.T, path string) int {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var n int
	for _, line := range strings.Split(string(src), "\n") {
		// Comment lines only. The phrases are prose, and a string literal
		// holding one would be a wording rather than a claim about the panel.
		if !strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if fourShellPanel.MatchString(line) &&
			!fourShellSubset.MatchString(line) &&
			!fourShellNotAPanel.MatchString(line) {
			n++
		}
	}
	return n
}
