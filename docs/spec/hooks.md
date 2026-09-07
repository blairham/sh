# Hooks: what a shell runs between commands

The functions a session runs on its own account — one before every prompt,
one after a line has been read and before it runs — and what each is told.

**Governed by:** `repl.HookStyle`, the dialect's answer and not the semantics
vector's. It is a table of names and one layout, filled in by
`dialect/zsh.HookStyle()` and left empty by the other three, which is what a
dialect with no hooks is.

**Measured:** through a paced pseudo-terminal on macOS 25.5, 2026-09-07, one
keystroke at a time, waiting on the next prompt rather than on output.
`/opt/homebrew/bin/zsh` 5.9.2 and `/opt/homebrew/bin/bash` 5.3.15, each in a
scratch `HOME` with `ZDOTDIR` and `HISTFILE` redirected into it, driven from a
startup file that defined every hook and printed a marker from each.

`precmd` fires **per prompt**, so a non-interactive `-c` run answers zero for
every shell and discriminates nothing. Nothing in this entry can be measured
by the corpus, and none of it is in the corpus.

## Which shells have them

| | zsh 5.9.2 | bash 5.3.15 | bash 3.2.57 | ksh93 | dash |
| --- | --- | --- | --- | --- | --- |
| before every prompt | `precmd` + `precmd_functions` | `PROMPT_COMMAND` | `PROMPT_COMMAND` | — | — |
| before every command | `preexec` + `preexec_functions` | — | — | — | — |

Two mechanisms, not one spelling of one. zsh's hooks hold **function names**
and call them; bash's `PROMPT_COMMAND` holds **command text** and evaluates it,
and as an array holds one command string per element. A shell with one has
neither the array-of-names nor the `preexec` half of the other, so they share a
firing site and nothing else. Only zsh's is implemented here.

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

## When each fires

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

`exit` inside a hook is the exception that ends the chain: a `precmd` that
called `exit 3` on its second firing ended the session and drew no further
prompt.

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

## The hooks that are named and not fired

`chpwd`, `periodic`, `zshaddhistory` and `zshexit` take the named function and
the `_functions` array exactly as `precmd` does — measured, with the same
undefined-name-in-the-middle probe for each — so the **chain** is one mechanism
and one implementation serves all six. Their **firing sites** are four more,
and none of them is the prompt loop:

| hook | fires | told |
| --- | --- | --- |
| `chpwd` | where the working directory changed | nothing |
| `periodic` | on a timer, `$PERIOD` seconds apart | nothing |
| `zshaddhistory` | where a line is saved | the raw line, newline included; returning non-zero rejects it |
| `zshexit` | on the way out | nothing |

Measured for `chpwd`: typing `cd /tmp` ran `zshaddhistory`, then `chpwd`, then
`chpwd_functions`, and only then that line's `precmd` — so it belongs inside
`cd` and not at the prompt, where it would also miss a `cd` inside a function.

This shell fires none of the four, and **says so by name**, once per name, the
first time it sees one defined:

    zsh: chpwd: hook not implemented yet

A hook that is registered and never called is the failure #1281 is about; one
that is registered and never called *quietly* is the same failure one level
down. The refusal is the same three things `interp`'s absent parameters do —
name it, refuse it, and let nothing quietly depend on it.
