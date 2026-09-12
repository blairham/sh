# Binaries the graders build live under the checkout rather than a shared
# TMPDIR name. Several worktrees of this module are often graded at once, and
# a fixed /tmp path let one checkout's build replace another's mid-run — which
# reads as a flaky implementation rather than as two builds sharing a name.
BINDIR := $(CURDIR)/build

# Where `make install` puts the shells, and why it is not $(PREFIX)/bin.
#
# These binaries are called `bash`, `zsh`, `ksh`, `dash` and `sh`. The name
# collision with the shells they model is the entire point — a script with
# `#!/…/bash` at the top has to reach one — and it is also the way to break a
# machine: anything earlier on PATH than /bin answers for *every* program that
# resolves a shell by name, including this repository's own tests, the system's
# scripts, and the terminal that would be needed to undo it.
#
# So the default lands in libexec, which is the directory convention for
# programs meant to be named by their full path rather than found. The names
# stay plain there, because the name is load-bearing at the other end: `login`
# and every terminal emulator start a login shell as `-bash`, and
# driver.LoginShell reads exactly that, so a binary renamed `sh-bash` would
# announce itself as `-sh-bash`, print the wrong `$0` in diagnostics, and match
# no shebang anybody has written.
#
# SHELLDIR is overridable, so shadowing PATH is possible — it just cannot
# happen by accident. `install` refuses when SHELLDIR is on PATH unless
# ALLOW_PATH_SHADOW=1 is passed with it.
#
# GOBIN is deliberately *not* consulted. It is on PATH on virtually every
# machine that sets it, so honoring it would mean `make install` shadowing the
# system shells for whoever had it set — which is also the reason to never run
# `go install ./cmd/...` in this repository.
#
# DESTDIR is the packaging convention and prefixes only the copy. The PATH
# check asks about SHELLDIR, because a staged tree's contents end up there.
#
# FUNCDIR is the other half of an installation, and it is not under SHELLDIR.
# One dialect searches a path of directories for function definition files,
# and driver.functionSearchDirs derives that path from where the binary sits:
# `$(PREFIX)/share/sh/site-functions` then `$(PREFIX)/share/sh/functions`. So
# the shipped functions go in the second of those, and a package that wants to
# override one drops its own into the first.
PREFIX ?= /usr/local
SHELLDIR ?= $(PREFIX)/libexec/sh
FUNCDIR ?= $(PREFIX)/share/sh/functions
SHELLS := sh bash zsh ksh dash ash

