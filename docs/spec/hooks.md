# Hooks: what a shell runs between commands

What a session runs on its own account between one command and the next — one
before every prompt, one after a line has been read and before it runs — and
what each is told. A hook is a **function** to call in one shell and a
**variable** of command text to evaluate in another; both are here.

**Governed by:** `repl.HookStyle` for the hooks whose site is the prompt loop —
a table of names and one layout, filled in by `dialect/zsh.HookStyle()` and
`dialect/bash.HookStyle()` and left empty by the other two — and by
`interp.Semantics` for the two things a hook needs that a front end cannot
hold: `HookListSuffix`, which every hook's list is spelled with wherever it
fires, and `DirectoryChangeHook`, whose site is inside `cd`.

The split is the sites, not the mechanism. The **chain** is one implementation
in `interp` (`Runner.HookChain`, `Runner.FireChain`, `Runner.FireHook`) because
`precmd` fires in a prompt loop and `chpwd` fires inside a builtin, and a
builtin cannot reach up into `repl`. What `repl` still owns is the panic guard,
which `interp` deliberately does not have — nothing under `interp/` recovers.

Two fields, because there are two mechanisms: `BeforePrompt` names a
**function** to call and `BeforePromptVariable` names a **variable** whose
command text is **evaluated**. What they share is the *chain* — save `$?`, run
each item behind the panic guard, put the status back before each and after the
last, stop at an item that exited — and that is `interp.Runner.FireChain`,
written once and called by both, and by `cd`.

**Measured:** through a paced pseudo-terminal on macOS 25.5, 2026-09-07 and
2026-09-08, one keystroke at a time, waiting on the next prompt rather than on
output. `/opt/homebrew/bin/zsh` 5.9.2, `/opt/homebrew/bin/bash` 5.3.15, that
same binary again under an argv[0] of `sh`, `/bin/bash` 3.2.57, `/bin/dash`
and `/bin/ksh` — each in a scratch `HOME` with `ZDOTDIR`, `ENV` and `HISTFILE`
redirected into it, driven from a startup file that defined every hook and
printed a marker from each. The line editor redraws a prompt on every
keystroke, so the reading is always the *first* prompt drawn.

A *prompt* hook fires per prompt, so a non-interactive `-c` run answers zero for
every shell and discriminates nothing — measured for `PROMPT_COMMAND` as well,
in all six columns, with `-c`, with `-i -c`, and from a script file: none of
them ran it and none of them said anything. None of that half is in the corpus,
because none of it can be.

`chpwd` is the exception and it is in the corpus: its site is `cd`, so it fires
with no terminal in sight, and `zsh -c 'chpwd() { echo M; }; cd sub'` prints the
marker where the other five print nothing. Three rows —
`cd/a-directory-change-hook`, `cd/nothing-runs-when-the-cd-failed` and
`cd/the-quiet-letter-is-what-suppresses-the-hook`. The middle one is what makes
the first mean anything: a shell that called the function on every `cd` would
score the first row and be wrong.

## Which shells have them

The six columns, measured 2026-09-07 and 2026-09-08. `bash-as-sh` is the same
5.3.15 binary under an argv[0] of `sh`, which changes nothing here.

| | zsh 5.9.2 | bash 5.3.15 | bash-as-sh | bash 3.2.57 | ksh93 | dash |
| --- | --- | --- | --- | --- | --- | --- |
| before every prompt | `precmd` + `precmd_functions` | `PROMPT_COMMAND` | `PROMPT_COMMAND` | `PROMPT_COMMAND` | — | — |
| an array of them | — (a list of *names*) | each element, in order | each element, in order | element 0 alone | — | — |
| before every command | `preexec` + `preexec_functions` | — | — | — | — | — |
| after the directory moved | `chpwd` + `chpwd_functions` | — | — | — | — | — |

Two mechanisms, not one spelling of one. zsh's hooks hold **function names**
and call them; bash's `PROMPT_COMMAND` holds **command text** and evaluates it,
and as an array holds one command string per element. A shell with one has
neither the array-of-names nor the `preexec` half of the other, so they share a
firing site and nothing else.

Both are implemented here. The array is 5.3's: bash 3.2.57 reads the same
variable as a scalar, so `PROMPT_COMMAND=('echo A' 'echo B' 'echo C')` printed
`A` alone there and `A B C` in 5.3. This tree carries one bash and it is 5.3's,
so that difference is recorded and not offered as an axis — one shell's older
build is not a disagreement between shells.

## The chain

