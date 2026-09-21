// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package promptfidelity

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/blairham/sh/internal/cellgrid"
)

// The three sources: this shell, and the two programs a configuration can
// be imported from.
//
// Each of them ends at the same place — bytes a terminal was sent, played
// into a grid at the pinned width — and the differences between them are
// entirely in how a prompt is got out of the program.

// settleFor is how long a source waits for a prompt to stop arriving.
//
// A quiet period and not a mark, because what is being captured is a
// *prompt* and a prompt has no end marker: it stops when the program has
// finished drawing it. The number bounds a wait rather than measuring work,
// exactly as driver's sessionBudget does, and a source that is still
// drawing after it is a source whose row will differ and say so.
const settleFor = 250 * time.Millisecond

// startupWait bounds how long a program has to become ready to be typed at.
//
// Generous, because one of the sources fetches a helper daemon the first
// time it runs in a fresh home and that is a download rather than a
// startup. It bounds a hang rather than measuring work: a program that is
// not ready after it is a row that says so.
const startupWait = 90 * time.Second

// Ours drives this shell's own prompt.
//
// Through a **pseudo-terminal**, because a shell declines to draw a prompt
// without one — which is also the spec's rule for this instrument, and the
// reason every other prompt test in this tree that reads a pipe is
// structurally unable to ask this question.
type Ours struct {
	// Binary is the dialect binary to drive, and Config is the theme
	// configuration it is pointed at.
	Binary string
	Config string
}

// Name is what the report calls it.
func (o Ours) Name() string { return "this shell" }

// Available checks the two files this needs.
func (o Ours) Available() (bool, string) {
	if o.Binary == "" {
		return false, "no binary was named"
	}
	if _, err := os.Stat(o.Binary); err != nil {
		return false, "no binary at " + o.Binary
	}
	if o.Config == "" {
		return false, "no configuration was named"
	}
	if _, err := os.Stat(o.Config); err != nil {
		return false, "no configuration at " + o.Config
	}
	return true, ""
}

// Render drives one prompt at this context.
//
// The session is pinned by *being* in the state rather than by being told
// it: the shell changes directory, runs something that exits with the
// status, and starts a background job — so what the prompt reads is the
// shell's own answer to each question rather than a number the harness
// pushed into a variable. A segment that read the real state and a harness
// that faked it would be a harness that could not fail.
func (o Ours) Render(ctx Context) (*cellgrid.Grid, error) {
	home, err := os.MkdirTemp("", "promptfidelity")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(home) }()

	session, err := startPty(o.Binary, []string{o.Binary, "-i"}, ctx, []string{
		"HOME=" + home,
		"ZDOTDIR=" + home,
		"SH_PROMPT_CONFIG=" + o.Config,
	})
	if err != nil {
		return nil, err
	}
	defer session.close()

	if err := session.ready(startupWait); err != nil {
		return nil, err
	}
	for range ctx.Jobs {
		if err := session.background(drawWait); err != nil {
			return nil, err
		}
	}
	session.line("cd " + shellQuoted(ctx.Dir))
	session.line(pinning(ctx))
	if err := session.settle(drawWait); err != nil {
		return nil, err
	}
	return session.screen(ctx.Columns), nil
}

// pinning is the one line that puts the session in the state the prompt is
// to be drawn for: the duration, the screen, and the status.
//
// **In that order, and the order is the bug this had.** The status a prompt
// reads is the *last* command's, so an erase written after the `(exit N)`
// makes it the erase's status and every row is drawn at zero. And the erase
// is second-to-last rather than first, because it has to take the echo of
// this very line off the screen: a terminal echoes what is typed, and a
// comparison against a program that was never typed at would report the
// echo as a difference in every row it covers.
func pinning(ctx Context) string {
	return fmt.Sprintf("sleep %s; printf '\\033[2J\\033[H'; (exit %d)",
		seconds(ctx.Duration), ctx.Status)
}

