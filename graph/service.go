package graph

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func ValidID(s string) bool     { return uuidPattern.MatchString(s) }
func nonempty(s string) bool    { return strings.TrimSpace(s) != "" }
func raw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func strict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return Err("invalid_argument", "Invalid payload shape.")
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return Err("invalid_argument", "Unexpected trailing JSON.")
	}
	return nil
}

type Limits struct {
	CommandBytes  int
	ResultBytes   int
	SnapshotBytes int
	MaxReferences int
}

func DefaultLimits() Limits { return Limits{256 * 1024, 64 * 1024, 64 * 1024 * 1024, 100} }

type Service struct {
	store  Store
	Limits Limits
}

func NewService(store Store) *Service { return &Service{store: store, Limits: DefaultLimits()} }
func (s *Service) Store() Store       { return s.store }
func validateContext(c CommandContext) error {
	if !ValidID(c.GraphID) || !nonempty(c.Namespace) || !nonempty(c.ActorID) || !nonempty(c.RequestID) || !utf8.ValidString(c.RequestID) || utf8.RuneCountInString(c.RequestID) > 200 {
		return Err("invalid_argument", "Valid graph, namespace, actor and request identity are required.")
	}
	return nil
}
func (s *Service) mutate(ctx context.Context, c CommandContext, kind string, payload any, create *Graph, fn func(*State) (map[string]any, error)) (Result, error) {
	return s.mutateWithLimit(ctx, c, kind, payload, create, s.Limits.CommandBytes, fn)
}
func (s *Service) mutateWithLimit(ctx context.Context, c CommandContext, kind string, payload any, create *Graph, maxInput int, fn func(*State) (map[string]any, error)) (Result, error) {
	var out Result
	if e := validateContext(c); e != nil {
		return out, e
	}
	encoded, e := json.Marshal(struct {
		Context CommandContext `json:"context"`
		Kind    string         `json:"kind"`
		Payload any            `json:"payload"`
	}{c, kind, payload})
	if e != nil {
		return out, Err("invalid_argument", "Input cannot be encoded.")
	}
	if len(encoded) > maxInput {
		return out, Err("limit_exceeded", "Command input is too large.")
	}
	var canonical any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&canonical); err != nil {
		return out, Err("invalid_argument", "Input cannot be canonicalized.")
	}
	encoded, _ = json.Marshal(canonical)
	sum := sha256.Sum256(encoded)
	hash := hex.EncodeToString(sum[:])
	update := s.store.Update
	if scoped, ok := s.store.(RequestUpdater); ok {
		update = func(ctx context.Context, ns, id string, create *Graph, fn func(*State) error) error {
			return scoped.UpdateRequest(ctx, ns, id, create, c.RequestID, fn)
		}
	}
	e = update(ctx, c.Namespace, c.GraphID, create, func(st *State) error {
		for _, ev := range st.Events {
			if ev.RequestID == c.RequestID {
				if ev.RequestHash != hash {
					return Err("idempotency_conflict", "Request identity was used with different input.")
				}
				out = ev.Result
				return nil
			}
		}
		if create != nil && st.Graph.Revision != 0 {
			return Err("already_exists", "Graph identity already exists.")
		}
		if c.ExpectedGraphRevision != nil && *c.ExpectedGraphRevision != st.Graph.Revision {
			return Err("revision_conflict", "Graph revision changed.")
		}
		if st.Graph.State != "active" && kind != "SetGraphState" {
			return Err("graph_read_only", "Graph does not accept content mutations.")
		}
		if c.Execution != nil {
			if _, e := checkExecution(st, c.Execution); e != nil {
				return e
			}
		}
		data, e := fn(st)
		if e != nil {
			return e
		}
		st.Graph.Revision++
		st.Graph.UpdatedAt = time.Now().UTC()
		out = Result{st.Graph.Revision, data}
		if len(raw(struct {
			OK bool `json:"ok"`
			Result
		}{true, out})) > s.Limits.ResultBytes {
			return Err("limit_exceeded", "Mutation result exceeds output limit.")
		}
		st.Events = append(st.Events, Event{Revision: st.Graph.Revision, RequestID: c.RequestID, RequestHash: hash, CommandKind: kind, ActorID: c.ActorID, Data: raw(payload), Result: out, CreatedAt: st.Graph.UpdatedAt})
		return nil
	})
	if e == nil {
		var detached Result
		if decodeErr := json.Unmarshal(raw(out), &detached); decodeErr != nil {
			return Result{}, Err("storage_unavailable", "Result could not be encoded.")
		}
		out = detached
	}
	return out, e
}
func findNode(st *State, id string) (*Node, error) {
	for i := range st.Nodes {
		if st.Nodes[i].ID == id {
			return &st.Nodes[i], nil
		}
	}
	return nil, Err("not_found", "Node not found in graph.")
}
func current(st *State, r Ref) (*Node, error) {
	n, e := findNode(st, r.NodeID)
	if e != nil {
		return nil, e
	}
	if r.Revision < 1 || n.CurrentRevision != r.Revision {
		return nil, Err("revision_conflict", "Node revision changed.")
	}
	return n, nil
}
func checkExecution(st *State, x *Execution) (*Node, error) {
	n, e := findNode(st, x.WorkItemID)
	if e != nil || n.Kind != "work_item" || n.Current().State != "running" {
		return nil, Err("stale_execution", "Worker execution is no longer current.")
	}
	var w WorkPayload
	_ = json.Unmarshal(n.Current().Payload, &w)
	if !ValidID(x.ExecutionID) || w.ExecutionID != x.ExecutionID {
		return nil, Err("stale_execution", "Worker execution is no longer current.")
	}
	return n, nil
}
func newNode(st *State, kind, state, title string, payload any, actor string) (*Node, error) {
	if !nonempty(title) {
		return nil, Err("invalid_argument", "Title is required.")
	}
	b := raw(payload)
	if e := validatePayload(kind, b); e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	st.Nodes = append(st.Nodes, Node{ID: NewID(), Kind: kind, CurrentRevision: 1, CreatedAt: now, Revisions: []Revision{{1, state, title, b, actor, now}}})
	return &st.Nodes[len(st.Nodes)-1], nil
}
func revise(n *Node, state, title string, payload json.RawMessage, actor string) {
	n.CurrentRevision++
	n.Revisions = append(n.Revisions, Revision{n.CurrentRevision, state, title, append(json.RawMessage(nil), payload...), actor, time.Now().UTC()})
}
func origin(st *State) *Node {
	for i := range st.Nodes {
		if st.Nodes[i].Kind == "origin" {
			return &st.Nodes[i]
		}
	}
	return nil
}
func nodeData(n *Node) map[string]any {
	return map[string]any{"node_id": n.ID, "node_revision": n.CurrentRevision}
}
func validatePayload(kind string, b []byte) error {
	switch kind {
	case "origin":
		var p OriginPayload
		if e := strict(b, &p); e != nil {
			return e
		}
		if !nonempty(p.Request) {
			return Err("invalid_argument", "Request is required.")
		}
	case "goal":
		var p GoalPayload
		if e := strict(b, &p); e != nil {
			return e
		}
		if len(p.AcceptanceCriteria) == 0 {
			return Err("invalid_argument", "Acceptance criteria are required.")
		}
		for _, c := range p.AcceptanceCriteria {
			if !nonempty(c) {
				return Err("invalid_argument", "Criteria must be nonempty.")
			}
		}
	case "work_item":
		var p WorkPayload
		if e := strict(b, &p); e != nil {
			return e
		}
		if !nonempty(p.Instruction) || p.Priority < 0 || p.Priority > 100 || (p.ExecutionID != "" && !ValidID(p.ExecutionID)) {
			return Err("invalid_argument", "Invalid work payload.")
		}
	case "observation", "conclusion":
		var p EvidencePayload
		if e := strict(b, &p); e != nil {
			return e
		}
		if !nonempty(p.Statement) || (p.EpistemicStatus != "observed" && p.EpistemicStatus != "inferred" && p.EpistemicStatus != "reported") || (p.Polarity != "" && p.Polarity != "positive" && p.Polarity != "negative" && p.Polarity != "neutral") {
			return Err("invalid_argument", "Invalid evidence payload.")
		}
	case "hint":
		var p HintPayload
		if e := strict(b, &p); e != nil {
			return e
		}
		if !nonempty(p.Instruction) {
			return Err("invalid_argument", "Instruction is required.")
		}
	default:
		return Err("invalid_argument", "Unknown node kind.")
	}
	return nil
}
func endpointKinds(a, b, relation string) bool {
	switch relation {
	case "spawns":
		return a == "origin" && b == "goal"
	case "derived_from":
		return (a == "origin" || a == "observation" || a == "conclusion") && b == "work_item"
	case "yields":
		return a == "work_item" && (b == "observation" || b == "conclusion")
	case "supports":
		return (a == "observation" || a == "conclusion") && (b == "conclusion" || b == "goal")
	case "depends_on":
		return a == "work_item" && b == "work_item"
	}
	return false
}
func hasPath(st *State, from, to, relation string) bool {
	seen := map[string]bool{}
	todo := []string{from}
	for len(todo) > 0 {
		x := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if x == to {
			return true
		}
		if seen[x] {
			continue
		}
		seen[x] = true
		for _, e := range st.Edges {
			if e.Relation == relation && e.SourceID == x {
				todo = append(todo, e.TargetID)
			}
		}
	}
	return false
}
func addEdge(st *State, a, b Ref, relation, actor string) (string, error) {
	an, e := findNode(st, a.NodeID)
	if e != nil {
		return "", e
	}
	bn, e := findNode(st, b.NodeID)
	if e != nil {
		return "", e
	}
	if a.NodeID == b.NodeID || !endpointKinds(an.Kind, bn.Kind, relation) {
		return "", Err("invalid_relation", "Relation endpoints are invalid.")
	}
	if relation == "depends_on" {
		a.Revision = 0
		b.Revision = 0
	}
	for _, ed := range st.Edges {
		if ed.SourceID == a.NodeID && ed.TargetID == b.NodeID && ed.Relation == relation && ed.SourceRevision == a.Revision && ed.TargetRevision == b.Revision {
			return "", Err("already_exists", "Relation already exists.")
		}
	}
	if (relation == "supports" || relation == "depends_on") && hasPath(st, b.NodeID, a.NodeID, relation) {
		return "", Err("cycle_detected", "Relation would create a cycle.")
	}
	id := NewID()
	st.Edges = append(st.Edges, Edge{id, a.NodeID, b.NodeID, a.Revision, b.Revision, relation, actor, time.Now().UTC()})
	return id, nil
}
func resourceValid(r ResourceInput) error {
	if !nonempty(r.ResourceType) || (r.SHA256 != "" && !digestPattern.MatchString(r.SHA256)) {
		return Err("invalid_argument", "Invalid resource reference.")
	}
	return validateResourceURI(r.URI)
}
func resources(st *State, n Ref, rs []ResourceInput, actor string) ([]string, error) {
	ids := []string{}
	for _, r := range rs {
		if e := resourceValid(r); e != nil {
			return nil, e
		}
		id := NewID()
		st.ResourceRefs = append(st.ResourceRefs, ResourceRef{id, n.NodeID, n.Revision, r, actor, time.Now().UTC()})
		ids = append(ids, id)
	}
	return ids, nil
}
func ref(n *Node) Ref { return Ref{n.ID, n.CurrentRevision} }

