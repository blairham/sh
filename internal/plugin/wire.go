// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package plugin runs a builtin whose implementation is in another process,
// in whatever language its author chose.
//
// The design is docs/design/plugins.md and the rule it is written around is
// the one to check any change here against:
//
//	A plugin adds no capability. It relocates one across a process boundary.
//
// So every method below corresponds to a seam this repository already
// publishes — interp.Builtin, and the handful of *interp.Runner accessors a
// registered builtin genuinely cannot do without — and to nothing else. There
// is no plugin/openFile and no plugin/runCommand: a plugin is already free to
// open whatever it likes, and offering it a route through us would add inward
// attack surface to the process holding the script's variables without
// removing any freedom it had.
//
// # The transport, and the second one
//
// The transport is JSON-RPC 2.0 over the plugin's own standard input and
// output — internal/jsonrpc, the framing internal/acp already drove three
// non-Go agents over. The whole of what a plugin has to do is read lines of
// JSON from standard input and write lines of JSON to standard output, which
// is a page in any language on the machine.
//
// It was chosen over gRPC, which is what #738 asked for, and
// docs/design/plugins.md carries the measured argument: `go list -deps
// ./cmd/sh` names zero packages outside the standard library and this module,
// so gRPC and protobuf would be the first runtime dependencies this
// repository has ever had; they would add a code generator to the build; and
// they would put HTTP/2 framing in front of the plugin author most likely to
// show up, who is writing a short script. The claim that reaching other
// languages needs gRPC is falsified thirty lines away rather than argued
// against.
//
// A second transport is expected — "JSON-RPC now, gRPC later" is the
// maintainer's decision, 2026-09-06 — and there is deliberately no
// abstraction here for it. Two transports invented before either has a plugin
// would be shaped by neither, which is #801's warning one layer up: a seam
// ends up shaped by its first remote consumer rather than by its callers.
//
// What would have to be true to add one, so that the argument is made against
// evidence rather than made again from scratch: a real plugin needing typed
// streaming or generated stubs, or a measured cost this encoding is paying.
// The place it would go is Launch, whose result is a *Host — a Host is a
// declared surface plus a way to make a call, and neither the message shapes
// above nor anything in command.go names a wire format. Two things would move:
// the base64 in outputParams and readResult, which exists because JSON strings
// are UTF-8 and a shell's streams are bytes, and the framing itself. Nothing
// about the surface, the security model or the lifetime rules is transport
// business, and a second transport that changed one of those would be a
// different design rather than the same one spoken differently.
package plugin

import (
	"encoding/base64"
	"fmt"
)

// Version is the protocol revision this host speaks.
//
// One integer, and a mismatch is a refusal rather than a negotiation — see
// Host.handshake. That is the policy format's `version 1` rule read at the
// protocol layer: a policy half-understood allows what it was written to
// refuse, and a protocol half-understood produces a builtin that does
// something other than what the script asked for.
const Version = 1

// The methods the host calls on a plugin.
const (
	// MethodInitialize is the handshake, and the host speaks first. A plugin
	// that dies before saying anything is then a launch failure with a
	// diagnostic, rather than a host blocked on a first message that will
	// never come.
	MethodInitialize = "initialize"
	// MethodInvoke runs one of the plugin's commands.
	MethodInvoke = "command/invoke"
	// MethodCancel says the shell has given up on a call. A notification
	// rather than a request: there is nothing to answer, and the call it names
	// is going to fail either way. What it buys the plugin is the chance to
	// stop cleanly before the host kills it.
	MethodCancel = "command/cancel"
)

// The methods a plugin calls on the host. Each is here because a process that
// is not this one has no other way to ask, and the list is deliberately
// shorter than *interp.Runner.
const (
	// MethodGetVar reads a shell variable. The environment a plugin inherited
	// is the environment at the moment it was launched; the shell's variables
	// are the Runner's and change under it.
	MethodGetVar = "shell/getVar"
	// MethodSetVar writes a shell variable. This is the reason Register
	// exists at all: `read` has to put a value into a variable of the calling
	// shell, and nothing a child process can do reaches it.
	MethodSetVar = "shell/setVar"
	// MethodDir is the shell's working directory, which is the Runner's and
	// not the process's, so a plugin resolving a relative path has no other
	// way to ask.
	MethodDir = "shell/dir"
	// MethodRead reads from the stream the command's standard input is
	// pointing at, which under a pipeline or a redirection is not the
	// process's.
	MethodRead = "shell/read"
	// MethodDiagnose writes a complaint located and named the dialect's way,
	// rather than an unattributed line on standard error.
	MethodDiagnose = "shell/diagnose"
	// MethodOutput is a chunk of what the command wrote. A notification, so
	// that a plugin streams output without waiting for an acknowledgement per
	// chunk — and handled on the read loop in order, which is what makes
	// `echo one; echo two` arrive in that order.
	MethodOutput = "command/output"
)

