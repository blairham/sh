# Sandboxing

How a policy is written, where it is loaded from, what it defaults to,
what a refusal looks like, and what an audit record contains.

`docs/design.md` establishes the seam: the interpreter asks a `Gate`
before every action that leaves its own memory, and reports every one to
a `Sink`. That is the mechanism. This document is the *policy* — the
shipped implementation that fills the seam, and the event schema its
consumers read.

Read `docs/design.md` first. In particular, read the paragraph that says
the boundary is drawn around the interpreter and not around the process
tree, because every limit in this document follows from it.

## What a sandbox here is, and what it is not

**It contains the shell, not the process tree.** A denied `open` stops
the shell reading a file. An allowed `exec` starts a process that makes
its own system calls, and nothing in this repository has any say over
them. `allow exec /bin/cat` is `allow read /**`, spelled less
obviously, and `allow exec /bin/sh` is `allow` everything.

That is not a defect to be fixed later by trying harder inside this
package. Containing a running child needs the operating system —
`seccomp`, Landlock, a sandbox profile, a container — and the substrate
does not own OS backends. What it owns is the decision point, and a
backend that wants one has it: a `Gate` implementation may consult the
kernel, prompt a person, or call out to a supervisor, because the
interface is `Allow(ctx, Action) Decision` and nothing constrains how the
answer is reached.

So the honest claim is narrow and worth stating in the words a reader
will need: **this refuses what the shell itself does, and it is the only
thing that can, because `eval` walks around anything applied from
outside.** A policy that also has to contain children composes this with
an OS sandbox; it does not replace it with one.

## The policy file

### Format: a line per rule

A policy is a line-oriented list of directives. This was the first
decision and JSON was the obvious alternative, since `encoding/json` is
in the standard library and this module has no runtime dependencies at
all. Three things decided it the other way.

**A rule is reviewed in a diff.** A policy is a security artifact, and
the thing that actually stops a bad one is a second person reading the
change. One rule per line makes a changed rule exactly one changed line.
Nested JSON makes the same edit touch brackets and indentation, and the
reviewer has to reconstruct which rule moved.

**Order is visible and comments are possible.** A policy accumulates
reasons — *this path is allowed because the build writes there* — and
the format has to hold them next to the rule. JSON cannot carry a
comment without inventing a JSON dialect, which is a worse outcome than
inventing a small format honestly.

**Paths are the whole content, and JSON escapes them.** Every value in
this file is a path or a glob. A format whose quoting rules turn `\` and
`"` into escapes is a format in which the common case is misread.

The cost is a parser we own, and it is deliberately tiny: no quoting, no
continuation, no nesting, no includes. The grammar is below in full and
it fits in a paragraph.

### Grammar

    policy   := line*
    line     := blank | comment | directive
    comment  := ws* '#' ...
    directive := version | default | rule
    version  := 'version' ws '1'
    default  := 'default' ws decision [ws selector]
    rule     := decision ws selector [ws pattern]
    decision := 'allow' | 'deny'
    selector := 'exec' | 'read' | 'write' | 'open' | 'stat' | 'list'
              | 'path' | 'signal'
    pattern  := rest of the line, trailing whitespace trimmed

The pattern is *the rest of the line* rather than a whitespace-delimited
field, so a path containing spaces is written as it stands. A path
containing a newline cannot be written; name a containing directory
instead. That is the one expressible path this format cannot hold, and
it is recorded here rather than left to be discovered.

`version 1` is **required** and must be the first directive. A parser
that met a file it does not understand has to refuse it rather than read
what it recognizes, because a policy half-understood is a policy that
allows what it was written to refuse. The same requirement is what lets
a later format change be detected instead of misread.

Every parse failure is fatal and names the line. There is no recovery
and no "unknown directive ignored": an unrecognized word is a typo in a
security file, and continuing past it would silently drop a rule.

### Selectors

A selector names a set of the action kinds `interp` defines. The
vocabulary is `interp`'s and stays `interp`'s — this is the same rule
`internal/boundary` follows — and a selector is only a convenient name
for a subset of it.

| selector | action kinds |
| --- | --- |
| `exec`   | exec |
| `read`   | open for reading, stat, read-dir |
| `write`  | open for writing |
| `open`   | open, either direction |
| `stat`   | stat |
| `list`   | read-dir |
| `path`   | exec, open either direction, stat, read-dir |
| `signal` | signal |

`read` covers the probes because reading a directory and asking whether
a file is there are both reading. Someone who writes `allow read
/srv/**` and then finds `[ -f /srv/x ]` false has been given a policy
that does not mean what it says.

`path` is every kind that carries a path, and deliberately not every
kind: a signal names a process rather than a file, so no pattern can
select one and `signal` has to be named on its own. Writing a pattern
after `signal` is a parse error rather than a silently ignored word.

`inherit` is not a selector. `ActionInherit` is recorded and never
gated, for the reason written on it in `interp/seams.go`, and naming it
in a policy is a parse error that says so — better than accepting a rule
that would never be consulted.

### Patterns

Patterns are globs over path components:

- `*` matches within one component and never crosses `/`.
- `?` matches one character that is not `/`.
- `[abc]`, `[a-z]`, `[^abc]` match one character, as `path.Match` reads
  them.
- `**` is a whole component and matches **zero or more** components, so
  `/srv/**` matches `/srv` itself and everything beneath it.

**A pattern must be absolute.** A relative pattern would mean "relative
to a working directory", and the policy does not own one — the shell's
directory moves with `cd` and is not the process's. A relative pattern
is a parse error.