# The autoloadable functions this shell ships, and the directory they are kept
# in within the checkout. That directory is *also* where an uninstalled build
# finds them: `build/zsh` derives its prefix as the checkout root, so
# `share/sh/functions` is on `$fpath` for a binary that was never installed,
# and `make check` exercises the files that would ship rather than a copy.
#
# The list is the directory rather than a list beside it, so a file added here
# is installed and packaged without anybody remembering to say so twice. The
# archive in .goreleaser.yaml takes the same directory as a glob.
FUNCSRC := share/sh/functions
FUNCS := $(sort $(notdir $(wildcard $(FUNCSRC)/*)))

.PHONY: all build test test-cover fmt vet tidy clean check corpus-guard oracle oracle-check conformance conformance-gated conformance-dialects axis-sweep wild wild-run wild-run-contained fmt-wild smoke acp acp-wire acp-bench startup perfgate suite-panel bash-suite zsh-suite ksh-suite dash-suite install uninstall

all: build

build:
	go build ./...

# -p bounds how many packages `go test` builds and runs at once. Without it
# Go uses GOMAXPROCS, which on an 18-core machine means one `make check` can
# have eighteen race-instrumented packages resident at once -- and several
# agents run their gates concurrently, so the real figure is that times the
# number of them. Measured with eleven concurrent runs: load average 64 on 18
# cores, two `syntax.test` processes alone at 242% and 176%, and everything
# else on the machine waiting.
#
# Four is the same bound `.golangci.yml` uses and for the same reason: it
# keeps the realistic worst case near the core count instead of far past it.
# One run in isolation is a little slower; several together are faster,
# because the machine stops thrashing. It also makes the timing-sensitive
# tests here -- pty sessions, job control, the descriptor loop -- less
# load-dependent, which is the direction that matters when a flaky test
# manufactures false kills in a mutation run.
TESTFLAGS ?= -p 4

# Two runs, because one test does not want the race detector and everything
# else does. TestNoDialectCombinationPanics is a recover()-based panic sweep
# over pure parsing: instrumentation cannot make it find a panic it was not
# already finding, and it costs 4.7x — measured on a CI runner, 89s against
# 19s for the same 32 vectors, which was ~80% of everything CI spent testing
# this module. So the instrumented pass runs a probe of it and this
# second pass runs the sweep itself, over every vector and the whole corpus.
# Dropping the second line does not make `make test` faster; it makes it stop
# proving the thing that file exists to prove.
test:
	go test -race $(TESTFLAGS) ./...
	go test $(TESTFLAGS) ./syntax -run TestNoDialectCombinationPanics

test-cover:
	go test -race $(TESTFLAGS) -coverprofile=coverage.out ./...

fmt:
	go tool gofumpt -l -w .

vet:
	go vet ./...

# There is deliberately no `lint` target. golangci-lint runs in CI's `Lint`
# job and nowhere else — it is not a pre-commit hook here, see
# .pre-commit-config.yaml for the measurement that took it out, and AGENTS.md
# for the rule. A hand-started run is a second copy of work that is already
# happening, and several of them at once is what starves this machine.

tidy:
	go mod tidy

clean:
	rm -f coverage.out
	rm -rf $(BINDIR)
	go clean

check: fmt vet test corpus-guard oracle-check

install: ## Build the five shells and install them into $(SHELLDIR) — see docs/install.md
	@case ":$$PATH:" in \
	*:"$(SHELLDIR)":*) \
		if [ "$(ALLOW_PATH_SHADOW)" != 1 ]; then \
			echo "make install: refusing — $(SHELLDIR) is on your PATH." >&2; \
			echo "  The binaries are named bash, zsh, ksh, dash, ash and sh, so installing them" >&2; \
			echo "  there shadows the system shells for everything that resolves one by name." >&2; \
			echo "  Install somewhere off PATH (the default is $(PREFIX)/libexec/sh) and name" >&2; \
			echo "  the full path to chsh, or repeat this with ALLOW_PATH_SHADOW=1." >&2; \
			exit 1; \
		fi; \
		echo "make install: $(SHELLDIR) is on PATH — shadowing the system shells, as asked" >&2 ;; \
	esac
	@mkdir -p $(BINDIR)/staged
	@for s in $(SHELLS); do go build -trimpath -o $(BINDIR)/staged/$$s ./cmd/$$s || exit 1; done
	@install -d "$(DESTDIR)$(SHELLDIR)"
	@for s in $(SHELLS); do install -m 0755 $(BINDIR)/staged/$$s "$(DESTDIR)$(SHELLDIR)/$$s" || exit 1; done
	@install -d "$(DESTDIR)$(FUNCDIR)"
	@for f in $(FUNCS); do install -m 0644 $(FUNCSRC)/$$f "$(DESTDIR)$(FUNCDIR)/$$f" || exit 1; done
	@echo "installed into $(DESTDIR)$(SHELLDIR): $(SHELLS)"
	@echo "installed into $(DESTDIR)$(FUNCDIR): $(FUNCS)"
	@echo "run one:      $(SHELLDIR)/bash -i"
	@echo "login shell:  docs/install.md — /etc/shells and chsh, and what a session does not read yet"

uninstall: ## Remove the shells and functions `make install` put in $(SHELLDIR) and $(FUNCDIR)
	@for s in $(SHELLS); do rm -f "$(DESTDIR)$(SHELLDIR)/$$s"; done
	@rmdir "$(DESTDIR)$(SHELLDIR)" 2>/dev/null || true
	@for f in $(FUNCS); do rm -f "$(DESTDIR)$(FUNCDIR)/$$f"; done
	@rmdir "$(DESTDIR)$(FUNCDIR)" 2>/dev/null || true
	@echo "removed $(SHELLS) from $(DESTDIR)$(SHELLDIR)"
	@echo "removed $(FUNCS) from $(DESTDIR)$(FUNCDIR)"
	@echo "if this was your login shell, change it back first — chsh -s /bin/zsh — and drop the line from /etc/shells"

corpus-guard: ## Fail if the corpus has lost a case since the merge base with main
	@go run ./internal/cmd/corpusguard

oracle: ## Regenerate docs/spec/measurements.md and the golden record from a live panel run
	go run ./internal/cmd/oracle

oracle-check: ## Fail if the reference shells no longer behave as recorded
	go run ./internal/cmd/oracle -check

conformance: ## Grade the core driver against bash over the whole corpus
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/sh-under-test ./cmd/sh
	@go run ./internal/cmd/oracle -bin $(BINDIR)/sh-under-test -binargs "-dialect bash" $(ARGS)

conformance-gated: ## Run the corpus twice — plain and under a sandbox policy — and report what the policy changed
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/sh-under-test ./cmd/sh
	@go run ./internal/cmd/oracle -bin $(BINDIR)/sh-under-test -binargs "-dialect bash" -gated $(ARGS)

fmt-wild: ## Lay out every shell script installed on this machine and check nothing changed but the layout
	@go run ./internal/cmd/fmtwild $(ARGS)

wild: ## Parse the shell scripts installed on this machine and report what fails (SH_WILD_DIRS adds framework trees)
	@go run ./internal/cmd/wild $(ARGS)

wild-run: ## Also RUN each script that parses, under both shells, and report where they disagree
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/wild-bash ./cmd/bash
	@go run ./internal/cmd/wild -run $(BINDIR)/wild-bash $(ARGS)

wild-run-contained: ## wild-run with the shell under a policy: it may write only in the directory each run is given
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/wild-sh ./cmd/sh
	@go run ./internal/cmd/wild -run $(BINDIR)/wild-sh -runargs "-dialect bash" -contained $(ARGS)

smoke: ## Drive a realistic interactive session through a pty and report, per feature, what works
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/smoke-bash ./cmd/bash
	@go build -o $(BINDIR)/smoke-zsh ./cmd/zsh
	@go run ./internal/cmd/smoke -bash $(BINDIR)/smoke-bash -zsh $(BINDIR)/smoke-zsh $(ARGS)

# Every dialect a person actually runs the shell as, because the binary's
# default is `core` and a grade of the core alone was a grade of the one
# dialect nobody uses (#2258). Narrow it with `make acp ACP_DIALECTS=zsh`.
ACP_DIALECTS ?= bash zsh core

acp: ## Drive the Agent Client Protocol front end as a client would, in every dialect, and report what works, what it costs, and what a pipe would have seen
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/acp-sh ./cmd/sh
	@go build -o $(BINDIR)/acpcheck ./internal/cmd/acpcheck
	@rc=0; for d in $(ACP_DIALECTS); do \
		$(BINDIR)/acpcheck -bin $(BINDIR)/acp-sh -dialect $$d $(ARGS) || rc=1; \
	done; exit $$rc

acp-wire: ## Print a real annotated ACP session, message by message, for showing somebody
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/acp-sh ./cmd/sh
	@go build -o $(BINDIR)/acpcheck ./internal/cmd/acpcheck
	@$(BINDIR)/acpcheck -bin $(BINDIR)/acp-sh -dialect $(firstword $(ACP_DIALECTS)) -wire $(ARGS)

# Deliberately not in `check`, and the comment is the rule: this is thousands
# of shell processes against a 3000-row corpus, on demand. See the `lint`
# comment above for what wiring a slow thing into every commit costs here.
axis-sweep: ## Move every axis in interp.Semantics and report the ones nothing objected to (#2031)
	@mkdir -p $(BINDIR)
	@go build -tags shaxissweep -o $(BINDIR)/axis-sh ./cmd/sh
	@go run ./internal/cmd/axissweep -bin $(BINDIR)/axis-sh $(ARGS)

sandbox: ## Try every way a script has of reaching the filesystem, against the shipped binaries, and report what the boundary stopped
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/sandbox-sh ./cmd/sh
	@go build -o $(BINDIR)/sandbox-bash ./cmd/bash
	@go build -o $(BINDIR)/sandbox-zsh ./cmd/zsh
	@go build -o $(BINDIR)/sandbox-ksh ./cmd/ksh
	@go build -o $(BINDIR)/sandbox-dash ./cmd/dash
	@go run ./internal/cmd/sandboxcheck -bin $(BINDIR)/sandbox-sh \
		-dialect-bin bash=$(BINDIR)/sandbox-bash \
		-dialect-bin zsh=$(BINDIR)/sandbox-zsh \
		-dialect-bin ksh=$(BINDIR)/sandbox-ksh \
		-dialect-bin posix=$(BINDIR)/sandbox-dash $(ARGS)

# A shell's own test suite, run through that shell and through the dialect
# binary claiming to be it. Report-only, never a gate, and never committed:
# the suites are other projects' work — bash's is GPLv3 — so each is fetched
# at test time into the gitignored build directory and only its tests are
# unpacked. See internal/suite for what may be reported and what may not.
#
# One target per dialect from the first commit, because the multi-dialect
# shape is the point: an instrument that could only grade bash would bend the
# substrate toward bash. The columns that are not built yet print why.
suite-panel: ## List the shells whose own suite this can run, and what the unbuilt columns still need
	@go run ./internal/cmd/suitecheck -panel

bash-suite: ## Run bash's own tests/ through real bash and through cmd/bash, and report where they part
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/suite-bash ./cmd/bash
	@go run ./internal/cmd/suitecheck -dialect bash -bin $(BINDIR)/suite-bash -build $(BINDIR) $(ARGS)

zsh-suite: ## Run zsh's own Test/ through real zsh and through cmd/zsh (not yet a column; prints why)
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/suite-zsh ./cmd/zsh
	@go run ./internal/cmd/suitecheck -dialect zsh -bin $(BINDIR)/suite-zsh -build $(BINDIR) $(ARGS)

ksh-suite: ## Run ksh93's own tests through real ksh93 and through cmd/ksh (not yet a column; prints why)
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/suite-ksh ./cmd/ksh
	@go run ./internal/cmd/suitecheck -dialect ksh -bin $(BINDIR)/suite-ksh -build $(BINDIR) $(ARGS)

dash-suite: ## dash has no suite of its own; prints why
	@go run ./internal/cmd/suitecheck -dialect dash $(ARGS)

conformance-dialects: ## Grade each dialect binary against the shell it claims to be
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/our-bash ./cmd/bash
	@go build -o $(BINDIR)/our-zsh ./cmd/zsh
	@go build -o $(BINDIR)/our-dash ./cmd/dash
	@go build -o $(BINDIR)/our-ksh ./cmd/ksh
	@go run ./internal/cmd/oracle -bin $(BINDIR)/our-bash -against bash $(ARGS)
	@go run ./internal/cmd/oracle -bin $(BINDIR)/our-zsh -against zsh $(ARGS)
	@go run ./internal/cmd/oracle -bin $(BINDIR)/our-dash -against dash $(ARGS)
	@go run ./internal/cmd/oracle -bin $(BINDIR)/our-ksh -against ksh93 $(ARGS)

acp-bench: ## Time the protocol's own costs in ns/op, against the process-per-command it replaces
	@go test ./internal/acpcheck/ -run XXX -bench . -benchtime 50x -count 3 $(ARGS)

startup: ## Time process start to a prompt, and the -c path, against the real shells
	@go test ./internal/startupcost/ -run XXX -bench . -benchtime 40x -count 3 $(ARGS)

perfgate: ## Fail if any dialect is slower than the shell it claims to be (#1403)
	@SH_PERFGATE=1 go test ./internal/startupcost/ -run TestNoDialectIsSlowerThanItsOriginal -v -count 1 -timeout 30m $(ARGS)
