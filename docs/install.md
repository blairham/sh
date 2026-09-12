# Installing, and running this as a login shell

Five binaries come out of a build — the substrate driver `sh`, and the
dialect binaries `bash`, `zsh`, `ksh` and `dash`. This file is how they
get out of the build directory, and what it takes to make one of them the
shell a terminal starts.

Read the last two sections before you `chsh`. An interactive session does
not read `~/.bashrc` or `~/.zshrc` yet.

## Where they go, and why that is not a `bin` directory

The binaries are named after the shells they model. That is the point —
a shebang line names a shell by path, `chsh` names one by path, and
`login` starts one as `-bash` — and it is also the hazard, because a
directory ahead of `/bin` on `PATH` answers for **every** program on the
machine that resolves a shell by name. That includes the system's own
scripts, this repository's tests, and the terminal you would need in
order to undo it.

So `make install` puts them in **`$(PREFIX)/libexec/sh`**, which is off
`PATH` by convention, and leaves the names alone:

    /usr/local/libexec/sh/sh
    /usr/local/libexec/sh/bash
    /usr/local/libexec/sh/zsh
    /usr/local/libexec/sh/ksh
    /usr/local/libexec/sh/dash

**Not renamed to `sh-bash`.** The name is load-bearing at the far end.
`login` and every terminal emulator start a login shell with `argv[0]`
set to the shell's name with a dash in front, and `driver.LoginShell`
reads exactly that, so a binary called `sh-bash` would be started as
`-sh-bash`, would print `sh-bash` where a diagnostic should say `bash`,
and would match no shebang anybody has ever written. Keeping the name and
moving the directory costs a full path in `chsh`; renaming costs
correctness in three places.

**Shadowing `PATH` is still possible, but never by accident.** Point
`SHELLDIR` at a directory that is on your `PATH` and `make install`
refuses, and says why. `ALLOW_PATH_SHADOW=1` on the same command line
goes ahead.

`GOBIN` is deliberately not consulted: it is on `PATH` on virtually every
machine that sets it. For the same reason, **do not run `go install
./cmd/...` in this repository** — that writes `bash`, `zsh`, `ksh`,
`dash` and `sh` straight into `GOBIN`, which is the accident this whole
arrangement exists to prevent.

## Install

    make install                            # /usr/local/libexec/sh — needs sudo to write
    sudo make install
    make install PREFIX="$HOME/.local"      # no sudo; ~/.local/libexec/sh

Then check it runs:

    /usr/local/libexec/sh/bash -c 'echo $0 works'
    /usr/local/libexec/sh/bash -i

| Variable | Default | What it does |
| --- | --- | --- |
| `PREFIX` | `/usr/local` | The usual prefix. `SHELLDIR` and `FUNCDIR` derive from it. |
| `SHELLDIR` | `$(PREFIX)/libexec/sh` | Where the five binaries land. |
| `FUNCDIR` | `$(PREFIX)/share/sh/functions` | Where the shipped autoloadable functions land. See below — this one is not free to move, because the shell derives where it looks. |
| `DESTDIR` | empty | Staging root for packaging. Prefixes the copy and nothing else — the `PATH` check still asks about `SHELLDIR`, because that is where a staged tree ends up. |
| `ALLOW_PATH_SHADOW` | unset | `1` allows an install into a directory on `PATH`. |

To remove them:

    make uninstall                          # same PREFIX/SHELLDIR you installed with

**Do not uninstall while one of them is your login shell.** A login shell
that is not there is a login that fails; change your shell back first.

## The functions this shell ships, and pointing at somebody else's

One dialect — `zsh` — looks for **function definition files** when a
script says `autoload -Uz name` and then calls `name`. The search is
`$fpath`, and with nothing in the environment saying otherwise it is two
directories, derived from where the running binary sits:

    $(PREFIX)/share/sh/site-functions      whatever a package installed
    $(PREFIX)/share/sh/functions           what this shell shipped

The second is what `make install` fills, and the four files in it are the
ones a real startup file reaches before it has done anything of its own:

