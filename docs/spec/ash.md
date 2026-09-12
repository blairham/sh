# BusyBox ash

The fifth dialect, and the only one in this tree whose behavior **no
instrument grades**. Read this file before trusting anything in
`dialect/ash`.

It exists for two reasons, and the second is the one that decided the
shell. The first is coverage: in the container world `/bin/sh` *is*
BusyBox ash — Alpine, most embedded images, most `FROM scratch`-adjacent
bases — so it is where a gated, sandboxed shell actually gets deployed.
The second is that the repository's central claim had never been tested.
`AGENTS.md` says **"adding fish adds a directory rather than editing the
substrate"**, and every dialect in the tree was written before that
sentence was. A fifth one is the experiment.

## The verdict on the substrate claim

**It held.** Nothing under `syntax/`, `interp/` or `driver/` changed.
`dialect/ash/` and `cmd/ash/` are the whole of the shell; the rest of the
diff is the lists a new binary has to appear in — `Makefile`'s `SHELLS`,
`cmd/sh`'s `-dialect`, `cmd/shfmt`'s `-dialect`, `.goreleaser.yaml`, and
two counts in prose.

That is not a claim that every measured behavior fits. Three do not, and
they are listed under *What could not be said* below. It is a claim about
the shape of the work: every answer this shell needed was a **value on an
existing axis**, and no axis anywhere acquired a branch on a shell's name.

## The prediction that was refuted

The issue this dialect came from predicted that ash — dash's sibling,
both NetBSD ash descendants — would "side with dash on most non-POSIX
omissions", and warned that the day it joined the panel every "dash is
the sole holdout, so the construct is core" argument would stop being a
sole-dissenter argument.

Measured, ash goes the other way on every question that was checked:

| question | dash | ash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `[^x]` negates | **no** | **yes** | yes | yes | yes |
| `echo 'a\tb'` interprets without `-e` | yes | **no** | no | no | yes |
| `echo -e` is a flag | **no** | **yes** | yes | yes | yes |
| valueless `local x` over an outer `x` | `outer` | **unset** | unset | no `local` | empty |
| `$((2**3))` | error | **8** | 8 | 8 | 8 |
| `$((10#08))` | error | **8** | 8 | 8 | 8 |

So the sentence in `interp/semantics.go` about `BracketCaretNegates` —
"dash alone treats `^` as an ordinary character" — survives ash rather
than losing to it, and `docs/spec/core.md`'s boundary is not
re-litigated. An ash column, when it lands, **widens** the non-dash
agreement rather than narrowing it.

