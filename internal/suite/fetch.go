// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// The fetch is the part of this instrument that has to be got right first
// time, because what it risks is not a wrong number. bash's suite is GPLv3
// and this repository is Apache-2.0 with a NOTICE saying the tree contains no
// third-party code. Committing a suite would relicense the repository by
// accident, and no score is worth that.
//
// Three things keep it out, and they are deliberately not one thing:
//
//  1. It lands under the build directory, which .gitignore excludes, so it
//     cannot be committed without somebody forcing it.
//  2. It lands under a directory named for the distribution — bash-5.3 — so
//     wild.Denied refuses to open anything inside it. That guard had a
//     carve-out for this project's own working tree until #1039, which is to
//     say the fetch's own destination was the one place on the machine the
//     rule did not apply.
//  3. Only the suite is unpacked. The archive carries a whole shell's source
//     and this streams past it: what reaches the disk is the test directory
//     and three C files, so there is no shell implementation here to read
//     even for somebody who went looking.

// ErrOffline is returned when the archive could not be fetched. It is a skip
// and not a failure: this instrument is a report, and a machine with no
// network has not told us anything about the shell.
var ErrOffline = errors.New("suite archive could not be fetched")

// ErrDigest is returned when the archive is not the one that was pinned.
//
// It stops the run rather than grading what arrived. A suite silently
// swapped for another version would move every number in the report with
// nothing in the report saying so, which is the shape of finding this
// repository has lost most to: an instrument that answers confidently about
// something other than what it names.
var ErrDigest = errors.New("suite archive digest does not match the pin")

// Root is where fetched suites live, under the build directory the Makefile
// already puts graded binaries in and .gitignore already excludes.
func Root(buildDir string) string { return filepath.Join(buildDir, "suite") }

// Dir is where one suite's files land.
func (s Suite) Dir(buildDir string) string { return filepath.Join(Root(buildDir), s.Root) }

// stampName is written beside an unpacked suite so a second run can tell the
// tree came from the pinned archive rather than from a hand copy or an
// interrupted unpack.
const stampName = ".fetched"

// Fetch makes sure the suite is unpacked under buildDir and returns its
// directory.
//
// A tree already unpacked from this exact archive is left alone; anything
// else is replaced. The digest is the identity, not the version string: two
// runs of a release with the same name are the same measurement only if the
// bytes were the same.
func Fetch(ctx context.Context, s Suite, buildDir string, refetch bool) (string, error) {
	dir := s.Dir(buildDir)
	if !refetch && stampMatches(dir, s.SHA256) {
		return dir, nil
	}
	archive, err := fetchArchive(ctx, s)
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := unpack(archive, s, dir); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, stampName), []byte(s.SHA256+"\n"), 0o600); err != nil {
		return "", err
	}
	return dir, nil
}

func stampMatches(dir, want string) bool {
	got, err := os.ReadFile(filepath.Join(dir, stampName))
	return err == nil && strings.TrimSpace(string(got)) == want
}

// One DNS answer used to decide whether a whole column existed.
//
// The bash column measured nothing on the Suites run of 2026-09-24 because
// `ftp.gnu.org` did not resolve inside the job container — `server
// misbehaving` from Docker's embedded resolver at 127.0.0.11, which is
// transient by name. Nothing was killed and nothing came near a memory bound;
// the step's own cgroup read said `oom_kill 0` against `memory.max = max`.
// The single request was the whole of the column's availability. #4435.
//
// So a fetch is now two axes rather than one call: each source is tried
// [fetchAttempts] times with a growing pause between, and [Suite.Mirrors]
// supplies the next source when a whole host is unreachable. The two are
// deliberately separate — a retry covers a resolver having a bad second, and a
// mirror covers a host being down for the afternoon, and neither covers the
// other.
//
// The pin is unchanged and is what makes a mirror safe to add at all: every
// source is graded against [Suite.SHA256] before a byte of it is unpacked, so
// a mirror is a second route to the *same bytes* and never a second version.
const fetchAttempts = 3

// fetchBackoff is the pause before the second attempt at a source; it doubles
// for each one after. A package variable rather than a constant for the reason
// fetchClient is one — a test shortens it, because a retry path nothing
// exercises is untested code that only runs on the day it is needed.
var fetchBackoff = 2 * time.Second

// sources is where this suite's archive may be fetched from, in order: the
// canonical URL first, then any mirrors.
func (s Suite) sources() []string {
	out := make([]string, 0, 1+len(s.Mirrors))
	if s.URL != "" {
		out = append(out, s.URL)
	}
	return append(out, s.Mirrors...)
}