Correspondingly, **a relative path in an action matches no pattern** and
falls to the default. In practice the interpreter resolves paths against
the Runner's directory before it asks — a redirect joins `r.Dir`, a glob
walks from `r.workDir()`, a PATH search resolves each candidate — so
what reaches the gate is already absolute. A relative one arriving here
means something upstream did not resolve, and denying it under a
default-deny policy is the fail-closed answer.

Incoming paths are cleaned lexically before matching, so `/srv/../etc/x`
is matched as `/etc/x` and a `..` cannot be used to walk out of a
pattern.

### A name is not an object, so the open is verified after it happens

**A symbolic link used to defeat a name-based rule.** `deny read
/etc/**` did not stop `cat < link` where the link pointed into `/etc`,
because the decision was made about the name the script wrote and the
name was never the object. `> link` was worse: the file was emptied as
part of the open, so a policy that refused afterwards would have
reported a refusal over a file it had already destroyed.

**Resolving the name before the open is still rejected**, and for the
two reasons that were always given. It would mean the gate performing
filesystem reads of its own, outside the boundary it is enforcing. And
it would still be a time-of-check-to-time-of-use race, because the link
can be replaced between the resolution and the open — the classic one,
where the check and the use are about two different objects.

**So the question is asked after the open, about the descriptor already
in hand.** The kernel performed the traversal once, as part of the open
the script asked for; this reads back the answer it arrived at, with
`F_GETPATH` on Darwin and `/proc/self/fd` on Linux. Nothing here
resolves anything: there is no `lstat` loop and no second walk of the
name. And there is no window, because a descriptor pins an object —
replacing the link afterwards changes what the *name* reaches and cannot
change what this descriptor holds, so what was checked and what is used
are the same thing.

Where the two spellings differ, the gate is asked a second time, about
the kernel's name for what was reached, and a denial there refuses the
open. It is the same action to a consumer: same `actionId`, so the
consultation and the record are one access with a name that resolved
elsewhere.

`O_TRUNC` is held back past that check and applied afterwards, on
regular files only. That guard is measured rather than cautious: on
Linux `open("/dev/null", O_WRONLY|O_TRUNC)` succeeds while `ftruncate`
on the same descriptor answers `EINVAL`, as it does on a FIFO, so
splitting the flag off without the guard would break `> /dev/null`.
Darwin accepts both, which is why one machine is not enough to justify
it.

**The refusal names the link and not where it went.** A diagnostic that
reported the target would hand the script the one fact the rule exists
to withhold, and would let it map a hidden directory one link at a time
by asking to be refused. So the script sees the ordinary refusal, naming
the path as written, and the audit record carries what was actually
reached — the same split the rest of *What a refusal looks like* is
built on.

**The record carries both names, whichever way the decision went.**
`path` is the name that was asked about and `resolved` is the kernel's
own name for what it reached, empty when the two are the same. Each
record used to hold half of one fact and a different half each way: an
allowed open said the name and nothing said it had gone elsewhere, a
refused one said the object and nothing said which name reached it.
"This script read `/srv/data/x`, and `/srv/data` is a link to
`/mnt/vol1`" is the thing somebody reviewing a run wants, and the shell
had just learned it and thrown it away.

That is not the script/operator asymmetry being softened. The asymmetry
is about **who is told**: the script gets the name it wrote and nothing
more, the operator gets both. `resolved` is entirely inside the
operator's half, so filling it in makes the two *records* consistent
with each other and changes nothing a script can observe. Making the
script's view symmetric with the operator's is the thing that must not
happen, and does not.

**The gate is asked about the object alone.** A second consultation
carries the object in `path`, with `resolved` set to the same string —
which looks redundant and is deliberate. A decision must be about the
object, so the object is what a rule matches, and a `Gate` written
before this field goes on deciding correctly. Handing it the name as
well would invite matching on the name, which is the bug this whole
section exists to have closed. `resolved` being non-empty is how a
prompting gate tells the second consultation from the first, so it does
not ask a person twice about one open.

One consequence inside the interpreter is worth writing down: a
directory listing records its access *after* the open rather than beside
the consultation, unlike the other probes in `fsgate.go`, because a
record written before there is a descriptor cannot say where the name
went. `internal/boundary` moved for the same reason and kept its own
rule — it records the *attempt*, so a startup file that is not there is
still an access in the stream.

#### What this closes, and what it does not

Closed, on Darwin and Linux: a redirect that reads or writes through a
link, `.` sourcing through one, and a glob enumerating a directory
through one.

**And the front end's own opens, which were the tail of this.**
`internal/boundary` consulted the gate and answered a bool; each of its
nine callers then did its own `os.Open`, `os.ReadFile` or
`os.WriteFile`, so out here the decision was about a name and the access
was about an object — the same defect, in the package whose whole
purpose is that the shell's own files are inside the boundary.
`HISTFILE` is the one a *script* can aim: it is a shell variable, so a
typed line points the history at a link and the shell reads and appends
through it. The block store is aimed the same way by `SH_BLOCKS_DIR`,
and the ACP pair is aimed by an agent, live, from outside the process.

The fix is the API rather than nine careful call sites. `Boundary` makes
the descriptor itself now — `OpenFile`, `ReadFile`, `WriteFile` — and
verifies it before returning it, so there is no longer a way to ask this
package's permission and then open something else. The shared half lives
in `internal/opened`: the platform call, the table of links an operating
system installs, and the held-back truncation, which is a security
property and not a convenience. Two copies of it would drift.
`TestEveryFrontEndOpenGoesThroughTheBoundaryOrSaysWhyNot` reads the
source of every package that holds a `Boundary` and fails on a direct
open that is not on a written-down exemption list, because the tenth
call site will be written by copying one of the nine.