| | |
| --- | --- |
| `is-at-least` | compare two version strings; `is-at-least 5.1 && setopt …` |
| `add-zsh-hook` | put a function in `precmd_functions` and its fellows |
| `colors` | fill `$fg`, `$bg`, `$color` and `$reset_color` |
| `regexp-replace` | replace in place in a named variable, with EREs |

They are written here from the published description of what each one
does and from what the installed shell answers when asked — never from
anybody's function files. `docs/spec/functions.md` records the run.

**`compinit` is not among them**, and neither is `vcs_info`, so a startup
file that calls either still gets a complaint. Both are subsystems rather
than functions — a completion system and a VCS status system — and both are
real gaps rather than silent ones.

On this machine's own `~/.zshrc`, with its plugin manager and prompt theme,
these four take the startup from **14 complaints to 3**, and the three left
are those two names. `docs/spec/functions.md` has the run.

### Pointing it at your own zsh's functions instead

There is nothing to build and no option to pass. `$fpath` follows `FPATH`
from the environment, and a value there **replaces** the default rather
than adding to it, so name both:

    export FPATH="$(/bin/zsh -fc 'print -rn -- ${(j.:.)fpath}'):/usr/local/share/sh/functions"

That is one line in whatever starts the shell. Ask your own zsh where its
functions are rather than copying a path out of this file: two builds of
zsh on one machine disagree about the layout as well as the prefix — a
Homebrew build answers `…/Cellar/zsh/5.9.2/share/zsh/functions` and
Apple's answers `/usr/share/zsh/5.9/functions` — so there is no path here
that would be right on your machine.

It is not the default, and will not become one. A portable binary can
honestly know where it was itself installed; it cannot know where some
other shell was, and a table of guesses in the code would be the paths of
the machine it was written on.

## Homebrew

Once a version is tagged, the tap carries it:

    brew install blairham/tap/sh

The formula installs into the keg's `libexec` and links **nothing** into
`bin`, for the reason above. `brew --prefix sh` names the keg; the shells
are in `$(brew --prefix sh)/libexec`.

## Using it without changing your login shell

The three routes that need no `chsh`, in increasing order of commitment:

1. **By path**, from the shell you already have:

       /usr/local/libexec/sh/bash -i

2. **In a script**, as a shebang. The installed path is absolute, so it
   works from anywhere:

       #!/usr/local/libexec/sh/bash

3. **As a terminal profile's command.** macOS Terminal has
   Settings → Profiles → Shell → "Run command", iTerm2 has Profiles →
   General → Command, and most Linux terminals have "Custom command".
   This is the safest way to live in it for a while: a new window runs
   it, every other window and every non-interactive use of a shell is
   untouched, and reverting is a checkbox rather than a rescue.

## Using this as a coding agent's shell

A coding agent runs the commands it wants run as `$SHELL -c '<command>'`,
after sourcing a snapshot of your shell. That is the case with the
strongest reason to want a policy under it, and it is also the case with
**nowhere to put a flag** — there is no argument vector you control.

There are two steps and neither needs a wrapper script.

**Point `$SHELL` at one of these binaries.**

    export SHELL=/usr/local/libexec/sh/bash

That much works on its own. Measured against realistic agent input —
pipelines, loops, `${PATH%%:*}`, `cd && pwd` — our bash matched real bash
on every one.

**Put the policy in `$SHELL` itself.** `--policy` is read by all five
binaries, so the path can carry it:

    export SHELL="/usr/local/libexec/sh/bash --policy $HOME/.config/agent.policy"

If whatever starts the agent will not accept arguments in `$SHELL` — some
launchers exec the value directly — write a two-line script and point
`$SHELL` at that instead:

    #!/bin/sh
    exec /usr/local/libexec/sh/bash --policy "$HOME/.config/agent.policy" "$@"

`--audit FILE` beside it writes every action the shell took as JSON, one
record per line, appended.

### What it refuses, measured

With a policy denying `/usr/bin/curl`, every way of spelling the command
name is refused identically, because the gate is asked about the
**resolved exec** rather than about the text you typed. Measured through
`bash --policy` on macOS 25.5, 2026-09-12; each refusal exits 126:

| what the agent runs | result |
| --- | --- |
| `echo works` | `works` |
| `echo ${BASH_VERSINFO+is-bash}` | `is-bash` — the dialect is really in effect |
| `/usr/bin/curl --version` | `exec: refused: /usr/bin/curl` |
| `echo x \| /usr/bin/curl …` | refused |
| `$(echo /usr/bin/curl) …` | refused |
| `c=/usr/bin/curl; $c …` | refused |
| `/bin/echo control-ok` (control) | `control-ok` |

The last row is the control, and it is the row that makes the others mean
something: a shell that refused everything would produce the same six
refusals and be useless.

### What this does not contain

**The boundary is around the shell, not around the process tree.** A
policy decides what the *shell* opens, stats, runs and signals. A command
the shell was allowed to start then makes its own accesses, and nothing
here sees them — `allow exec /bin/cat` is `allow read /**` spelled less
obviously. Containing a running child needs an OS sandbox, which sits
above this; `docs/design/sandboxing.md` has the argument and the policy
file's grammar.

**A policy is never discovered.** No environment variable, no dotfile. It
comes from `--policy` and from nowhere else, because a policy that could
be named by the environment could be replaced by anything that can set the
environment — including the script being sandboxed, on its way to
invoking a nested shell.

## Making it a login shell: `/etc/shells` and `chsh`

`chsh` will only give a non-root user a shell that is listed in
`/etc/shells`, so that is the first step and it needs `sudo`. Entries are
absolute paths, one per line:

    echo /usr/local/libexec/sh/bash | sudo tee -a /etc/shells

Then change the shell:

    chsh -s /usr/local/libexec/sh/bash

`chsh` with no `-s` opens an editor on the same field. It asks for your
password, and the change applies to **new** sessions — the shell you ran
it from is still the old one, which is what makes the next step possible.

Check it took effect, without logging out:

    dscl . -read "/Users/$USER" UserShell        # macOS
    getent passwd "$USER"                        # Linux

**Keep the terminal you ran `chsh` in open** until a new window has
opened successfully. If the new shell fails to start you still have a
working one, and

    chsh -s /bin/zsh

puts it back. If every window is gone, an `ssh` session in, a recovery
console, or `sudo chsh -s /bin/zsh <user>` from another account all reach
the same field.

### What a login shell is, here

Login-ness is `argv[0]` beginning with a dash — the convention `login`
and every terminal emulator use — and that is the whole of it. There is
**no `-l` or `--login` flag yet**, so a login shell started by hand needs
the same trick the system uses:

    exec -a -bash /usr/local/libexec/sh/bash     # from bash or zsh

## What a session does not read yet

**No `~/.bashrc`, and no `~/.zshrc`.** That is [#807][807], and it is the
thing that most makes this not a daily driver: aliases, functions,
exports and `PS1` written in those files reach an interactive session not
at all.

What *is* read today, in `driver/startup.go`:

- **`~/.profile`, for a login shell.** Measured through a pty on an
  installed build: a session started as `-bash` has the variables
  `~/.profile` exported; started as `bash -i` it does not.
- **`$ENV`, for any interactive session**, login or not, after
  parameter expansion — so `ENV='$HOME/.shrc'` in the environment gets
  `~/.shrc` sourced at every prompt. This is what `dash` and `ksh93` read
  and is nobody's brand, which is why the substrate reads it rather than
  a name of its own.

So the usable arrangement until #807 lands is `~/.profile` for a login
shell plus `$ENV` for the interactive settings, rather than the rc file
the dialect's name would suggest.

Two smaller differences worth knowing before you live in it:

- The default prompt is the shell's name and version, and a login shell
  currently shows the leading dash — `-bash-5.3$` where bash itself would
  print `bash-5.3$`.
- `$SHELL` is inherited from the environment, so it says what your login
  shell is, not which binary is running. `$0` is the one that answers
  that.

[807]: https://github.com/blairham/sh/issues/807