// fetchArchive returns the pinned archive from the first source that serves
// it, or says everything it tried and why each one did not.
//
// The digest is checked *here*, inside the loop, rather than once by the
// caller. That is what keeps a mirror from being able to change the
// measurement: a source that serves something other than the pin is not used
// and the next source is tried, and if every source failed the pin the error
// is [ErrDigest] rather than [ErrOffline] — a wrong archive everywhere is a
// stale pin, which is a different thing from a network that was not there.
func fetchArchive(ctx context.Context, s Suite) ([]byte, error) {
	var (
		tried      []string
		mismatched bool
	)
	for _, url := range s.sources() {
		for attempt := 1; attempt <= fetchAttempts; attempt++ {
			body, err := download(ctx, url)
			if err == nil {
				sum := sha256.Sum256(body)
				if got := hex.EncodeToString(sum[:]); got != s.SHA256 {
					// Not retried: the same source will serve the same bytes
					// a second later. Move on to the next one.
					mismatched = true
					tried = append(tried, fmt.Sprintf("%s: have %s, want %s", url, got, s.SHA256))
					break
				}
				return body, nil
			}
			tried = append(tried, fmt.Sprintf("%s (attempt %d/%d): %v", url, attempt, fetchAttempts, err))
			if ctx.Err() != nil {
				// A Ctrl-C is not a flaky mirror. Stop rather than spending
				// the remaining attempts and every mirror on a canceled run.
				tried = append(tried, ctx.Err().Error())
				return nil, fmt.Errorf("%w: %s", ErrOffline, strings.Join(tried, "; "))
			}
			if attempt < fetchAttempts {
				if err := pause(ctx, fetchBackoff<<(attempt-1)); err != nil {
					tried = append(tried, err.Error())
					return nil, fmt.Errorf("%w: %s", ErrOffline, strings.Join(tried, "; "))
				}
			}
		}
	}
	if len(tried) == 0 {
		return nil, fmt.Errorf("%w: %s has no source to fetch from", ErrOffline, s.Name)
	}
	if mismatched {
		return nil, fmt.Errorf("%w: %s", ErrDigest, strings.Join(tried, "; "))
	}
	return nil, fmt.Errorf("%w: %s", ErrOffline, strings.Join(tried, "; "))
}

// pause waits, or returns the reason it stopped waiting.
func pause(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// fetchTimeout bounds the whole request — connect, headers and body — because
// nothing else does.
//
// This ran on `http.DefaultClient`, which has no timeout of its own, under a
// context from `signal.NotifyContext`, which has no deadline of its own. So a
// stalled read here had exactly one bound: the CI job's own `timeout-minutes:
// 30`, reached with nothing printed, because the report is written after the
// sweep. That is the shape of the one run in sixty that #3082 could not
// explain — job 35323005114 ran 34m41s, its step never completed, and GitHub
// retained no log for it at all.
//
// Generous rather than tight: the archive is a few megabytes and the fetch
// happens once per build directory, so this is a bound on *hanging* and not a
// performance budget. A cold runner on a slow morning must not be turned into
// a red job by it.
//
// Not a context deadline, because the caller's context is the one that carries
// interruption and wrapping it here would put the bound on whichever of the
// two expires first — which reads as "the fetch timed out" for a Ctrl-C. A
// client timeout says what it is.
const fetchTimeout = 5 * time.Minute

// fetchClient is that bound, as a value a test can shorten. A package variable
// rather than a client built per call so that a test can prove the timeout is
// *there*: a bound nothing exercises is the bound that turns out to be missing
// the day it was needed.
var fetchClient = &http.Client{Timeout: fetchTimeout}

// download reads the archive into memory. They are a few megabytes and
// holding one is cheaper than a partial file on disk that a later run would
// have to decide about.
func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := fetchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %s", resp.Status)
	}
	// Bounded, because this writes to a developer's disk and an unbounded
	// read from a URL in a table is how a typo becomes a full filesystem.
	const most = 64 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, most+1))
	if err != nil {
		return nil, err
	}
	if len(body) > most {
		return nil, fmt.Errorf("archive larger than %d bytes", most)
	}
	return body, nil
}

// unpack writes the parts of the archive the suite names, and discards the
// rest without ever putting it on the disk.
func unpack(archive []byte, s Suite, dir string) error {
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	tr := tar.NewReader(zr)
	wrote := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		rel, ok := s.Wanted(hdr.Name)
		if !ok {
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(filepath.Join(dir, rel), 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeMember(tr, filepath.Join(dir, rel)); err != nil {
				return err
			}
			wrote++
		default:
			// Links and devices are not unpacked. A suite that needs one
			// would fail identically under both shells, and following a link
			// out of the destination is the classic way an archive writes
			// where it was not asked to.
		}
	}
	if wrote == 0 {
		return fmt.Errorf("archive held nothing matching %v under %s", s.Include, s.Root)
	}
	return nil
}

func writeMember(r io.Reader, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	// Bounded for the same reason the download is, one member at a time.
	const most = 32 << 20
	if _, err := io.Copy(f, io.LimitReader(r, most)); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Wanted reports whether an archive member is part of the suite, and where it
// goes relative to the suite's directory.
//
// The rules are all denials with one allow, in this order: the member has to
// be under the archive's single root, has to be under one of the Include
// prefixes or be one of them exactly, must not carry an Exclude suffix, and
// must not climb. The climb check is last because it is about the path rather
// than about the suite, and it is present because an archive is somebody
// else's bytes: `..` in a member name is how one writes outside the directory
// it was unpacked into.
func (s Suite) Wanted(name string) (string, bool) {
	name = path.Clean(name)
	rel, ok := strings.CutPrefix(name, s.Root+"/")
	if !ok {
		return "", false
	}
	for _, suffix := range s.Exclude {
		if strings.HasSuffix(rel, suffix) {
			return "", false
		}
	}
	matched := false
	for _, want := range s.Include {
		if strings.HasSuffix(want, "/") {
			if strings.HasPrefix(rel, want) || rel == strings.TrimSuffix(want, "/") {
				matched = true
			}
			continue
		}
		if rel == want {
			matched = true
		}
	}
	if !matched {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.FromSlash(rel), true
}