A hook is a **named function** followed by the contents of an array called the
hook's name plus `_functions`. Both halves matter: `add-zsh-hook precmd f`
defines no function called `precmd` — it appends `f` to `precmd_functions` — so
a session that read only the named function would find a correctly registered
hook and run nothing. That was the bug in #1281.

Measured, with `precmd` itself defined and
`precmd_functions=(precmd builtin_echo_zz print /bin/echo pcX pcX)`:

    [NAMED]   ← the named function
    [NAMED]   ← and again, because the array names it too
    [PCX]     ← pcX
    [PCX]     ← and again, because the array names it twice

- The named function runs **first**, then the array in the order it holds.
- Neither list is deduplicated, against itself or against the other.
- `builtin_echo_zz` is undefined, `print` is a builtin and `/bin/echo` is a
  file on `PATH`. **None of the three ran and none was reported.** Only a name
  that resolves to a *function* is a hook.
- A name whose function was removed mid-session behaves as an undefined one:
  with `precmd_functions=(pcA nosuchfn_zz pcB)` and `unset -f pcA` typed at the
  prompt, the next prompt ran `pcB` alone and said nothing about the other two.
- A hook that **fails** stops nothing. With the second member of a chain
  returning 3, the third still ran.
- An **empty** element is passed over like an undefined name.
- A **scalar** by the list's name is not a list. `chpwd_functions=a` ran
  nothing, and `precmd_functions=a` ran nothing at the prompt either — the list
  has to be an array. This shell reads it the same way:
  `interp.Runner.HookChain` asks for the array of that name rather than for the
  one-element reading a plain string would give `${x[0]}`.

## When each fires

Every clause below was measured for both mechanisms, and they answer alike.
The transcript is zsh's; bash 5.3.15 and 3.2.57 were driven through the same
harness with `PROMPT_COMMAND` in place of the four names, and moved no cell —
before the first prompt, after an empty line, after a refused line, never at
`PS2`, and always after the job notices.

Measured with a startup file defining all four names, and a prompt whose text
was produced by a command substitution so that prompt expansion left a marker
of its own:

    [PRECMD-NAMED st=0]     ← at startup, before the first prompt
    [PCF1] [PCF2] [PCF3]
    [PEXP]RDY> true         ← the prompt's own $(…) ran after every hook
    [PXNAMED st=0 n=3 …]    ← after the line was read, before it ran
    [PXF1] [PXF2]
    [PRECMD-NAMED st=0] …   ← and again before the next prompt

- Both fire **before the prompt is expanded**, so a hook's output cannot land
  in the middle of a prompt. It is above the prompt, always.
- The prompt hook fires **once at startup** before the first prompt, and once
  after every accepted line — including an **empty** one, and including a line
  the parser **refused**.
- It does **not** fire at a continuation prompt. A four-line `for` loop drew
  `PS2` three times and ran nothing; a backslash continuation the same.
- The command hook fires **once per accepted line**, and only where something
  will run: nothing for an empty line, nothing for a line that would not parse.
  A construct typed over several lines fires it once, when the construct is
  complete.
- **Job notices come first.** With a background job finishing during the
  previous command, the screen held `[1] + done sleep 0.3`, then the hooks,
  then the prompt.

## `$?`

A hook is told the status of the command before it, and cannot change what the
next command reads. Both halves were measured in one run: after `(exit 7)`,

    [PRECMD-NAMED st=7]
    [PCF1 st=7]
    [PCF2 st=7]   ← returns 3
    [PCF3 st=7]   ← still told 7, not 3
    RDY> print -r -- "LEAK$((6*7))=$?"
    LEAK42=7

so the status is put back **before each hook** as well as after the last one.
A chain that restored only at the end would show the second hook the first
one's status, which zsh does not. The command hook behaves the same way, with
the named hook returning 4 and the first array member returning 9.

bash's `PROMPT_COMMAND` preserves `$?` identically — with an element that fails
with `command not found`, the next line still read the previous command's
status — so this is not an axis, it is the answer both shells give.

The same fact one level down is what a hook *sees*, and it is not the hook
machinery's at all: borrowed text is shown the caller's status. `false; eval
'echo $?'` prints 1 in all six columns, and `false; . f.sh` with `echo $?` in
the file prints 1 in all six — while `false; eval ""` and an empty file both
report 0. Two rows, not one, and a shell that clears the status before running
the text answers the second and gets the first wrong: `eval` and `.` then read
success immediately after a failure, and so does every prompt hook, since the
hook *is* borrowed text and the status is the whole of what it is told
(`eval/text-sees-the-callers-status`, `dot/text-sees-the-callers-status`).