// initializeRequest is what the host sends first.
type initializeRequest struct {
	ProtocolVersion int `json:"protocolVersion"`
}

// initializeResult is the plugin's declared surface.
//
// Commands is every name it claims, declared here and fixed for its life. A
// plugin rewriting the command table while a script runs is a shell whose
// `type` answer depends on what has already run.
type initializeResult struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Name            string   `json:"name"`
	Commands        []string `json:"commands"`
}

// invokeRequest is one call of one of the plugin's commands.
//
// Args are the operands without the name, which is interp.Builtin's own shape:
// `greet a b` gives {"a", "b"}. Name is beside them because a plugin claiming
// several commands needs to know which one this is, and because a plugin must
// not have to infer it from a vector it was never given.
//
// Call is the id every message about this call carries. It exists because a
// shell has more than one goroutine running commands — a background job, each
// half of a pipeline — so two calls into one plugin are the ordinary case, and
// a chunk of output that named no call would land on whichever stream was
// most recently asked for.
type invokeRequest struct {
	Call string   `json:"call"`
	Name string   `json:"name"`
	Args []string `json:"args"`
}

// invokeResult is the command's exit status, and nothing else. Output has
// already been streamed by the time this arrives, because notifications are
// delivered on the read loop in the order they were sent and the response
// travels the same loop behind them.
type invokeResult struct {
	Status int `json:"status"`
}

// cancelParams names the call the shell has given up on.
type cancelParams struct {
	Call string `json:"call"`
}

// The two streams a chunk of output can be for. A plugin's own standard error
// is a third thing and is not this: the host relays that, prefixed, whether or
// not a call is in flight.
const (
	StreamOut = "out"
	StreamErr = "err"
)

// outputParams is one chunk of what a command wrote.
//
// Data is base64, which is the one place gRPC would have been plainly better:
// JSON strings are UTF-8 and a shell's streams are bytes, so this costs a
// third more bytes and a copy. It is acceptable for the reason
// docs/design/plugins.md gives rather than by assumption — a plugin whose job
// is moving bulk bytes should be an ordinary external command, which inherits
// real descriptors and pays none of this. If that stops being true, the
// encoding is what changes and none of the shapes here do.
type outputParams struct {
	Call   string `json:"call"`
	Stream string `json:"stream"`
	Data   string `json:"data"`
}

// getVarParams and getVarResult read one variable. Set says whether the shell
// has it at all, so a plugin can tell an unset variable from an empty one —
// which is a distinction a shell spends a great deal of its grammar on.
//
// Every host method carries the call it belongs to, this one included, and it
// is not ceremony: a subshell has its own variables and its own directory, and
// the two halves of a pipeline are two Runners. A host method that named no
// call would read whichever of them answered most recently.
type getVarParams struct {
	Call string `json:"call"`
	Name string `json:"name"`
}

type getVarResult struct {
	Value string `json:"value"`
	Set   bool   `json:"set"`
}

type setVarParams struct {
	Call  string `json:"call"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type dirParams struct {
	Call string `json:"call"`
}

type dirResult struct {
	Dir string `json:"dir"`
}

// readParams asks for at most Max bytes of the command's input.
//
// A pull rather than a push, which is a deliberate departure from the design
// document's first sketch of "input chunks, streamed in", and the reason is
// the lifetime rule that document states: nothing blocks on an operation whose
// only unblocking event is a cooperating peer. Pushing input to a plugin that
// never reads it makes the host's write block forever on a full pipe — #690 in
// a new costume — and there is no bound on it that is not a lie about how much
// input there was. A read the plugin asks for cannot outrun the plugin.
//
// A Max of zero or less asks for the host's own chunk size, so a plugin that
// omits the field gets something sensible rather than nothing.
type readParams struct {
	Call string `json:"call"`
	Max  int    `json:"max"`
}

// readResult is what was read. EOF is separate from an empty Data because a
// short read is not the end of a stream, and a plugin that treated it as one
// would truncate a pipeline.
type readResult struct {
	Data string `json:"data"`
	EOF  bool   `json:"eof"`
}

// diagnoseParams is the complaint. Format is deliberately not a format string
// on this side: the text arrives already assembled, because a format string
// from another process applied to arguments from another process is a way for
// a plugin to crash the shell's formatter rather than a feature.
type diagnoseParams struct {
	Call string `json:"call"`
	Text string `json:"text"`
}

// encode and decode are the base64 the byte-carrying fields use. Standard
// encoding with padding, which is what every language's standard library
// offers under the name base64.
func encode(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func decode(s string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("not base64: %w", err)
	}
	return b, nil
}
