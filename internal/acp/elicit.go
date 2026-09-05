// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp

import (
	"context"
	"encoding/json"
)

// elicitation/create: where a question for a person lives.
//
// The permission request is not the only thing an agent may need a human for,
// and it is the only one the protocol had until this. An agent that needs a
// choice, a name, a confirmation — anything a schema can describe — asks for it
// here, and a client that can reach a person answers.
//
// It matters on this shell's *agent* side too, and by its absence. That side
// is non-interactive because the protocol occupies the descriptors a prompt
// would need: ACP is standard input and standard output, and a prompt drawn
// into them is a message the client cannot read. elicitation/create is the
// protocol's own answer to that, and it is the direction this file does not
// go — see docs/design/acp.md for why a script's `read` cannot reach it yet.

// MethodCreateElicitation is the client method an agent calls to ask a person.
const MethodCreateElicitation = "elicitation/create"

// CreateElicitationRequest asks the client to collect something from a person.
//
// The schema makes it a union on `mode` with two variants and two scopes, and
// this is all four flattened, which is the same choice the permission outcome
// made: every variant is a flat object, so one struct reads them all and the
// discriminator says which fields mean anything.
type CreateElicitationRequest struct {
	Message string `json:"message"`
	Mode    string `json:"mode,omitempty"`

	// Form mode.
	RequestedSchema *ElicitationSchema `json:"requestedSchema,omitempty"`

	// URL mode.
	ElicitationID string `json:"elicitationId,omitempty"`
	URL           string `json:"url,omitempty"`

	// The scope: a session, optionally a tool call within it, or a request
	// outside any session — which is where an agent asks something during
	// authentication, before a session exists.
	SessionID  string          `json:"sessionId,omitempty"`
	ToolCallID string          `json:"toolCallId,omitempty"`
	RequestID  json.RawMessage `json:"requestId,omitempty"`
}

// The elicitation modes.
const (
	ElicitForm = "form"
	ElicitURL  = "url"
)

// ElicitationSchema describes the form to put in front of a person. It is a
// JSON Schema object whose properties are primitives, which the protocol
// requires: a form is fields, not a document.
type ElicitationSchema struct {
	Type        string                         `json:"type"`
	Title       string                         `json:"title,omitempty"`
	Description string                         `json:"description,omitempty"`
	Properties  map[string]ElicitationProperty `json:"properties"`
	Required    []string                       `json:"required,omitempty"`
}

// ElicitationProperty is one field.
//
// Enum is the single-select case, which the schema spells as a string property
// carrying `enum`; the multi-select case is an array property and is not
// served, because a terminal line is a poor multi-select and offering a bad
// one is worse than saying so.
type ElicitationProperty struct {
	Type        string   `json:"type"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// The property types a form may carry.
const (
	PropertyString  = "string"
	PropertyNumber  = "number"
	PropertyInteger = "integer"
	PropertyBoolean = "boolean"
)

// CreateElicitationResponse is what the person did.
type CreateElicitationResponse struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

// What a person may do with an elicitation. Anything that is not accept is an
// answer too — the agent is owed one either way.
const (
	ElicitAccept  = "accept"
	ElicitDecline = "decline"
	ElicitCancel  = "cancel"
)

// ElicitationCapabilities is which elicitation modes the client can serve.
//
// Both are pointers to an empty object because that is the schema's own way of
// saying it: supplying `{}` advertises the mode and omitting it does not, so a
// bool here would encode "not supported" as a claim rather than as silence.
type ElicitationCapabilities struct {
	Form *ElicitationMode `json:"form,omitempty"`
	URL  *ElicitationMode `json:"url,omitempty"`
}

// ElicitationMode is the empty object that advertises one mode.
type ElicitationMode struct{}

// elicit puts a form to whoever answers for the person.
func (c *Client) elicit(ctx context.Context, params json.RawMessage) (any, error) {
	if c.Elicit == nil {
		return nil, Errorf(CodeMethodNotFound, "this client does not serve %s", MethodCreateElicitation)
	}
	var req CreateElicitationRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, Errorf(CodeInvalidParams, "%s: %v", MethodCreateElicitation, err)
	}
	if req.Mode != ElicitForm {
		// Only the form mode is advertised, so only the form mode is answered.
		// A client that served a mode it never claimed would be telling the
		// agent something untrue about what it can rely on — the same rule the
		// file and terminal capabilities are held to.
		return nil, Errorf(CodeInvalidParams,
			"this client serves only %q elicitation, not %q", ElicitForm, req.Mode)
	}
	out, err := c.Elicit(ctx, req)
	if err != nil {
		return nil, err
	}
	return out, nil
}
