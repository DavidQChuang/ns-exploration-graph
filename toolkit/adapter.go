package toolkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"ns-exploration-graph/graph"
)

type Service interface {
	ResultLimit() int
	ProjectContext(context.Context, string, string, graph.ProjectionOptions) (graph.Projection, error)
	GetNode(context.Context, string, string, graph.GetNodeOptions) (graph.NodeView, error)
	ListNodes(context.Context, string, string, graph.ListOptions) (graph.NodePage, error)
	ProposeWork(context.Context, graph.CommandContext, graph.ProposeWorkInput) (graph.Result, error)
	RecordObservation(context.Context, graph.CommandContext, graph.RecordObservationInput) (graph.Result, error)
	RecordConclusion(context.Context, graph.CommandContext, graph.RecordConclusionInput) (graph.Result, error)
	AddSupport(context.Context, graph.CommandContext, graph.AddSupportInput) (graph.Result, error)
	AcceptGoal(context.Context, graph.CommandContext, graph.AcceptGoalInput) (graph.Result, error)
}

// Binding is supplied exclusively by the authenticated host, never tool arguments.
// Keep the same binding and invocation identity when reconciling a committed retry.
type Binding struct {
	Namespace        string
	GraphID          string
	ActorID          string
	Role             string
	Execution        *graph.Execution
	AcceptGoal       bool
	AcceptanceMethod string
}
type Limits struct {
	InputBytes    int
	ResultBytes   int
	OverviewBytes int
}

func DefaultLimits() Limits { return Limits{256 * 1024, 64 * 1024, 16 * 1024} }

type Authorizer func(context.Context, Binding, string) error
type AcceptancePolicy func(context.Context, Binding, graph.AcceptGoalInput) error
type Config struct {
	Binding          Binding
	Limits           Limits
	Authorize        Authorizer
	AcceptancePolicy AcceptancePolicy
}
type Adapter struct {
	service Service
	config  Config
}
type Envelope struct {
	OK            bool         `json:"ok"`
	GraphRevision int64        `json:"graph_revision,omitempty"`
	Data          any          `json:"data,omitempty"`
	Error         *graph.Error `json:"error,omitempty"`
}

// Failure builds the stable error envelope. Anything that is not a coded graph
// error, and every storage_unavailable, is replaced by a fixed safe message so
// driver or SQL text never reaches the model.
func Failure(err error) Envelope {
	var e *graph.Error
	if !errors.As(err, &e) || e.Code == "storage_unavailable" {
		e = graph.Err("storage_unavailable", "Graph service is unavailable.")
	}
	return Envelope{Error: e}
}
func New(s Service, c Config) (*Adapter, error) {
	if e := load(); e != nil {
		return nil, e
	}
	b := c.Binding
	if s == nil || !graph.ValidID(b.GraphID) || b.Namespace == "" || b.ActorID == "" {
		return nil, fmt.Errorf("service and trusted graph/namespace/actor required")
	}
	if _, ok := contract.Roles[b.Role]; !ok {
		return nil, fmt.Errorf("unknown role")
	}
	if b.AcceptGoal && (c.AcceptancePolicy == nil || (b.AcceptanceMethod != "agent" && b.AcceptanceMethod != "human" && b.AcceptanceMethod != "validator")) {
		return nil, fmt.Errorf("acceptance grant requires method and policy")
	}
	if c.Limits == (Limits{}) {
		c.Limits = DefaultLimits()
	}
	if c.Limits.InputBytes < 1 || c.Limits.ResultBytes < 1024 || c.Limits.ResultBytes > 65536 || c.Limits.OverviewBytes < 1024 || c.Limits.OverviewBytes > c.Limits.ResultBytes || c.Limits.ResultBytes != s.ResultLimit() {
		return nil, fmt.Errorf("invalid tool limits: mutation output limit must match service")
	}
	if b.Execution != nil {
		if !graph.ValidID(b.Execution.WorkItemID) || !graph.ValidID(b.Execution.ExecutionID) {
			return nil, fmt.Errorf("invalid execution binding")
		}
		x := *b.Execution
		c.Binding.Execution = &x
	}
	return &Adapter{s, c}, nil
}
func (a *Adapter) Binding() Binding {
	b := a.config.Binding
	if b.Execution != nil {
		x := *b.Execution
		b.Execution = &x
	}
	return b
}
func (a *Adapter) Limits() Limits { return a.config.Limits }
func (a *Adapter) permitted(name string) bool {
	b := a.config.Binding
	if name == "graph_accept_goal" {
		return b.AcceptGoal
	}
	for _, n := range contract.Roles[b.Role] {
		if n == name {
			return true
		}
	}
	return false
}
func (a *Adapter) Definitions() []Definition {
	ds, _ := Definitions()
	out := []Definition{}
	for _, d := range ds {
		if a.permitted(d.Name) {
			out = append(out, d)
		}
	}
	return out
}
func IsMutation(name string) bool {
	return name != "graph_overview" && name != "graph_get_node" && name != "graph_list_nodes"
}