Open, by construction and not by omission:

- **Hard links and bind mounts.** Both names are real names for one
  object, and the kernel answers with whichever the descriptor was
  opened by, so there is no second name to notice. Closing these means
  matching on *identity* rather than on paths, which is the operating-
  system backend below. `TestAHardLinkIsNotCovered` asserts the current
  behavior so that closing it has to come with a change to this page.
- **`stat`, `[ -f x ]` and the rest of the probes.** A probe is answered
  without opening anything, and opening a path in order to check a
  question about it would be a heavier access than the one asked about:
  it can block on a FIFO, and it fails for a file this process may stat
  but not read. The disclosure is metadata rather than content.
- **`exec` through a link.** Verifying it means opening the program to
  look at it, and execute permission does not imply read permission.
  `allow exec` is already total in the sense this page opens with.
- **Platforms other than Darwin and Linux**, where there is no way to
  ask the kernel and the gate matches the name alone, as it always did.

### The shell's own scaffolding is recorded, never refused

A process substitution runs a command with one end of a pipe and expands
to a path the other end can be opened by. That path is a FIFO under a
directory the interpreter makes for itself, named by the operating
system: a script writes `<(cmd)` and can never write
`<TMPDIR>/sh-procsubNNNNNNNN/sub1`, because it does not know the name
and the name is different every time.

So the gate is not asked about it. `ActionOpen` already said the
scaffolding around the pipe — the directory, the `mkfifo`, the removal —
is outside the boundary, for the reason that gating a path the script
could not have named lets **a policy refuse the mechanism while believing
it refused an access**. The pipe is on that side of the line and was on
the other one, which is what the corpus measurement above found. An
operator wrote `default deny write`, got a shell whose process
substitutions had stopped working, and was shown a diagnostic naming a
path they had never seen.

Every place in the shell that can open one is covered, because there is
more than one: the substitution's own end, a redirect that names the pipe
(`cmd > >(inner)`, `read x < <(inner)`), and `. <(inner)`. Only the
shell's own opens — an *external* command that opens the path is a child
process and outside the boundary entirely, as every child is.

**What is still refused is everything worth refusing.** The inner command
is an `exec` the gate is asked about, before it runs. It executes in a
`Runner` of its own whose every access passes the gate, so `<(cat
/etc/secret)` is refused at the read, under the rule about `/etc`, and the
refusal names that file. What crosses the pipe is that command's output
and nothing else.

**And it is still recorded.** The open reaches the event stream as an
`EventAccess` naming the pipe, so an audit trail says the shell made one
and when. Recorded always, asked never, which is the shape `ActionInherit`
already has.

**The exemption is the command's, not the shell's.** A pipe is in the set
from the moment the word expands until the command that named it ends,
which is what keeps this from being something a script can aim at:
`p=$(echo <(true))` prints a path and lets its command end, so `> "$p"`
afterwards is an ordinary open of an ordinary path and the gate is asked
about it. A subshell starts with an empty set.

Probes are deliberately not included. `[ -f <(cmd) ]` is a stat, which a
policy refuses quietly by answering the way a missing path answers — so a
policy hiding the temporary directory makes that test false rather than
making the construct fail. Widening a suppression to the loudest oracle
the filesystem has, to fix a wrong answer nobody asks for, is a trade this
does not make.

**A compatibility consequence worth stating plainly.** Under a policy, a
path reached through a link is now checked under the kernel's name for
it, so a rule naming a directory that happens to be a symbolic link
protects — and permits — less than its author may expect. On a
merged-`/usr` Linux `allow read /bin/**` does not permit reading
`/bin/ls`, because the object is `/usr/bin/ls`. That is deliberately not
absorbed by the alias table below: `/bin -> usr/bin` is a
distribution's arrangement rather than the platform's, and a table that
assumed it would make one policy file mean two things. The fix is the
posture this page already recommends — name the subtree you control —
and the audit record says which name was refused.

### Platform aliases are expanded once, when the rule is read

One case looks like the section above and is answered somewhere else
entirely, and it is worth separating carefully. That one is about a
*decision*, made when a file is opened; this one is about a *rule*, and
is settled when the file of rules is read.

On macOS `/tmp` is a symbolic link to `/private/tmp`. Always, on every
machine, installed by the operating system, and not something a script
can change. So `deny path /tmp/**` — written by somebody using the name
they type every day — was a rule a shell that had resolved the path
walked straight past: `cd -P /tmp` puts the Runner's directory at
`/private/tmp`, and every access from there reaches the gate under the
other name. *The policy matched nothing*, which from outside is
indistinguishable from *the policy allowed it*.

There is no attacker in that and no filesystem read behind it, so it is
answerable **when the rule is parsed** rather than when a decision is
made. A pattern under a platform alias gains the other spelling beside
it, from a table that is a compile-time constant, and the rule matches
either. Per decision this costs one more pattern comparison and no
system call at all, and there is no time-of-check race because nothing
is looked up.

**Both spellings are kept, rather than rewriting to the physical one.**
That is what an OS backend does and it would be wrong here, because the
two match different things. A backend matches *objects* — by the time
the kernel decides, the name is gone. This matches *names as the
interpreter presents them*, and the interpreter does not resolve links:
`cat /tmp/x` arrives as `/tmp/x` and `cd -P /tmp; cat x` arrives as
`/private/tmp/x`. Rewriting would close the second hole by opening the
first. Keeping both is strictly a widening — no rule stops covering
anything it covered before — which is what makes it safe to apply to
policies already written.