// closure validates recursive freshness and returns exactly the incoming support closure.
func closure(st *State, r Ref, seen map[string]bool) ([]string, error) {
	return newIndex(st).closure(r, seen)
}
func unique(xs []string) []string {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	out := []string{}
	for x := range m {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}
func edgeByID(st *State, id string) *Edge {
	for i := range st.Edges {
		if st.Edges[i].ID == id {
			return &st.Edges[i]
		}
	}
	return nil
}
func invalidate(st *State, id, actor, reason string) {
	var edges map[string]*Edge
	for i := range st.Acceptances {
		a := &st.Acceptances[i]
		if a.State != "current" {
			continue
		}
		if edges == nil {
			edges = make(map[string]*Edge, len(st.Edges))
			for j := range st.Edges {
				edges[st.Edges[j].ID] = &st.Edges[j]
			}
		}
		hit := a.GoalID == id
		for _, eid := range a.SupportEdgeIDs {
			ed := edges[eid]
			if ed != nil && (ed.SourceID == id || ed.TargetID == id) {
				hit = true
			}
		}
		if hit {
			a.State = "stale"
			a.StaleReason = reason
			g, _ := findNode(st, a.GoalID)
			if g.ID != id && g.Current().State == "met" {
				r := *g.Current()
				revise(g, "needs_review", r.Title, r.Payload, actor)
			}
		}
	}
}

func (s *Service) ResultLimit() int { return s.Limits.ResultBytes }