The grammar goes the same way. ash takes nine constructs dash refuses:

    $'…'      [[ ]]      <(…)      function f { }      function f() { }
    ${v:1:2}  ${v/a/b}   $((2**3)) $((10#08))

and, beyond the table, `++`, `,` in arithmetic, `&>`, `>|` and the `time`
keyword. It refuses arrays, subscripts, the C-style `for`, `select`,
`<<<`, `(( ))`, `$[…]`, floating-point arithmetic, `${v^^}`, `${!x}`,
`${v@Q}`, `|&`, `;;&`, `$(<f)`, `{fd}>f`, `@(…)`, brace expansion and
`+=`.

## How it was measured

There is **no ash binary on the panel machine and no easy way to get
one**. `brew install busybox` answers `No available formula`; BusyBox is
Linux-centric C that does not build cleanly on macOS; its published
static binaries are Linux ELF. macOS's `/bin/sh` is bash 3.2 in `sh`
mode, which is already the `bash-as-sh` column.

So it was measured in a container, **ad hoc and by hand**, on
2026-09-12, against BusyBox **v1.37.0** — the `/bin/ash` of the
`alpine:3` image, `linux/arm64`, under colima.

Two passes:

1. **The whole corpus.** A throwaway program linked against
   `internal/oracle` was cross-compiled for `linux/arm64`, mounted into
   the container and run over every case in `oracle.Corpus` through
   `oracle.Exec` — the same invocation, the same scrubbed environment and
   the same normalization the golden record's own columns get. 3383 cases
   recorded. The results were then diffed against the golden record's
   `dash` column, which is what produced the divergence list the dialect
   was written from.
2. **Hand probes.** Roughly 180 snippets aimed at the questions the
   corpus does not ask of a shell it has never seen: which location shape
   each route uses, which `set` letters exist, which builtins are absent,
   what `[[` actually is, and the wording of every diagnostic family.

No BusyBox source was read, and none may be: it is GPLv2 and
`CLEANROOM.md`'s red list names it. Everything above is the observed
behavior of a binary, which is a fact rather than anyone's expression.

Every number here is reproducible with:

    docker run --rm alpine:3 /bin/ash -c '<snippet>'

## What nothing checks

The oracle locates a panel shell with `exec.LookPath`
(`internal/oracle/shell.go`) and runs it as a local binary
(`internal/oracle/run.go`). A shell that exists only inside a container
has no spelling in `oracle.Shell`. So:

- **there is no `ash` column in `internal/oracle/testdata/golden.json`**;
- **`make conformance-dialects` has no ash row**, and must not grow one,
  because the row would be graded against a column that is not there;
- **`make axis-sweep` has no ash target**, for the same reason — a
  target names the panel column its answers are graded against;
- **`make check` therefore says nothing about this dialect** beyond the
  unit tests in `dialect/ash`, which assert what somebody already
  believed.

`make conformance-dialects` exists precisely to stop a dialect drifting
from the shell it claims to be. This one is exempt from it, and the
exemption is not a decision about ash — it is a fact about the machine.
**It lasts until the oracle can reach a shell that is not on this
machine's PATH**, which is #2263 — the two options and what each of them
costs the golden record are written out there rather than left to be
worked out again.

Until then, treat this package the way the repository treats a gate that
is inert on one platform: a green `make check` is not evidence about ash.

### Where it stood when it landed

Graded against the container recording — our `cmd/ash` run over the same
3382 graded cases and compared with `oracle.Verdict`, the same rule
`make conformance` uses — this dialect scored **2671/3382, 79.0%** on the
day it landed. `cmd/dash` scored 97.3% against its own column on the same
run, after a long campaign; 79% is what a first pass looks like.

The number is written down here rather than in `AGENTS.md` because it
**cannot be re-derived on this machine without Docker**, which is exactly
the property that keeps it out of the golden record. Re-measure it before
quoting it.

## What could not be said

Three measured behaviors have no value on any existing axis. They are
recorded here rather than approximated in code, because an invented
answer in a dialect nothing grades is indistinguishable from a measured
one.

**1. The builtin location prefix.** A message a *builtin* speaks carries
the builtin's name and a line, separated the ordinary way:

    ash: unset: line 0: a[0]: bad variable name
    s.sh: export: line 2: :: bad variable name

`Diagnostics.NamesBuiltinInLocation` is the axis for "the builtin's name
rides on the shell's name", and it is a `bool` whose one true value joins
them *tight* — zsh's `zsh:shift:1:`. ash wants the same thing spaced.
Closing it means widening that bool into a three-valued enum
(absent / tight / spaced), which is 31 references across 14 files. Left
undone deliberately: a substrate edit made for a dialect nothing grades
cannot be checked, so it should land with #2263.

**2. `line 0` under `-c`.** On the command-string route this shell
numbers a builtin's line from zero — `ash -c 'unset "a[0]"'` is `line 0`
and a second line is `line 1` — while a script file and standard input
both number from one. No axis carries a per-route line origin.

**3. `divide by zero`.** The reason word for a division by zero is the
substrate's own string (`division by zero`); this shell writes `divide by
zero`. There is no `Diagnostics` field for it, and adding one for a
single word was not worth a substrate edit.

A fourth is a difference in kind rather than in wording: **`[[ ]]` here is
a builtin, not a keyword.** `type '[['` answers `[[ is a shell builtin`,
and it does not suppress word splitting — `v="a b"; [[ $v == "a b" ]]` is
`b: unknown operand`. The parser still has to know the construct, because
`&&` and `||` inside it are not the shell's operators, so the dialect
sets `DoubleBracket` and accepts that our `[[` is the keyword kind. It is
the largest single family of remaining corpus disagreements (28 rows).

## Vector summary

`dialect/ash/ash.go` carries the evidence for each answer beside the
answer. The shape of the file is the point worth repeating here: it is
one `syntax.Dialect`, one `interp.Semantics`, one `interp.Diagnostics`, a
one-line prelude, and an `Apply` that unregisters nine builtins this
shell does not have. `Prelude` sets `BB_ASH_VERSION`, which is the only
variable this shell names itself in — there is no `$ASH_VERSION` and no
`${.sh.version}` — so it is what a script testing for this shell tests
for.