**The table is only what a platform ships unconditionally.** macOS has
three: `/tmp`, `/var` and `/etc`, each into `/private`, in both
directions. Everywhere else the table is empty, and that is a claim
rather than a gap: a distribution merging `/bin` into `/usr/bin` is a
*distribution's* arrangement, absent on machines that predate or decline
it, and a table that assumed it would make one policy file mean two
things with nothing in the file to say so. An ordinary symbolic link
somebody made is not widened here at all — it is caught at the open, by
the section above, which is a different mechanism with a different
answer.

**What a rule normalized to is reported**, because a rule whose meaning
is not in the file is a rule nobody can review. `cmd/sh` prints one line
per normalized rule under `-trace-events` — the flag whose job is
showing what the boundary is doing — and nothing at all otherwise:

    policy: deny path /tmp/secrets/** (also /private/tmp/secrets/**)

It is not prefixed `trace:`, because it is not an event. An event is
something the shell did; this is something the policy is.

#### What the enforcement backends do, measured

Both backends a real sandbox would delegate to resolve at
rule-*creation* time, which is the same answer arrived at from the
kernel's side, and it is why this is the load-time question rather than
the per-decision one.

**Landlock** (Linux) never takes a pathname. A rule is an `O_PATH`
descriptor handed to `landlock_add_rule`, so the name is resolved once
while the ruleset is being built and the rule thereafter follows the
object. There is no name left to resolve when a decision is made.

**Seatbelt** (macOS) takes a path string, and measured with
`sandbox-exec` on Darwin 25.5.0:

| profile rule | access via `/tmp/x` | access via `/private/tmp/x` |
| --- | --- | --- |
| `(deny file-read* (subpath "/tmp/x"))` | allowed | allowed |
| `(deny file-read* (subpath "/private/tmp/x"))` | refused | refused |

So the alias failure is not ours alone: **a Seatbelt profile written
with `/tmp` protects nothing either**, and one written with
`/private/tmp` covers both spellings. A policy that disagreed with the
kernel it delegates to would be worse than either, and this is the case
where agreement was available.

`cd -P` and `pwd -P` are the exception that shows the shape, and it is
already handled: those walk each component through the gate rather than
handing a path to the standard library, so the policy sees every link on
the way. See `interp/physicalpath.go`.

### Evaluation: deny wins, and order does not matter

A rule set is evaluated as:

1. if any `deny` rule matches the action, deny;
2. otherwise if any `allow` rule matches, allow;
3. otherwise the default for that action's kind.

Order-independence is the point, and the argument is composition.
Concatenating two policies has to yield a policy that allows only what
*both* allow — otherwise a policy assembled from an operator's file and
an embedder's baseline means something that depends on which was read
first, and nobody can reason about the result. Deny-overrides is the
only precedence rule with that property.

First-match-wins, the firewall convention, is the alternative and was
rejected for exactly that: it makes concatenation order-dependent, and
it makes a rule's meaning depend on lines above it that a reviewer of
the diff cannot see.

The consequence to know is that a `deny` cannot be narrowed by a later
`allow`. `deny read /srv/**` plus `allow read /srv/public/**` denies
`/srv/public` too. Carving a hole is done the other way — a broad
`allow` with narrow `deny`s inside it — which reads correctly under
deny-overrides and is the shape a default-deny policy wants anyway.

### Default posture: deny, uniformly, for every kind

**The default default is `deny`, for every action kind, and a policy
that says nothing about a path refuses it.**

The obvious reason is that a sandbox whose default is allow is not a
sandbox. The less obvious reason is the one that settles the *per-kind*
question the issue asks, and it is worth spelling out, because a
per-kind mixture looks reasonable and is not.

Suppose `open` defaults to deny and `stat` defaults to allow, on the
theory that a probe reveals less than a read. Then `[ -f /home/x/.ssh/id_rsa ]`
answers truthfully, and the script has learned that the file exists
while being unable to read it. Worse, it has learned it *by asking the
policy* — the difference between "denied" and "not there" is the whole
of the information, and the deny shape below exists precisely to keep
that difference invisible. A per-kind default that allows probes turns
the policy into an oracle for the filesystem it is hiding.

So the posture is uniform, and the file can still express a mixture:
`default allow stat` sets the default for one kind. It is supported
because an operator debugging a policy genuinely wants it, and because
refusing to represent a thing does not stop anyone doing it — they will
write `allow stat /**` instead, which is the same policy spelled longer.
It is documented as discouraged, with the leak named, rather than
prohibited.

`default allow` with no selector is the other supported shape and it is
what turns this into the existing `-deny` debug flag: an allow-all
policy with `deny` rules carving holes. That is a way to *watch* a gate
rather than a sandbox, and `docs/design.md` already draws that
distinction for `-deny`. It is the same distinction here.

**`-deny` is that policy and not a second rule language.** It used to be
a path-prefix list of its own, which was two matchers over one gate, and
the one nobody exercised was the one that was wrong twice over. It
compared lexically where this cleans a path first, so `..` walked out of
a `-deny` rule and not out of the identical file rule. And every rule it
could hold named a path, so `ActionSignal` — the one kind with no path —
could be watched and never refused.

Each `-deny` value is now a rule body, parsed by this package:

    -deny /etc              # the shorthand: deny path /etc/**
    -deny path:/etc/**      # the same rule, written out
    -deny signal            # every signal the shell sends
    -deny exec:/usr/bin/**  # one kind, one subtree

A colon rather than a space, and that is the whole of the difference. A
flag value is one shell word, so `-deny exec /usr/bin` would give the
flag `exec` and the shell a script — which runs, quietly, under a policy
nobody wrote. Every form above is one word and needs no quoting.

The guard is the half worth having. A test reads the `ActionKind`
constant block out of `interp/seams.go` and fails when a kind lands that
`-deny` cannot refuse, or that is not named as deliberately exempt with
its reason. It reads the *declaration* rather than a list in Go, because
a list is a second thing to remember to update and forgetting it is the
failure being guarded against. `ActionInherit` is the one exemption and
its reason is `interp`'s own: it is recorded and never gated.

### Where a policy comes from

Two routes, and the split is deliberate.

**The embedder API is the primitive.** A `Policy` is a value. A program
embedding a Runner builds one in Go, or parses one from any
`io.Reader`, and assigns it to `driver.Shell.Gate` — or writes its own
`Gate` and never touches this package at all. The seam is the interface;
this package is one implementation of it.

**The invocation flag is the reachability route.** `cmd/sh` grows
`-policy FILE`, alongside `-deny` and `-trace-events`, because a seam
nothing reaches is a seam nothing grades. Until a shipped binary can be
handed a policy, the conformance harness and the wild sweep run ungated
and a hole in the boundary looks exactly like a shell that works. That
is the same argument that put `-deny` there, and it is why this is not
"embedder API only".

It is `cmd/sh`'s own flag rather than a `driver` one. `driver` is the
shared front end for binaries that claim to *be* bash or zsh, and no
real shell has a `-policy`; adding it there would make `./bash -policy`
accept a flag bash rejects, which is the rule `AGENTS.md` states about
which flags belong where.

`-audit FILE` is the other half of the same route, and writes the event
schema below to a file — or to standard error for a lone `-`. Appended
rather than truncated, because a trail that erases the previous run on
the next one is not one, and written straight through rather than
buffered, so a shell that dies mid-script has still recorded everything
up to the action that killed it.

**More than one gate composes as an intersection.** `-policy` and
`-deny` together consult both, and any refusal refuses. That is the same
rule the file uses between its own lines, and it is what makes adding a
`-deny` to an existing policy a narrowing rather than a way around the
file. Sinks compose the other way and are fanned out to: watching a
trace by eye and keeping an audit file are different jobs.

**A policy is never discovered.** Not from an environment variable, not
from a dotfile searched for in the working directory or the home
directory, not from a path a shell variable names. A policy that can be
named by `$SH_POLICY` can be replaced by anything that can set the
environment — including the sandboxed script itself, on the way to
invoking a nested shell. The policy comes from the command line or from
the embedder, and from nowhere else. This is a rule about the *source*
of the policy and not about its content, so it cannot be relaxed by a
policy.

### The apparatus is outside the boundary

The policy file and the audit stream are opened by the front end,
before any Runner exists, and **they do not pass the gate**.

This is an exemption and it needs its reason written down, because
`docs/design.md` says the test of which side a front-end access falls on
is who chose the path, and both of these are chosen at the invocation —
which is the same place a script operand comes from, and a script
operand *is* inside the boundary.

The distinction is subject versus apparatus. The script is what the
policy is about. The policy file and the audit log are the policy's own
machinery, and gating them would let a policy refuse to be read or
refuse to record what it did — a boundary that can switch off its own
enforcement record is not a boundary. They are also read before there is
a gate to ask.

That exemption is exactly two paths, both named on the command line by
whoever installed the policy, and it is listed here so that silence is
never the record.

## What a refusal looks like

The rule is one sentence:

> **A denial is opaque to the script and transparent to the audit
> sink.**

Nothing in this initiative changes the shapes already established, and
this section records why they are what they are, because the issue asks
whether a denial should instead be policy-visible everywhere.

| action | the script sees | why |
| --- | --- | --- |
| stat, read-dir | ENOENT — the path is not there | a refusal that identifies itself is an oracle for what is hidden |
| signal | EPERM — a process this one may not signal | the errno the kernel gives for the same refusal |
| open | the failure any unopenable redirect gives | the command must not run without the stream it asked for |
| exec | `exec: refused: /path`, status 126 | a command that visibly did not run cannot honestly report otherwise |

The first two are the important ones and they are ENOENT-shaped and
EPERM-shaped on purpose. If a denied stat said "refused", then
`[ -f /secret/x ]` would distinguish *hidden* from *absent*, and a script
could enumerate the policy's deny list and, through it, the filesystem it
covers. That is the same reason a login prompt does not say which of the
user and the password was wrong.

The last two are visible, and the difference is deliberate rather than
an inconsistency:

**Existence is never leaked. Policy shape is leaked only where a command
visibly did not run.** A refused `exec` tells the script that the policy
said no; it does not tell it whether the program is there. A refused
redirect reports as an unopenable redirect, which is what it is. In both
cases the script already knows something did not happen — it has a
failing status and no output — so a wrong diagnostic would buy no
secrecy and would cost every operator debugging a policy the ability to
tell a refusal from a bug.

The complete picture is in the audit stream, which carries the kind, the
path, and `EventDenied` rather than an error. The operator sees
everything; the script sees a failure with no attribution. That
separation is the whole design, and it is why "policy-visible
everywhere" was rejected.

One caveat worth stating because it is a genuine residual: the *timing*
and the *pattern* of refusals is observable to a script that is looking,
and a determined script can map an allow list by trying things. The
defense against that is the audit trail — a script probing its own
boundary produces a very distinctive event stream — and not an attempt
to make refusals unobservable, which is not achievable.

## Policy and dialects do not interact

**The gate is dialect-blind, and anything that makes a policy answer
depend on the dialect is a design error.** This is the strongest
statement in this document and it is enforced in three ways.

**By type.** `Gate.Allow` receives a `context.Context` and an
`interp.Action`. An `Action` carries a kind, a path, an argument vector,
a write flag, a PID and a signal. There is no dialect in it, no
semantics vector, and no route to one. A gate cannot ask which shell it
is inside because nothing tells it.

**By the import graph.** `internal/policy` imports `interp` and the
standard library. It imports nothing under `dialect/`, and a test
asserts that over the real dependency list rather than by inspection.
This is the substrate rule from `AGENTS.md` — nothing under the core
names a shell — applied to a package that sits beside it.

**By behavior.** A test runs the same script under the same policy in
every dialect and asserts the same denials, in the same order. This is
the assertion that would actually fail if someone reached for the
semantics vector inside a rule, and it is cheap because `cmd/sh` already
takes `-dialect`.

The confusion worth heading off: dialects *do* change which actions
happen. A dialect with a prelude sources it, which opens files; `zsh`
and `bash` split words differently, so a glob may expand to different
paths; one dialect's `kill` builtin may take an argument another's does
not. Those are different **actions**, and the policy answers each of
them the same way it would answer any other. What must never happen is
the same action getting two answers because the shell was in a different
mode. The behavioral test above pins the second and tolerates the first
by using a script the dialects agree about.

## The event schema

The `Sink` half. This is a **shared contract**, not a sandbox feature:
the audit log reads it, an agent protocol's session updates read it, an
assistant assembling context reads it. It is designed here because the
sandbox needed it first, and it is deliberately neutral about all of
them — nothing in it names a protocol, a front end, or a dialect.

It lives in `internal/event`, per the promotion rule, and it is a
*serialization* of `interp.Event` rather than a second event type.
`interp` owns what an event is; this owns how one is written down.

### Shape

One JSON object per line — JSON Lines. A stream is appendable, tailable,
greppable, and survives truncation with the loss of one record. A single
JSON array would have to be closed to be valid, which a log that is
being written cannot be.

    {"v":1,"seq":4,"time":"2026-09-05T11:02:03.000000001Z",
     "session":"0VJ8QG7K2M4N6P8R0S2T4V","actionId":"12",
     "event":"denied","action":"open","path":"/etc/shadow",
     "write":false,"line":3,"file":"script.sh"}

(wrapped here for width; a record is one line)

| field | type | present |
| --- | --- | --- |
| `v` | integer | always — the schema version, `1` |
| `seq` | integer | always — 1, 2, 3, … within one stream |
| `time` | RFC 3339 with nanoseconds | always |
| `session` | string | when the run has an identity |
| `actionId` | string | when the action has an identity |
| `event` | string | always — `command-start`, `command-end`, `denied`, `error`, `access` |
| `action` | string | always — `exec`, `open`, `stat`, `read-dir`, `signal`, `inherit` |
| `path` | string | when the action has one — the name that was *asked about* |
| `resolved` | string | for an open whose name reached an object the kernel calls something else |
| `args` | array of string | for an exec |
| `write` | bool | for an open |
| `pid` | integer | for a signal, always, including 0 |
| `signal` | integer | for a signal, always, including 0 — 0 is `kill -0` |
| `status` | integer | for `command-end`, always, including 0 |
| `error` | string | when the event carries a failure |
| `line` | integer | always — the source line a diagnostic would name |
| `file` | string | when the line came from a file |

`event` and `action` are the names `interp`'s `String()` methods already
produce, so the wire names and the Go names cannot drift.

### The stability rules

These are the contract, and they are what a consumer may rely on.

1. **`v` identifies the schema.** It changes only for a change that
   would break a conforming consumer. A consumer must check it and
   refuse a version it does not know.
2. **A field name means one thing forever.** A name is never reused for
   a different meaning; a field that becomes wrong is abandoned, not
   repurposed.
3. **New fields may be added within a version.** A consumer must ignore
   fields it does not recognize. This is what makes additive change
   possible without a version bump, and a consumer that fails on an
   unknown field has broken the contract rather than found a bug.
4. **New `event` and `action` values may be added within a version.** A
   consumer must not fail on a name it does not know; the useful default
   is to record it and carry on. `ActionSignal` arriving after the other
   five is the worked example of why this rule is needed — see
   `docs/design.md`.
5. **An absent field means the zero value.** Fields are omitted rather
   than written as `null`, except where the table above says *always*,
   and those are always written even when zero because zero is
   meaningful there: status 0 is success, signal 0 is the existence
   probe.

### `seq` and `time` are the encoder's, not the interpreter's

Neither is on `interp.Event`, and both are added when a record is
written. That needs justification in both directions.

`seq` gives a stream a total order of *emission*. It is not a causal
order and must not be read as one: a `Sink` is called from every
goroutine a shell has, so a background job's events interleave with the
foreground's, and two records whose `seq` differ by one may be unrelated.
What `seq` is for is replay and gap detection — a consumer that has
records 1..40 and then 42 knows it lost one.

`time` is stamped at encode time rather than at the action. The gap is
the time between the interpreter emitting and the encoder writing, and
because a `Sink` is called synchronously on the emitting goroutine, that
is a function call. It is honest to within that, and stating the
mechanism is better than implying a precision the record does not have.
The clock is injectable so tests are deterministic.

Neither was added to `interp.Event` itself. An event is a description of
what happened; a sequence number is a property of a stream, and a
timestamp is a property of an observation. Putting them on the event
would make every consumer pay for a clock read whether or not it wanted
one, and would make `interp`'s tests depend on time.

### `session` and `actionId` are identity, and neither is `seq`

These are the first change the schema took after it landed, and they
went in as *added fields* rather than a version bump — which is rule 3
being used rather than described. A consumer written against the
original fifteen fields reads a record carrying these exactly as it did
before, and a test in `internal/event` decodes into that original field
set to keep it true.

They were added because two consumers arrived at the same missing field
from opposite directions and neither could work around it. A front end
that owns a command's outer boundary knows when a command started and
ended but cannot say **which events belong to which command**, so its
record and this stream describe the same session and cannot be joined.
An agent protocol has the same problem one level down: it opens a tool
call when a command starts and closes it when the command ends, and
`EventCommandStart` and `EventCommandEnd` were matched by *ordering* —
which concurrency breaks, because a background job and each half of a
pipeline emit from their own goroutines. Matching on a fingerprint of
(kind, path, args, line, file) closes the wrong record whenever a script
runs the same command twice at once.

Doing it per consumer was the expensive path and the reason this is one
field rather than two: two private id schemes that do not agree are
worse than no id at all, because they look joinable and are not.

**`actionId` is on `interp.Action`,** so the id a `Gate` is consulted
about is the id the events for that action carry. That is the whole
promise — a permission request and the records of what was permitted are
provably the same action — and it is why the field could not live only
on the wire the way `seq` does.

**It is a counter, and `session` is what makes it unique.** A `PATH`
search stats a candidate in every directory `PATH` names and a glob
stats every entry it descends past, so this is one of the hottest things
the interpreter does; sixteen bytes of randomness per stat would be paid
by every shell that had merely asked to watch itself. The counter is
shared across a subshell rather than copied, because a cloned Runner
that numbered from its own copy would hand two different actions the
same id. The pair `(session, actionId)` is what is unique everywhere.

**`actionId` is not `seq` and the two must never be conflated.** `seq`
orders *emission* within one stream and differs on every record;
`actionId` names one action and is the *same* on every record about it.
A consumer that joined on `seq` would pair a command's start with
whatever happened to be emitted next.

**`session` comes from the front end, not from `interp`.** A Runner does
not invent identity, for the same reason it does not read a clock on an
event's behalf: what needs the identity is the thing that has more than
one account of a run to line up, and that is the front end. `driver`
makes one per invocation, in `withDefaults`, which is the single point
every route passes through before anything is built — so the Runner and
the prompt cannot end up with different answers. A caller that already
has a notion of a session, such as an agent protocol with several shells
behind one connection, sets `driver.Shell.Session` and it is left alone.
An empty `session` is a run nobody gave an identity, and the field is
then omitted rather than filled with a placeholder: a record that cannot
be joined should say so.

The id generator itself is `event.NewID`, here rather than beside any
one consumer, because one generator is the whole point. It is
`math/rand/v2` and deliberately not `crypto/rand`, and the reason is
specific to a shell: what is wanted is uniqueness rather than
unpredictability, and measured on macOS the first `crypto/rand.Read` in
a process permanently opens a descriptor. In a shell that number is not
an implementation detail — descriptor 3 is the first one a script parks
with `exec 3>f`, and the process's table is what a replacement inherits.

**The front end's own accesses are identified too, and mint their ids
rather than counting.** `internal/boundary` is the third emitter — the
script operand, `$ENV`, `HISTFILE`, the block store's own index — and
its records are the ones most likely to be joined, since one of the
files it opens *is* the record of what the shell ran. It carries the
same `session`, and stamps each access with an `event.NewID`. The
opposite choice from `interp`, for the opposite reason: there a counter
is forced by the hot path, and out here a run makes a handful of these,
so an id needing no coordination is what lets the several places that
build a `Boundary` go on building one independently. A shared counter
would have to be threaded through all of them and the first that forgot
would issue a duplicate.

## Watching the boundary with the corpora

The gate has unit tests, and unit tests of a boundary are written by
somebody who already knows where the boundary is. The two corpora this
repository already has are written by people who did not, which makes
them the better instrument — so both can run under a policy, and both
report rather than gate.

Neither is a gate, for the reason `make conformance` is not one: a run
that *failed* on a denial would be failing on the policy rather than on
compatibility, and a boundary nobody can watch without breaking the
build is a boundary people switch off. What is produced is a number, and
somebody reading it is what turns a change in it into work.

### `make conformance-gated` — the corpus, twice

The same fourteen hundred cases run twice through the same binary, once
plain and once with a policy, and the cases that answer differently are
listed. No reference shell is consulted at all, which is what makes the
number the same on a machine with a thin panel.

**The posture is the decision worth arguing about**, and two were
measured.

*Deny everything and allow nothing* is the strongest statement and
needs no argument about what to permit. It moves **497 of 1397** cases,
because most of them run a program — and a wall of differences is not a
signal, it is a second corpus nobody will read. It stays available by
naming a policy file with `-policy`; it is not the default.

*Writes are confined to the directory the harness gave the case* is the
default, and it is the posture that makes a claim: a corpus case writes
where it was put and nowhere else. Reads and execs are deliberately not
confined — an allowed program is outside the boundary the instant it
starts, so a policy refusing reads while permitting programs is telling
itself a story. What can honestly be said is about writes, and it is
said exactly.

The scratch directory is named rather than assumed, because it is
`/var/folders/…` on a Mac and `/tmp` on Linux, so the file is generated
per run rather than committed.

**Measured, that policy moves 15 of 1475 cases, none of them in more
than the wording**, and the split is why the report has two headings:

- **15 differ only in the wording of a diagnostic.** Each is a case that
  redirects to a path that does not exist — `/nope/x` — to pin what a
  failed redirection does. The policy refuses the write *before* the
  open, so the shell never learns the path is missing and says `open:
  refused` where it used to say `No such file or directory`. Same
  standard output, same status. This is the design working: a gate that
  let the kernel answer first would be leaking whether a denied path
  exists.
- **None differ in more than the wording**, and that number used to be
  three. All three were process substitution, and they were the finding:
  `<(cmd)` opens a FIFO the *interpreter* chose the path of, under the
  shell's own temporary directory, so a policy confining writes refused
  the mechanism rather than an access the script asked for — which is
  exactly what `ActionOpen`'s scaffolding exemption exists to prevent,
  applied to everything around the pipe but not to the pipe. Closed by
  extending the exemption to the pipe, in the section below.

### `make wild-run-contained` — every script on the machine

`make wild-run` executes third-party scripts, and its containment
argument has been that each script runs under both shells with the same
arguments, in a directory of its own, with no standard input and a
timeout. With `-contained`, the shell under test is handed a policy
naming that directory and nothing else to write to, which **turns the
containment from a statement about the arrangement into a statement
about the shell** — and turns every script on the machine into a test of
the boundary, in bulk, written by people who did not know this
implementation exists.

Only the shell under test is contained. A real shell has no such flag
and could not be given one, so a difference under `-contained` reads as
"the policy refused something" rather than as "the shells disagree" —
which is the asymmetry the comparison lives with, and the reason the
posture is printed on the line with the numbers.

**The first run of it found that the old containment argument was not
true.** Over the default scope — 252 scripts, 501 runs — three write
outside the directory the sweep gave them, on every probe:

| script | writes to |
| --- | --- |
| `/usr/bin/imptrace` | `/tmp/imptrace.XXXXXX` |
| `/usr/libexec/locate.mklocatedb` | `/tmp/mklocateXXXXXX/_mklocatedbNNNNN.list` |
| `/opt/homebrew/bin/check_commit_msg.sh` | a `mktemp` file under the system temporary directory |

Under the old arrangement all three wrote there and nothing noticed —
"a directory of its own" is where a script is *started*, not where it
can write. Under the policy the shell refuses and the diagnostic names
the path. That is three files a `--help` left on the machine per sweep,
found by the boundary and not by reading anything.

## Consequences a user meets immediately

These follow from the decisions above and are listed so they are not
discovered as surprises.

**A default-deny policy must allow the script itself.** The script
operand passes the gate — `internal/boundary` put it there, on purpose —
so `sh -policy p script.sh` needs a rule that permits reading
`script.sh`. That is correct: the shell reading a file is an access, and
the person who wrote the policy is the person who named the script.

**A default-deny policy must allow the interpreter's own reachable
paths.** A dialect with a prelude sources it from a string rather than a
file, so that costs nothing, but `$ENV` and `HISTFILE` are real opens
and are inside the boundary — and, since they are verified after the
fact, a rule must name the place the path *reaches* rather than the
place it is spelled. A `HISTFILE` under a home directory that is itself
a symbolic link needs a rule naming what the link reaches.

**Allowing an interpreter allows everything.** `allow exec /bin/sh`,
`/usr/bin/python3`, `/bin/busybox`, and anything else that runs a
program of its own, ends the usefulness of the rest of the policy. This
is the sharpest footgun in the format and there is no way for the parser
to detect it — an allowlisted binary is a name, and what a name can do is
not knowable from here.

## What is deliberately not here

- **Network rules.** The gate has no network action kind, so a policy
  cannot express one. Adding one is an `interp` change first — an
  `ActionConnect` and the places that would raise it — and this format
  gains a selector when that exists, not before. A format that accepted
  `allow net` today would be accepting a rule nothing consults, which is
  the exact failure mode `docs/design.md` warns about.
- **Resource limits.** `umask` and the resource limits are already hooks
  that a binary fills in, and `docs/design.md` explains why they are
  outside the boundary: they name no path and perform no access.
- **Per-command policy.** Rules match actions, not the command that
  caused them. "Let `git` read `/srv` but not `curl`" is not
  expressible, because by the time an exec has happened the child is
  outside the boundary entirely and the shell's own later accesses have
  no command to attribute them to.
- **Prompting.** `Decision` has `Allow` and `Deny` and no `Ask`. A
  question needs somewhere to ask it, which a library does not have; an
  embedder that has one writes a `Gate` that blocks on its own UI, which
  the interface already permits. That is the agent-protocol case and it
  belongs to whatever owns the conversation.

## Open, and flagged for the maintainer

Two decisions here are defensible defaults chosen in the absence of an
answer, and both are cheap to reverse:

1. **`default allow <selector>` is supported at all.** The uniform
   posture is argued above and the per-kind override is documented as
   discouraged. The alternative is to refuse it and make people write
   `allow stat /**`, which is the same policy with less signal in it.
   Supported, with the leak named.
2. **`read` includes `stat` and `read-dir`.** The alternative is three
   separate selectors and no grouping, which is more explicit and more
   verbose. The grouping was chosen because the ungrouped form makes
   `[ -f x ]` fail under a policy that plainly meant to allow it, which
   is the mistake people actually make.