type modelRef struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
}

func (r modelRef) core() graph.Ref { return graph.Ref{NodeID: r.ID, Revision: r.Revision} }
func refs(rs []modelRef) []graph.Ref {
	out := make([]graph.Ref, len(rs))
	for i, r := range rs {
		out[i] = r.core()
	}
	return out
}

// Invoke must receive a host-persisted identity for writes. A fresh identity means
// a fresh operation, even for identical arguments. Never use a reconnecting RPC ID.
func (a *Adapter) Invoke(ctx context.Context, name string, args json.RawMessage, invocation string) Envelope {
	if !a.permitted(name) {
		return Failure(graph.Err("permission_denied", "Tool is not granted to this session."))
	}
	b := a.Binding()
	if a.config.Authorize != nil {
		if e := a.config.Authorize(ctx, b, name); e != nil {
			return Failure(graph.Err("permission_denied", "Host authorization denied the operation."))
		}
	}
	if len(args) > a.config.Limits.InputBytes {
		return Failure(graph.Err("limit_exceeded", "Tool input exceeds configured limit."))
	}
	if e := ValidateInput(name, args); e != nil {
		return Failure(graph.Err("invalid_argument", "Arguments do not match the tool schema."))
	}
	var result graph.Result
	var err error
	c := graph.CommandContext{Namespace: b.Namespace, GraphID: b.GraphID, ActorID: b.ActorID, Execution: b.Execution}
	if IsMutation(name) {
		if invocation == "" || !utf8.ValidString(invocation) || utf8.RuneCountInString(invocation) > 200 {
			return Failure(graph.Err("invalid_argument", "Host invocation identity is required."))
		}
		h := sha256.Sum256([]byte(b.GraphID + "\x00" + b.ActorID + "\x00" + invocation))
		c.RequestID = "tool:" + hex.EncodeToString(h[:])
	}
	switch name {
	case "graph_overview":
		var p struct {
			ByteBudget int `json:"byte_budget"`
		}
		_ = json.Unmarshal(args, &p)
		if p.ByteBudget == 0 {
			p.ByteBudget = a.config.Limits.OverviewBytes
		}
		if p.ByteBudget > a.config.Limits.ResultBytes {
			return Failure(graph.Err("limit_exceeded", "Overview exceeds configured result limit."))
		}
		o := graph.ProjectionOptions{ByteBudget: p.ByteBudget}
		if b.Execution != nil {
			o.AssignedWorkItemID = b.Execution.WorkItemID
		}
		v, e := a.service.ProjectContext(ctx, b.Namespace, b.GraphID, o)
		err = e
		result = readResult(v)
	case "graph_get_node":
		var p graph.GetNodeOptions
		_ = json.Unmarshal(args, &p)
		p.ByteBudget = a.config.Limits.ResultBytes
		v, e := a.service.GetNode(ctx, b.Namespace, b.GraphID, p)
		err = e
		result = readResult(v)
	case "graph_list_nodes":
		var p graph.ListOptions
		_ = json.Unmarshal(args, &p)
		v, e := a.service.ListNodes(ctx, b.Namespace, b.GraphID, p)
		err = e
		result = readResult(v)
	case "graph_propose_work":
		var p struct {
			Title       string     `json:"title"`
			Instruction string     `json:"instruction"`
			Priority    int        `json:"priority"`
			Parents     []modelRef `json:"parents"`
		}
		_ = json.Unmarshal(args, &p)
		result, err = a.service.ProposeWork(ctx, c, graph.ProposeWorkInput{Title: p.Title, WorkPayload: graph.WorkPayload{Instruction: p.Instruction, Priority: p.Priority}, Parents: refs(p.Parents)})
	case "graph_record_observation":
		if b.Execution == nil {
			return Failure(graph.Err("permission_denied", "Observation requires a host-bound execution."))
		}
		var p graph.RecordObservationInput
		_ = json.Unmarshal(args, &p)
		result, err = a.service.RecordObservation(ctx, c, p)
	case "graph_record_conclusion":
		if b.Role == "worker" && b.Execution == nil {
			return Failure(graph.Err("permission_denied", "Worker conclusion requires a host-bound execution."))
		}
		var p struct {
			Title string `json:"title"`
			graph.EvidencePayload
			Supports  []modelRef            `json:"supports"`
			Resources []graph.ResourceInput `json:"resource_refs"`
		}
		_ = json.Unmarshal(args, &p)
		result, err = a.service.RecordConclusion(ctx, c, graph.RecordConclusionInput{Title: p.Title, EvidencePayload: p.EvidencePayload, Supports: refs(p.Supports), Resources: p.Resources})
	case "graph_add_support":
		var p struct {
			Source modelRef `json:"source"`
			Target modelRef `json:"target"`
		}
		_ = json.Unmarshal(args, &p)
		result, err = a.service.AddSupport(ctx, c, graph.AddSupportInput{Source: p.Source.core(), Target: p.Target.core()})
	case "graph_accept_goal":
		var p struct {
			Goal           modelRef `json:"goal"`
			SupportEdgeIDs []string `json:"support_edge_ids"`
			Rationale      string   `json:"rationale"`
		}
		_ = json.Unmarshal(args, &p)
		in := graph.AcceptGoalInput{Goal: p.Goal.core(), SupportEdgeIDs: p.SupportEdgeIDs, Rationale: p.Rationale, Method: b.AcceptanceMethod}
		if e := a.config.AcceptancePolicy(ctx, b, in); e != nil {
			return Failure(graph.Err("permission_denied", "Acceptance policy denied this evidence request."))
		}
		result, err = a.service.AcceptGoal(ctx, c, in)
	}
	if err != nil {
		return Failure(err)
	}
	if name == "graph_accept_goal" { // Normalize host refs to the language-neutral tool contract.
		raw, _ := json.Marshal(result.Data["reviewed_evidence"])
		var rs []graph.Ref
		_ = json.Unmarshal(raw, &rs)
		ms := make([]modelRef, len(rs))
		for i, r := range rs {
			ms[i] = modelRef{r.NodeID, r.Revision}
		}
		data := map[string]any{}
		for k, v := range result.Data {
			data[k] = v
		}
		data["reviewed_evidence"] = ms
		result.Data = data
	}
	out := Envelope{OK: true, GraphRevision: result.GraphRevision, Data: result.Data}
	encoded, e := json.Marshal(out)
	if e != nil || ValidateOutput(name, encoded) != nil {
		return Failure(graph.Err("storage_unavailable", "Graph service returned an invalid result."))
	}
	if len(encoded) > a.config.Limits.ResultBytes {
		return Failure(graph.Err("limit_exceeded", "Tool result exceeds configured limit."))
	}
	return out
}
func readResult(v any) graph.Result {
	b, _ := json.Marshal(v)
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(b, &raw)
	var r int64
	_ = json.Unmarshal(raw["graph_revision"], &r)
	delete(raw, "graph_revision")
	m := map[string]any{}
	for k, v := range raw {
		m[k] = v
	}
	return graph.Result{GraphRevision: r, Data: m}
}
