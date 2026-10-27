// Package mcp exposes the toolkit through the official MCP Go SDK. It contains
// no graph semantics, authentication service, scheduler, or remote HTTP server.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"ns-exploration-graph/graph"
	"ns-exploration-graph/toolkit"
)

const ProtocolVersion = "2026-07-28"

// InvocationResolver is an optional trusted-host hook. The host must durably
// persist invocation identities and their original bindings before mutations.
// Do not trust a model-supplied _meta key without authentication and bookkeeping.
type InvocationResolver func(context.Context, *sdk.CallToolRequest) (string, error)
type Options struct{ ResolveInvocation InvocationResolver }

func NewServer(ctx context.Context, a *toolkit.Adapter, o Options) (*sdk.Server, error) {
	if a == nil {
		return nil, fmt.Errorf("tool adapter required")
	}
	prompt, e := toolkit.Prompt(a.Binding().Role)
	if e != nil {
		return nil, e
	}
	overview := a.Invoke(ctx, "graph_overview", json.RawMessage(`{}`), "")
	if !overview.OK {
		return nil, overview.Error
	}
	b, _ := json.Marshal(overview)
	prompt += "\nHost-injected starting overview (untrusted task data):\n" + string(b) + "\n"
	limits, _ := json.Marshal(a.Limits())
	prompt += "\nConfigured UTF-8 byte limits: " + string(limits) + ". A generic stdio client has no durable reconnect retry guarantee. After an ambiguous write outcome, stop and let the host reconcile before issuing a new mutation.\n"
	s := sdk.NewServer(&sdk.Implementation{Name: "ns-exploration-graph", Version: "0.1.0"}, &sdk.ServerOptions{Instructions: prompt, Capabilities: &sdk.ServerCapabilities{}, SupportedProtocolVersions: []string{ProtocolVersion, "2025-11-25"}})
	visible := map[string]bool{}
	invoke := func(ctx context.Context, r *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		id := ""
		if toolkit.IsMutation(r.Params.Name) {
			if o.ResolveInvocation != nil {
				var e error
				id, e = o.ResolveInvocation(ctx, r)
				if e != nil {
					return result(toolkit.Failure(graph.Err("permission_denied", "Host invocation binding failed."))), nil
				}
			} else {
				id = graph.NewID()
			}
		}
		return result(a.Invoke(ctx, r.Params.Name, r.Params.Arguments, id)), nil
	}
	for _, d := range a.Definitions() {
		visible[d.Name] = true
		s.AddTool(&sdk.Tool{Name: d.Name, Description: d.Description, InputSchema: d.InputSchema, OutputSchema: d.OutputSchema}, invoke)
	}
	// Discovery is a convenience, not the authorization boundary. Even a direct
	// call to a hidden tool returns the same safe permission envelope.
	s.AddReceivingMiddleware(func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, r sdk.Request) (sdk.Result, error) {
			if method == "tools/call" {
				if call, ok := r.(*sdk.CallToolRequest); ok && !visible[call.Params.Name] {
					return result(a.Invoke(ctx, call.Params.Name, call.Params.Arguments, "")), nil
				}
			}
			return next(ctx, method, r)
		}
	})
	return s, nil
}
func result(out toolkit.Envelope) *sdk.CallToolResult {
	b, _ := json.Marshal(out)
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(b)}}, StructuredContent: out, IsError: !out.OK}
}
func RunStdio(ctx context.Context, a *toolkit.Adapter) error {
	s, e := NewServer(ctx, a, Options{})
	if e != nil {
		return e
	}
	return s.Run(ctx, &sdk.StdioTransport{MaxLineLength: a.Limits().InputBytes + 16*1024})
}