`exit` inside a hook is the exception that ends the chain: a `precmd` that
called `exit 3` on its second firing ended the session and drew no further
prompt, and `PROMPT_COMMAND='echo A; exit 3; echo NOTREACHED'` printed `A` and
was gone with status 3 — the rest of the element and the rest of the array both
unrun.

## The evaluated hook

bash's is a **variable**, not a function, and its value is command text. It is
read at every prompt rather than once, which is what makes a `PROMPT_COMMAND`
that assigns to `PROMPT_COMMAND` work: measured, the old text ran out to its
end and the new text took effect from the next prompt on.

An **array** is one command string per element, run in order:

    PROMPT_COMMAND=('echo A' 'echo B; false' 'echo C=$?')

printed `A`, `B`, and `C=` the status of the line *before* the prompt — so a
failing element stops nothing and no element sees another's status, exactly as
in the function chain. Empty and whitespace-only elements run nothing and say
nothing. The list is the value as the prompt found it: an element that replaced
the whole array mid-chain did not change what the rest of that chain ran.

Text that will not parse is reported **against the variable's name**, the
variable is left set, and the session carries on — so the same complaint
arrives at every prompt after:

    bash: PROMPT_COMMAND: line 3: syntax error near unexpected token `('
    bash: PROMPT_COMMAND: line 3: `echo unbalanced ((('

The name is the load-bearing part. The text is in a variable and the person's
only way back to it is that variable's name; a failure reported as `eval` would
send them looking for a builtin they never ran. `interp.Runner.EvalVariable` is
where that naming lives, and it is the other half of the seam
`interp.Runner.CallFunction` is: one runs a hook whose value is a *name*, the
other a hook whose value is *text*, and neither can be reached through the
other.

Unset and empty run nothing and report nothing. This shell is silent for both.

After a line the parser refused, the hook is told the status that refusal set —
2 in bash 5.3.15 and **258** in 3.2.57, which is the same per-shell answer
`repl.Shell.ParseFailureStatus` carries and the same column that cannot be an
exit status at all.

## What the command hook is told

Three arguments, always three. Measured with `alias gg='echo aliased'` defined:

| typed | `$1` | `$2` | `$3` |
| --- | --- | --- | --- |
| `echo    a     b` | `echo    a     b` | `echo a b` | `echo a b` |
| `gg` | `gg` | `echo aliased` | `echo aliased` |
| `true;false` | `true;false` | `true; false` | `true`⏎`false` |
| `for i in 1 2; do echo $i; done` | as typed | `for i in 1 2; do; echo $i; done` | `for i in 1 2`⏎`do`⏎⇥`echo $i`⏎`done` |
| `(exit 7)` | `(exit 7)` | `( exit 7; )` | `(`⏎⇥`exit 7`⏎`)` |

So `$1` is the line exactly as it was typed, newlines and all for a multi-line
construct, and `$2` and `$3` are the command that will actually run written
back out — one line and many. Alias expansion is visible in the second and
third and not in the first.

This shell produces all three. `$1` is the text the editor accepted; `$2` and
`$3` are the parsed line printed with `syntax.PrintFileWith`, which is
alias-expanded because a typed line is parsed with alias expansion on. The
many-line arrangement is the dialect's `FunctionLayout` — the same one
`typeset -f` lists a body in, and field for field what was measured above.

Two spellings differ and are recorded rather than chased, because both are how
a shell writes a tree back rather than what it will run: zsh puts a stray `;`
after the `do` of a one-line loop, and breaks a subshell over three lines in
`$3` where this printer keeps it on one.

## The directory-change hook

`chpwd` fires once `cd` has moved the shell. It is the one hook of the family
whose site is a **builtin** rather than the prompt loop, and that is why it is
`interp.Semantics.DirectoryChangeHook` and not a field on `repl.HookStyle`: a
prompt-loop hook would miss a `cd` inside a function, a `cd` in a subshell, and
every `cd` a `-c` script makes with no prompt in sight. Measured at a prompt,
typing `cd /tmp` ran `zshaddhistory`, then `chpwd`, then `chpwd_functions`, and
only then that line's `precmd`.

Same chain as the prompt hooks, measured the same way: the named function
first, then the array in order, neither deduplicated, a non-function name
passed over in silence, a failing member stopping nothing, every member told
the status of the command *before* the `cd`, and `exit` ending it. It is told
**no arguments** — `$#` is 0 — and `$PWD` and `$OLDPWD` are already set when it
runs.

Measured 2026-09-10, zsh 5.9.2 with `-c`, against the five that have no such
hook (bash 5.3.15, bash-as-sh, bash 3.2.57, ksh93, dash: a `chpwd` function ran
on none of their `cd`s and none of them said anything):

| what happened | runs it |
| --- | --- |
| `cd DIR` that moved | yes |
| `cd` with no operand, to `$HOME` | yes |
| `cd -` | yes |
| `cd` to the directory the shell is already in | **yes** — the move, not the change |
| `cd` that failed | no |
| `cd -q DIR` | **no** — this is the whole of what that letter means |
| `pushd` / `popd` | yes, once per move |
| `pushd -q` / `popd -q` | no |
| a bare directory name under `autocd` | yes |
| a `cd` inside a function | yes, at the `cd` |
| a `cd` inside a subshell or `$(…)` | yes, in there |
| assigning to `PWD` | no |
| shell startup | no |

It runs **last**, after anything `cd` itself printed: at a prompt `cd -` wrote
`/usr` and then the marker, and a CDPATH move wrote the directory it found and
then the marker. A hook that itself calls `cd` fires the hook again, with no
guard beyond the ordinary recursion limit — a pair of hooks moving back and
forth ended with `chpwd: job table full or recursion limit exceeded`.

Everything in that table follows from one placement, which is the reason to
state it that way: `pushd`, `popd` and `autocd` are `cd` here and in zsh both,
so putting the hook at the end of `cd` answers all of them at once. #1775.

## The hook that fires as the shell ends

zsh's `zshexit`, and it is where a plugin tears down what it started:
`gitstatus` registers `_gitstatus_cleanup_…` there to stop the daemon it
launched and powerlevel10k's async worker registers `_p9k_worker_cleanup`.

Same chain again — the named function, then the array in order, no
deduplication, an undefined name passed over in silence — and it is told **no
arguments**. Measured 2026-09-12, zsh 5.9.2:

| asked | answer |
| --- | --- |
| where it fires relative to `trap … EXIT` | **after** it: the trap wrote its line, then the hook |
| what `$?` is | the status the shell is leaving with, put back before **each** item — with the shell exiting 4, an item that returned 5 and an item that ran `false` were both followed by one reading 4 |
| whether `return` changes the status | no: a hook returning 5 left a shell exiting 4 exiting 4 |
| whether `exit` changes it | **yes**, and the *last* one wins: `exit 9` in the named hook and `exit 11` in a member left it exiting 11 |
| whether an item that exited stops the rest | **no** — the member after `exit 9` still ran |
| after a fatal signal | neither this nor the EXIT trap runs: a script killed by SIGTERM wrote nothing and exited 143 |
| in a subshell | a subshell that calls `exit` explicitly fires it — `(exit 7)` ran it with `$ZSH_SUBSHELL` of 1 — where one that falls off its end does not |

The exit rule is the single place this chain parts company with every other
one here, and it follows from the site rather than being a special case: at a
prompt hook, `exit` ends the session, so the chain has nothing left to do; on
the way out the session is already over, so all `exit` can do is record a
status.

The site is **the end of the session**, which every route out of a shell
reaches and no prompt loop reaches at all — a script run with no prompt in
sight fires it too. So it is `Semantics.ExitHook`, fired from `interp`'s
`Finish`, for the same reason `chpwd` is `Semantics.DirectoryChangeHook`. The
subshell firing is written down and not modeled: a subshell here does not pass
through `Finish`, so nothing reaches it, and a guess would be a plausible
wrong answer. #2111.

## The hooks that are named and not fired

`periodic` and `zshaddhistory` take the named function and the `_functions`
array exactly as `precmd` does — measured, with the same
undefined-name-in-the-middle probe for each — so the **chain** is one mechanism
and one implementation serves all of them. Their **firing sites** are two more,
and neither is the prompt loop:

| hook | fires | told |
| --- | --- | --- |
| `periodic` | on a timer, `$PERIOD` seconds apart | nothing |
| `zshaddhistory` | where a line is saved | the raw line, newline included; returning non-zero rejects it |

bash has no such list: every hook it has is `PROMPT_COMMAND` and it runs, so
`dialect/bash.HookStyle()` names nothing as unfired. What follows is zsh's.

This shell fires neither of the two, and **says so by name**, once per name,
the first time it sees one defined:

    zsh: periodic: hook not implemented yet

A hook that is registered and never called is the failure #1281 is about; one
that is registered and never called *quietly* is the same failure one level
down. The refusal is the same three things `interp`'s absent parameters do —
name it, refuse it, and let nothing quietly depend on it.
