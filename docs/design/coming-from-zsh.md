# coming from zsh (or bash)

> **Status: design, not built.** Nothing on this page is implemented in
> this repository. It is kept because the reasoning is worth having when
> the work is picked up; read it as a proposal, not as a description of
> what the shell does today.


One command:

```sh
sh migrate            # read your setup, show what would be imported
sh migrate --apply    # write the sh rc
sh migrate --apply --history   # and bring your history across
```

It reads `.zshrc`, `.bashrc`, `.bash_profile`, `.bash_aliases`,
`.zprofile`, `.zshenv` and `.profile`, and writes an rc file with your
aliases, functions, exports, `PATH` entries and the nearest theme.

**Nothing is executed.** Your rc files are parsed, never sourced —
importing a config by running it is how an import becomes an attack, and
it is also how a migration inherits the startup cost you are leaving.

**Nothing is dropped silently.** Every line that did not translate is
listed with its file, its line and the reason. That report is the point:
it is what makes the result safe to adopt without diffing it against the
original.

## What comes across

| from your rc | into the shell |
| --- | --- |
| `alias ll='ls -alF'` | the same alias — the shell expands aliases interactively, as bash does |
| `mkcd() { … }` | copied verbatim |
| `export EDITOR=nvim` | the same export |
| `PATH=$HOME/bin:$PATH` | the same prepend, in order |
| `eval "$(starship init …)"` | `SH_THEME=starship` — your starship prompt, rendered by starship |
| `eval "$(oh-my-posh init bash …)"` | the same line, unchanged — its init sets `PS1` and the shell honors it |
| `POWERLEVEL9K_*` / `source ~/.p10k.zsh` | `SH_THEME=p10k`; `prompt import` takes the whole config, all 300-odd settings |
| `ZSH_THEME="agnoster"` | the closest built-in, named in the report |
| zinit / zi | `zi migrate` — the shell has that engine natively |
| `.zsh_history` / `.bash_history` | the JSONL store, with timestamps and durations kept |

History import goes through the same secret rules as a typed
command: a history file is
the single most likely place for a token to be sitting, and the importer
does not put one back on disk.

## What does not, and why

- **Anything inside a conditional.** `if [ "$(uname)" = Linux ]; then
  alias …` stays where it is: the condition was written for a reason,
  usually "only on this machine", and flattening it would import an
  alias that was never meant to be unconditional.
- **zsh-only syntax** — `setopt`, `zstyle`, `autoload -Uz`, `${(f)…}`.
  A file that uses it is read line by line instead, so the ninety lines
  that do translate still do; the rest is listed.
- **oh-my-zsh plugins and themes.** They are zsh scripts, and the shell does
  not run zsh. Plugins that are shell-agnostic can be listed in
  `plugins.toml` with `plugin add`; themes have no port.
- **Your prompt string**, if you wrote one by hand. the shell renders `PS1`,
  including bash's escapes, so it usually just works — but it is
  reported rather than assumed, because a `PROMPT` written in zsh's
  escape vocabulary is a different language.

## After the import

Your existing tools do not need converting at all. the shell implements
bash's hook surface, so `eval "$(direnv hook bash)"`,
`eval "$(fzf --bash)"`, `eval "$(zoxide init bash)"` and the rest work
as they are — see the tool matrix and the
measured matrix in `make smoke`.

Try it without commitment: run `sh` in one tab. No `chsh`, nothing to
undo but two directories.