// drawWait bounds the wait for a prompt to finish being drawn, once the
// program is known to be ready. A prompt is meant to be immediate; a
// program still drawing after this is a row that will differ and say so.
const drawWait = 15 * time.Second

// seconds is a duration as a shell's `sleep` takes it.
//
// A real wait rather than a variable set to a number, for the reason the
// directory is a real `cd`: the duration a prompt draws is the one the
// shell measured.
func seconds(d time.Duration) string {
	if d <= 0 {
		return "0"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', -1, 64)
}

// Starship drives the real starship.
//
// Not through a terminal, and that is the program's own design rather than
// a shortcut: `starship prompt` writes a prompt to standard output and takes
// the context as flags, which is how every shell it supports asks it for
// one. So the context is pinned by *passing* it, and what comes back is the
// same bytes a shell would have put on the screen.
type Starship struct {
	// Binary is the program, found on PATH where it is not named.
	Binary string
	// Config is the starship configuration to drive it with, which is the
	// file the comparison's own import was made from.
	Config string
}

// Name is what the report calls it.
func (s Starship) Name() string { return "starship" }

// Available finds the program and the configuration.
func (s Starship) Available() (bool, string) {
	if _, err := s.resolve(); err != nil {
		return false, err.Error()
	}
	if s.Config == "" {
		return false, "no starship configuration was named"
	}
	if _, err := os.Stat(s.Config); err != nil {
		return false, "no starship configuration at " + s.Config
	}
	return true, ""
}

func (s Starship) resolve() (string, error) {
	if s.Binary != "" {
		if _, err := os.Stat(s.Binary); err != nil {
			return "", errors.New("no starship at " + s.Binary)
		}
		return s.Binary, nil
	}
	path, err := exec.LookPath("starship")
	if err != nil {
		return "", errors.New("starship is not on PATH")
	}
	return path, nil
}

// Render asks starship for one prompt.
func (s Starship) Render(ctx Context) (*cellgrid.Grid, error) {
	binary, err := s.resolve()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(binary, "prompt",
		"--status="+strconv.Itoa(ctx.Status),
		"--jobs="+strconv.Itoa(ctx.Jobs),
		"--cmd-duration="+strconv.FormatInt(ctx.Duration.Milliseconds(), 10),
		"--terminal-width="+strconv.Itoa(ctx.Columns),
		"--logical-path="+ctx.Dir,
		"--path="+ctx.Dir,
	)
	cmd.Dir = ctx.Dir
	cmd.Env = append(os.Environ(),
		"STARSHIP_CONFIG="+s.Config,
		"HOME="+ctx.Home,
		"COLUMNS="+strconv.Itoa(ctx.Columns),
		"TERM=xterm-256color",
		// Its own cache, so a run here never touches the person's.
		"STARSHIP_CACHE="+filepath.Join(os.TempDir(), "promptfidelity-starship"),
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("starship prompt: %w", err)
	}
	grid := cellgrid.New(ctx.Columns)
	_, _ = grid.Write(out)
	return grid, nil
}

// Powerlevel10k drives the real powerlevel10k, under a real zsh.
//
// Through a pseudo-terminal, because that prompt exists only in an
// interactive zsh — and this is the only source here that needs a *second*
// thing installed, which is why it has no default path and is named or not
// run. The same rule `make wild`'s plugin roots follow: the path is one
// machine's, one plugin manager's and one person's, and baking it into a
// binary is an argument already lost elsewhere in this tree.
//
// Running it is reading no source. CLEANROOM.md's green list covers
// "observed behavior of real binaries" in as many words, and that is the
// whole of what this does: drive the program, record what it drew.
type Powerlevel10k struct {
	// Zsh is the real zsh, found on PATH where it is not named.
	Zsh string
	// Plugin is the directory holding the theme, which is named or the
	// source is not run.
	Plugin string
	// Config is the configuration to drive it with, which is the file the
	// comparison's own import was made from.
	Config string

	// home is the scratch startup directory, prepared once and reused for
	// every render.
	//
	// Reused rather than made per render, and it is not a saving: this
	// theme fetches a helper daemon on its first run in a fresh home, and a
	// fresh home per render meant every render was a download — which the
	// first run of this instrument reported, faithfully, as a screen
	// reading "fetching gitstatusd" rather than a prompt. The stability
	// rule would not have caught it either, since both renders were equally
	// wrong.
	home string
}

// Name is what the report calls it.
func (p *Powerlevel10k) Name() string { return "powerlevel10k" }

// Available checks zsh, the plugin and the configuration.
func (p *Powerlevel10k) Available() (bool, string) {
	if _, err := p.resolve(); err != nil {
		return false, err.Error()
	}
	if p.Plugin == "" {
		return false, "no powerlevel10k directory was named (there is no default, and will not be)"
	}
	if _, err := os.Stat(filepath.Join(p.Plugin, "powerlevel10k.zsh-theme")); err != nil {
		return false, "no powerlevel10k.zsh-theme under " + p.Plugin
	}
	if p.Config == "" {
		return false, "no powerlevel10k configuration was named"
	}
	if _, err := os.Stat(p.Config); err != nil {
		return false, "no powerlevel10k configuration at " + p.Config
	}
	return true, ""
}

func (p *Powerlevel10k) resolve() (string, error) {
	if p.Zsh != "" {
		if _, err := os.Stat(p.Zsh); err != nil {
			return "", errors.New("no zsh at " + p.Zsh)
		}
		return p.Zsh, nil
	}
	path, err := exec.LookPath("zsh")
	if err != nil {
		return "", errors.New("zsh is not on PATH")
	}
	return path, nil
}

// Render drives one prompt out of a real zsh.
func (p *Powerlevel10k) Render(ctx Context) (*cellgrid.Grid, error) {
	zsh, err := p.resolve()
	if err != nil {
		return nil, err
	}
	home, err := p.scratchHome()
	if err != nil {
		return nil, err
	}

	session, err := startPty(zsh, []string{zsh, "-i"}, ctx, []string{
		"HOME=" + home,
		"ZDOTDIR=" + home,
	})
	if err != nil {
		return nil, err
	}
	defer session.close()

	if err := session.ready(startupWait); err != nil {
		return nil, err
	}
	for range ctx.Jobs {
		if err := session.background(drawWait); err != nil {
			return nil, err
		}
	}
	session.line("cd " + shellQuoted(ctx.Dir))
	session.line(pinning(ctx))
	if err := session.settle(drawWait); err != nil {
		return nil, err
	}
	return session.screen(ctx.Columns), nil
}

// scratchHome prepares the startup directory, once.
//
// A startup file that loads the theme and the configuration and **nothing
// else**. The person's own rc is deliberately not read: what is being
// compared is a configuration, and a plugin manager's forty other plugins
// are not part of it.
func (p *Powerlevel10k) scratchHome() (string, error) {
	if p.home != "" {
		return p.home, nil
	}
	home, err := os.MkdirTemp("", "promptfidelity-p10k")
	if err != nil {
		return "", err
	}
	rc := "source " + shellQuoted(filepath.Join(p.Plugin, "powerlevel10k.zsh-theme")) + "\n" +
		"source " + shellQuoted(p.Config) + "\n"
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(rc), 0o600); err != nil {
		return "", err
	}
	p.home = home
	return home, nil
}

// Close removes the scratch home, and is what keeps a harness from leaving
// one per run behind.
func (p *Powerlevel10k) Close() error {
	if p.home == "" {
		return nil
	}
	home := p.home
	p.home = ""
	return os.RemoveAll(home)
}

// shellQuoted wraps a path so a shell reads it as one word, whatever is in
// it.
func shellQuoted(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
