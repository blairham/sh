# Differences kept by design

Every case listed in `internal/oracle/testdata/conformance/<dialect>.txt` is
one this shell does not match exactly, and this file gives each one its
reason: the reference shell crashes or hangs, the panel's own builds
disagree, the difference is the C library or the harness rather than the
shell, or it is a deliberate choice. A listed case with no reason here fails
`TestConformanceListsHaveAReason`, so the lists cannot grow a case nobody has
decided about.

The tags name the reference: `[b]` bash, `[d]` dash, `[k]` ksh93, `[z]` zsh,
`[ash]` BusyBox ash. Entries are grouped by the lane that decided them.
Revisiting one means deciding to copy the reference's behavior after all;
when that lands, remove the case from its list and its entry from here.

A few entries are not conformance cases — a measured difference with no
oracle row — and are kept here for the same reason.

This ledger was kept on issue #5761 until 2026-10-06.

## From L3 (#5714)

- `complist/the-widget-the-module-defines` [z] — sh deliberately has no `menu-select` widget; the line editor has no menu selection (the case's own Why says so).
- `zmodload/the-selection-as-the-command-that-would-make-it` [z] — the only difference is a missing trailing newline the corpus records as a zsh bug sh does not copy.

## From L2 (#5713)

- `expand/tilde-after-a-home-assignment` [b]: the panel's bash builds disagree. Only 5.3.20 caches `HOME`, and 5.3.15 reads the variable like every other shell. `TildeReadsACachedHome` was declined deliberately (#3484, #4039, #4156).
- `expand/tilde-after-a-home-assignment-reaches-every-road` [b]: the same 5.3.20-only `HOME` cache.
- `expand/tilde-with-no-home-at-all` [b half]: the same 5.3.20-only cache, refreshed from the password database. The ksh half is still open in L2.
- `param/prompt-percent-refuses-an-escape-by-name` [z]: refusing `%z` by name is a deliberate sh choice. A wrong answer that looks like success is worse than a complaint.
- `pat/a-whole-match-flag-in-mid-pattern` [z]: a deliberate divergence. zsh's two answers depend on how the pattern is compiled, which cannot be derived without the implementation (CLEANROOM.md).

## From L7 (#5718, ash)

- `axis/printf-non-finite-is-converted` [ash] — the only gap is `[%+f]` of a NaN printing `+nan`. That is musl's formatter, not the language; `PrintfNonFiniteIsConverted`'s doc records that sh writes `nan` unsigned everywhere.
- `printf/non-finite-is-spelled-c-s-way` [ash] — the only gap is `nan(a-b)`/`nan(*)`, which musl's `strtod` refuses and BSD's accepts. The case's own Why says it is "a C library rather than a language".
- `harness/a-dashed-argv-zero-is-a-login-shell` [ash] — the reference's `applet not found` at 127 comes from the BusyBox multi-call dispatcher, not from ash.
- `harness/an-undashed-argv-zero-is-not-a-login-shell` [ash] — same: BusyBox's `applet not found` dispatcher.
- `invoke/a-name-that-is-not-sh-does-not-start-in-posix-mode` [ash] — same: BusyBox's `applet not found` dispatcher.
- `invoke/a-name-that-is-not-sh-leaves-a-special-builtins-usage-error-alone` [ash] — same: BusyBox's `applet not found` dispatcher.
- `kill/signaling-a-process-that-is-not-ours` [ash] — harness artifact: `kill -TERM 1` inside the container kills the oracle runner, which is pid 1 there.
- `jobs/a-background-job-outlives-the-shell` [ash] — the same deliberate, pinned difference L5 records for the other four (#519): a `&` body is a goroutine, so the job ends with the process.
- `jobspec/wait-for-the-next-job` [ash] — timing: BusyBox answers 129 only because its forked `(exit 3)` is still running when `wait -n` starts (`(exit 3) & sleep 0.2; wait -n` is 0 there); this shell's job has ended before the next builtin runs. The reading itself, `WaitNextJobFirstToSucceed`, is modeled.
- `printf/c-ignores-a-precision` [ash] — a defect of BusyBox printf's star handling: a `*` in a `%c` prints a literal `*` and takes the field from the next operand's first character code (`printf '[%*c]' 4 abc` is 96 blanks and `*`). Not a reading of the format.
- `pat/a-collating-body-that-is-not-an-element` [ash] — the C library's fnmatch: ash's `case` answers what `find -name` answers in the same image over the whole grid (closed and unclosed `[.` bodies, collating range bounds). Not modeled per dialect, for #956's reason.
- `pat/a-collating-element-as-a-range-bound` [ash] — the same: musl's fnmatch, matched cell for cell by `find -name`.
- `pat/alnum-outside-ascii-is-a-letter-or-a-decimal-digit` [ash] — musl's iswctype tables (Nl and non-ASCII Nd are alpha, Lt is upper and lower, Mn and No are punct). interp/pattern.go records (#956) that a class outside ASCII is the host C library's table and is not modeled per dialect.
- `param/a-replacement-runs-over-characters` [ash] — musl's fnmatch ignores a trailing incomplete UTF-8 sequence, so `?` matches `日` plus two bytes of `本` and the replacement eats them; `find -name '?'` matches the same truncated names in the pinned image. The C library, not the language.
- `variable/a-seeded-random-draws-a-sequence-of-its-own` [ash] — BusyBox's own generator. `RANDOM` is wired for ash (produced, seeded, listed), but its sequence (`RANDOM=42` draws 20351 9206) has not been fitted from black-box draws; a seeded script here is reproducible with numbers of its own.
- `heredoc/stripping-and-joining-are-ordered-and-the-panel-parts-over-which-comes-first` [ash] — recorded and deliberately not modeled (#2430, and the case's own Why): the `<<-` line of a tab and a backslash parts the panel three ways, and dash and ash keep the backslash-newline outright and join nothing.
- `trap/an-elements-pipe-handler-runs-inside-the-elements-redirections` [ash] — harness naming: since #5787 the element writes `write error: Broken pipe` behind the shell's own name as BusyBox does, and the only difference left is that name in the middle of a line — the reference's applet is `ash` and ours runs as `our-ash`, which the oracle normalizes only at the start of a line.

## From L4 (#5715, printf and arithmetic)

- `printf/non-finite-is-spelled-c-s-way` [k]: a build defect. The macOS ksh93u+ evaluator reads its `inf`/`nan` constants as `-0`/`-2` but writes its own computed infinity as `inf`. #2729 chose not to reproduce this; see `PrintfNonFiniteIsConverted`.
- `axis/printf-non-finite-is-converted` [k]: the same `inf`→`-0`, `nan`→`-2` evaluator defect.
- `axis/printf-c99-float-conversions` [k]: the same defect, on the `[%F][%A]` line for `inf nan`.
- `axis/printf-star-beside-the-field-digits` [d]: macOS libc undefined behavior. `%5*d` takes one operand as the width and always prints `1`; C has no grammar for this, and glibc and musl would answer differently.

## From L6 (#5717)

- `invoke/a-program-on-a-standard-input-that-is-closed` [b]: cannot be met in Go. The runtime's checkfds reopens a closed fd 0 on /dev/null before `main`, so a closed stdin looks the same as `</dev/null`.
- `diag/trap-refusing-a-condition-it-does-not-know` [z]: the reference segfaults on `trap '' 99`, and sh does not reproduce a crash.
- `print/a-prompt-escape-this-shell-has-not` [z]: a deliberate divergence. The case's own Why says naming the missing `%q` escape is the choice here.
- `commands/an-and-or-with-a-terminator-after-it` [k]: ksh93 parses `: || & echo two` and runs neither side, and `: || & echo two > f` then refuses the `>`, so what it read `echo two` as is not a command. The case's own Why says recorded rather than implemented.
- `core/a-bare-brace-in-a-nested-quoted-operand` [k]: the case's own Why says no dialect answers for it. ksh93 balances a bare brace inside a quoted nested operand of a here-document body and refuses at expansion time; every other column reads it as quoted content.
- `core/appending-an-empty-array-literal-to-a-scalar` [k]: ksh93 keeps the scalar as an unnamed `.` member that `${a}` lists (`( .=1 )`) while `typeset -p a` prints `typeset -C a=()`. Its two listings disagree, so there is no single model to copy.
- `help/the-help-option-is-the-whole-word-and-stands-where-an-option-stands` [k]: `alias --help=x` prints ksh93's built-in documentation for option `x` (`OPTIONS` / `-x  Ignored, this option is obsolete.`). sh carries no ksh93 builtin documentation text.
- `assignment/arithmetic-that-will-not-parse` [k]: ksh93's lexer, not an arithmetic rule. Any brace inside `$(( ))` throws the read off: `$(( {} ))` and `$(( 1 }+2 ))` refuse naming `)`, `$(( (}) ))` and `$(( x[}] ))` name `}`, and `$(( "}" ))` prints ` ))`.
- `assignment/arithmetic-that-will-not-parse-in-a-command` [k]: the same lexer artifact.
- `core/a-bare-brace-inside-a-quoted-expansion` [k]: only ksh93's `-c` reading of an operand that takes the rest of the input is involved. What remains is quote parity inside that remainder: `"; printf "[%s]" "${a:-p{q}r}"; echo` loses its `; echo` here. Every row that does not nest a quoted expansion inside the swallowed text matches.

## From L5 (#5716)

- `jobs/a-background-job-outlives-the-shell` [bzdk]: a deliberate, pinned difference (#519, driver/backgroundoutlives_test.go). A `&` body is a goroutine on a cloned Runner; Go cannot fork, so the job ends with the process.
- `jobspec/kill-a-job-that-is-not-there` [k]: the reference segfaults (`killed by signal 11`); sh does not reproduce a crash.
- `pipe/both-streams-takes-no-blank-between-its-two-bytes` [k]: ksh93's `| &` starts a coshell; the recorded silence depends on the machine (re-measured 2026-10-03: `unable to create namespace`). The case's own Why calls it a resource failure.
- `jobs/a-stopped-background-job-in-the-listing` [k]: ksh93's `sleep` is a builtin, so the job here is an in-process body with no process for SIGSTOP to stop. The bash half is fixed in #5762.

- `expand/tilde-with-no-home-at-all` [k half]: with no `HOME`, ksh93 expands `~` to the session's login name (`logname`), which follows whoever owns the controlling terminal. Pinning it would record the machine, not the shell. Recorded rather than modeled in dialect/ksh (#4179).

## From L1 (#5712)

- `assoc/a-table-lists-in-a-different-order-than-it-expands` [b] — the only difference is the key order of bash's listing, which comes from its hash table. That order is visible only in the source CLEANROOM.md forbids reading, so a script cannot rely on it and sh will not copy it.
- `decl/an-index-array-literal-replacing-a-table` [b] — same cause: `declare -A n=([p]="q" [a]="1" )` against our `[a] [p]`, with every value correct.
- `array/a-literal-value-holding-a-bracket` [k] — sh does this on purpose. ksh93 lets a bracket anywhere in a bare literal's element take in blanks (`a=( x[1 2] )` is one element), but not in a `typeset` operand. `opensArrayElementSubscript` documents that this extra reach is recorded and not implemented.
- `array/a-name-before-a-literal-elements-bracket` [k] — same decision: `pre[1 2]=x` is a subscripted element only in ksh93. The case's own Why says "Recorded and deliberately not matched".
- `assoc/a-subscript-search-is-a-list-of-its-matches` [k] — this is zsh-only syntax. Unquoted, ksh93's lexer rejects the whole line at `${(o)m[(I)a*]}` with "`(' unexpected" before anything runs. Quoted, or without the inner `(`, ksh93 runs earlier commands and then fails, just as sh does. Copying that quirk of an invalid construct buys nothing.
- `declare/f-says-a-nested-declaration-back` [k] — a ksh93u+ listing bug. `typeset -f f` cuts the body off at the first nested function's `};` (`f() { g() { :; }; h() …` prints `f() { g() { :; };`), so the listing does not read back. sh prints the whole body.
- `declare/f-says-a-nested-keyword-declaration-back` [k] — the same truncation, with `function inner {`.
- `declare/an-attribute-does-not-leave-a-subshell` [k] — `(typeset -u x)` in ksh93u+'s virtual subshell leaks the case attribute into the parent (`x=def` then reads `DEF`), while `-i` does not leak. Copying this would break subshell isolation.
- `decl/a-subscripted-operand-with-no-value` [k] — a ksh93u+ listing quirk. After `typeset a[3]`, ksh counts no elements (`${#a[@]}` is 0, `${a[0]+set}` is empty) yet lists `typeset -a a=([0]=)`, which reads back as one element. sh lists `typeset -a a`, which matches what the array actually holds, and every other part of the row agrees.

## From the bash wording lane (#5719)

- `compgen/an-action-this-shell-does-not-generate` [b] — deliberate: an action this shell does not generate (`-A alias`, `-A user`, …) is refused out loud as `not implemented` rather than answered empty; the case's own Why records the divergence, and `interp/compgendegrade_test.go` pins it.
- `compgen/there-is-no-short-letter-for-function` [b] — the same deliberate refusal: `-u` is user names, which this shell does not generate, so it says so rather than answering an empty list at 1 as bash does.
- `param/a-substring-modifier-that-substitutes` [b] — bash evaluates as it parses, so in `${x:s/X/-/}` the division `s/X` by zero is met before the syntax error at the trailing `/` (`1/0 1` is `division by 0`, `0 && 1/0 +` is the syntax error). This shell reads the whole expression before evaluating any of it; status and output already agree.

## From the ksh wording lane (#5722)

- `heredoc/a-substitution-in-the-delimiter-is-its-own-text` [k]: the token ksh93 quotes is cut from its own lexer buffer at an offset that moves with the text (`$(export q)` comes out as `<<rt`). That is an artifact, not a wording, and `syntax.HeredocDelimiterRefusesACommandSubstitution` already records approximating it on purpose.
- `expansion/element-set-difference-and-intersection` [k]: ksh93's token is a raw `\x03` byte, one of its lexer's internal markers. It is not text from the input, so there is nothing to model.
- `expansion/element-set-operators-against-an-unset-name` [k]: the same raw `\x03` marker byte.
- `pat/the-tilde-flag-in-a-group-decides-snippet-from-plugin` [k]: the token is a raw `\x02` lexer marker byte (`[[ a == ((x)a) ]]` writes the same one).
- `pat/a-pattern-flag-reaches-to-the-end-of-its-group` [k]: the same raw `\x02` marker byte.
- `nounset/a-key-the-association-does-not-have` [k]: ksh93u+ writes `m[(null)]`, which is C's printing of a null pointer where the key belongs. It is a defect, not a wording, and the key is named correctly here.

- `read/from-a-standard-input-that-is-closed` [k, and the bash row]: a Go program cannot see that it was started with fd 0 closed. The runtime reopens it on `/dev/null` (O_RDWR) before `main` runs (`runtime.checkfds`), so `read` gets a working descriptor at end of file. The only tell would be guessing from an O_RDWR `/dev/null`, and that is also what daemon(3) hands a daemon. The `<&-` spelling of the same refusal is modeled (#5820).

## From the bash wording lane (#5719)

- `read/from-a-standard-input-that-is-closed` [b] — not observable from Go: the runtime reopens a descriptor 0 that is closed at exec on `/dev/null` (read-write) before `main` runs, so `read` sees an ordinary end of file where bash sees `EBADF` and writes `read: 0: read error: Bad file descriptor`. Only the open mode (O_RDWR, which `<>/dev/null` also gives) hints at it; status and output already agree.

## From #5721 (dash wording)

- `ulimit/the-file-size-block` [d] — the line is dash's notice that a forked subshell died of SIGXFSZ (`Filesize limit exceeded: 25`); a subshell here is not a process, so there is no child death to report (the same boundary as #519).
- `path/binary-content-is-not-run-as-a-script` [d] — the sentence is not dash's: on ENOEXEC dash execs the host `/bin/sh`, which on the macOS panel is bash 3.2 (`./b.img: ./b.img: cannot execute binary file`). Copying it would copy the host.
- `subst/a-comment-in-a-current-shell-body` [d] — dash does not count a newline that ends a `${` name (`echo ${⏎'a` is line 1 there, `echo ${x:-⏎'a` line 2); observable only as the line of a later parse error in text that is already a bad substitution.
- `redir/a-picked-descriptor-may-outlive-its-command` [d] — dash locates a run-time `Bad fd number` at its *reader's* line, one past the statement once the newline is read (`f() { echo >&$x; }⏎f⏎echo c` is line 3); the runner does not know where the reader stands, and the status and output already agree.

## From L5 (#5716)

- `heredoc/an-expansion-left-unterminated-in-a-body` [k]: stdout and status already match (`after`, 0); only the sentence differs. ksh93 words it `syntax error at line 2: `end of file' unexpected` with a line that follows neither the body's lines nor the script's — measured 2026-10-03, `${` on body line 1 and on body line 2 both report line 2, `${x` reports line 4 — so there is no rule to write without the implementation. (The d half landed in #5793.)
- `trap/an-elements-pipe-handler-runs-inside-the-elements-redirections` [z]: zsh runs the PIPE handler once per failed write(2) of the 128 KiB `echo` — twice before its two error lines and once after — so the count is set by its stdio buffering, not by the language. Ours runs it once, in the right place, inside the element's redirections.

## From #5723 (ash wording)

- `ulimit/the-file-size-block` [ash] — `File size limit exceeded (core dumped)` is BusyBox's notice that a forked subshell died of SIGXFSZ; a subshell here is not a process, so there is no child death to report (the same boundary as the dash row above, and #519).
- `subst/a-comment-in-a-current-shell-body` [ash] — the same as the dash row above: BusyBox does not count the newline that ends a `${` name, observable only as the line of a later parse error (`unterminated quoted string` at 5 rather than 6) in text that is already a bad substitution.
- `param/a-substring-modifier-that-substitutes` [ash] — BusyBox evaluates arithmetic as it reads it, so `${x:s/X/-/}`'s length `s/X/-/` divides 0 by 0 before the reader reaches the `-/` it cannot parse and says `divide by zero`; our arithmetic parses the whole expression before evaluating any of it and says `arithmetic syntax error`. Status and output already agree.
- `core/a-refused-older-body-leaves-an-empty-field` [ash] — a nested backquote's `EOF in backquote substitution` is `line 0` when the command is a script's first line and `line 1` for the same text on lines 2 and 3 (measured 2026-10-04, four placements); no rule fits all four without the implementation.

## From the ksh wording lane (#5722)

- `xtrace/ps4-repeats-inside-a-trap-body` [k]: ksh93 fires the DEBUG trap after it has expanded and traced the command, not before. `trap 'echo T' DEBUG; echo $(echo s)` runs the substitution before `T`. Matching the trace line's position means moving the point at which the trap fires for every command, which is a semantics change outside a wording lane. Measured 2026-10-04 on ksh93u+ 2012-08-01.

## From L5 (#5716)

- `procsub/a-writing-body-in-a-command-substitution-is-captured` [k]: a race in the reference. ksh93's `$( … )` does not wait for a `>( … )` body, so whether the body's bytes are in the value depends on whether it wrote before the substitution's own commands ended — measured 2026-10-03, `tee >(cat)` lands `a` before `IN`, the corpus row's `sleep 0.3` body lands nothing. sh joins the body at the substitution (bash's reading, deterministic).
- `jobs/a-disowned-job-is-not-in-the-listing` [k]: `a &| b` in ksh93 is an operator no other column has — measured 2026-10-03, it pipes `a`, run as a background job that `jobs` lists as `<command unknown>` and that sets no `$!`, into a foreground `b` (zsh's `&|` is disown). Not modeled: it needs a pipeline element that is also a job-table entry, for one shell's undocumented spelling.

## From the ksh wording lane (#5722), grammar

All seven rows already agree with ksh93 on status and output. Each one's wording comes from a ksh93 parse quirk that also changes what runs, so matching it needs a grammar change, not a wording change.

- `cmd/a-function-name-may-hold-an-expansion` [k]: ksh93 reads a `(` straight after a `${…}` as a literal parenthesized run inside the word. `w=foo; echo ${w}(a b)` prints `foo(a b)` and `${w}($v)` expands inside. So `_p_${w}() {` is a word, then `{` as an argument, then `}` is unexpected. We refuse the `(`. Modeling it means a new word-lexing rule.
- `cmd/an-expanded-function-name-is-fixed-at-the-definition` [k]: the same `${…}(` rule.
- `subst/a-paren-body-is-the-parenthesis-and-not-a-list` [k]: ksh93 ends a `${(…)` word where the refused group's tail stops, with no closing brace needed. So in `echo ${(echo a); echo b;}` the `;` ends the word and `}` is unexpected. Our lexer pairs the braces first, so this needs the `${` scan itself to change for that dialect.
- `axis/where-a-substitution-has-no-arm-to-choose` [k]: ksh93u+ accepts `${x/#(a|ab)/X}` and `${x/%(c|bc)/X}` with a pattern that never matches, even the literal text (`x='(a)bc'; echo "${x/#(a)/X}"` prints `(a)bc`), but refuses `${x/(a|ab)/X}`. The row's wording comes from that acceptance, so matching it means modeling a never-matching pattern.
- `pat/a-case-arms-own-paren-in-front-of-a-group` [k]: ksh93 reads `((…))` at a case pattern as one token that never matches. `case b in ((a|b))) echo hit;;` parses and does not match, and `((a|b))|b)` matches through `b`. So `((a|b)) echo` is refused at `echo`. That needs a never-matching pattern token in the case grammar.
- `pat/a-case-arms-paren-in-front-of-an-expression` [k]: the same `((…))` case-pattern token.
- `pat/a-numeric-range-in-a-group-starting-a-case-arm` [k]: the same `((…))` case-pattern token.
Measured 2026-10-04 on ksh93u+ 2012-08-01.

## From the final audit (#5707), ash

- `expansion/a-replacement-scan-stops-where-the-last-match-ended` [ash] — BusyBox 1.37 ash does not terminate on `${v//*/X}`: the golden record holds a timeout for ash (dash too). Ours prints `[X][aX][XXXX]`, the answer bash, ksh93 and zsh agree on. Nothing to copy.

## From lane P (#5971)

No oracle row. In the C locale, a typed multibyte character reaches a `self-insert` wrapper once here, where zsh 5.9.2 calls it once per byte. The line ends up identical. Matching it would need a line that can hold a lone byte, and no wrapper in the maintainer's plugin set counts calls. Recorded in `editing.md`, "Measured, and deliberately not a field".
