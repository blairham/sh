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
PREFIX ?= /usr/local
SHELLDIR ?= $(PREFIX)/libexec/sh
SHELLS := sh bash zsh ksh dash

.PHONY: all build test test-cover fmt vet lint tidy clean check corpus-guard oracle oracle-check conformance conformance-gated conformance-dialects wild wild-run wild-run-contained smoke startup perfgate install uninstall

all: build

build:
	go build ./...

test:
	go test -race ./...

test-cover:
	go test -race -coverprofile=coverage.out ./...

fmt:
	go tool gofumpt -l -w .

vet:
	go vet ./...

lint:
	go tool golangci-lint run

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
			echo "  The binaries are named bash, zsh, ksh, dash and sh, so installing them" >&2; \
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
	@echo "installed into $(DESTDIR)$(SHELLDIR): $(SHELLS)"
	@echo "run one:      $(SHELLDIR)/bash -i"
	@echo "login shell:  docs/install.md — /etc/shells and chsh, and what a session does not read yet"

uninstall: ## Remove the shells `make install` put in $(SHELLDIR)
	@for s in $(SHELLS); do rm -f "$(DESTDIR)$(SHELLDIR)/$$s"; done
	@rmdir "$(DESTDIR)$(SHELLDIR)" 2>/dev/null || true
	@echo "removed $(SHELLS) from $(DESTDIR)$(SHELLDIR)"
	@echo "if this was your login shell, change it back first — chsh -s /bin/zsh — and drop the line from /etc/shells"

corpus-guard: ## Fail if the corpus has lost a case since the merge base with main
	@go run ./cmd/corpusguard

oracle: ## Regenerate docs/spec/measurements.md and the golden record from a live panel run
	go run ./cmd/oracle

oracle-check: ## Fail if the reference shells no longer behave as recorded
	go run ./cmd/oracle -check

conformance: ## Grade the core driver against bash over the whole corpus
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/sh-under-test ./cmd/sh
	@go run ./cmd/oracle -bin $(BINDIR)/sh-under-test -binargs "-dialect bash" $(ARGS)

conformance-gated: ## Run the corpus twice — plain and under a sandbox policy — and report what the policy changed
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/sh-under-test ./cmd/sh
	@go run ./cmd/oracle -bin $(BINDIR)/sh-under-test -binargs "-dialect bash" -gated $(ARGS)

wild: ## Parse the shell scripts installed on this machine and report what fails (SH_WILD_DIRS adds framework trees)
	@go run ./cmd/wild $(ARGS)

wild-run: ## Also RUN each script that parses, under both shells, and report where they disagree
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/wild-bash ./cmd/bash
	@go run ./cmd/wild -run $(BINDIR)/wild-bash $(ARGS)

wild-run-contained: ## wild-run with the shell under a policy: it may write only in the directory each run is given
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/wild-sh ./cmd/sh
	@go run ./cmd/wild -run $(BINDIR)/wild-sh -runargs "-dialect bash" -contained $(ARGS)

smoke: ## Drive a realistic interactive session through a pty and report, per feature, what works
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/smoke-bash ./cmd/bash
	@go build -o $(BINDIR)/smoke-zsh ./cmd/zsh
	@go run ./cmd/smoke -bash $(BINDIR)/smoke-bash -zsh $(BINDIR)/smoke-zsh $(ARGS)

conformance-dialects: ## Grade each dialect binary against the shell it claims to be
	@mkdir -p $(BINDIR)
	@go build -o $(BINDIR)/our-bash ./cmd/bash
	@go build -o $(BINDIR)/our-zsh ./cmd/zsh
	@go build -o $(BINDIR)/our-dash ./cmd/dash
	@go build -o $(BINDIR)/our-ksh ./cmd/ksh
	@go run ./cmd/oracle -bin $(BINDIR)/our-bash -against bash $(ARGS)
	@go run ./cmd/oracle -bin $(BINDIR)/our-zsh -against zsh $(ARGS)
	@go run ./cmd/oracle -bin $(BINDIR)/our-dash -against dash $(ARGS)
	@go run ./cmd/oracle -bin $(BINDIR)/our-ksh -against ksh93 $(ARGS)

startup: ## Time process start to a prompt, and the -c path, against the real shells
	@go test ./internal/startupcost/ -run XXX -bench . -benchtime 40x -count 3 $(ARGS)

perfgate: ## Fail if any dialect is slower than the shell it claims to be (#1403)
	@SH_PERFGATE=1 go test ./internal/startupcost/ -run TestNoDialectIsSlowerThanItsOriginal -v -count 1 -timeout 30m $(ARGS)
