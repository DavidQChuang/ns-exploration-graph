package graph

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
)

type ListOptions struct {
	Kind   string `json:"kind,omitempty"`
	State  string `json:"state,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}
type NodeSummary struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Revision  int    `json:"revision"`
	State     string `json:"state"`
	Title     string `json:"title"`
	Freshness string `json:"freshness"`
}
type NodePage struct {
	GraphRevision int64         `json:"graph_revision"`
	Nodes         []NodeSummary `json:"nodes"`
	Total         int           `json:"total"`
	Omitted       int           `json:"omitted"`
	NextCursor    string        `json:"next_cursor,omitempty"`
}
type GetNodeOptions struct {
	NodeID        string `json:"node_id"`
	Revision      int    `json:"revision,omitempty"`
	RelatedCursor string `json:"related_cursor,omitempty"`
	ByteBudget    int    `json:"byte_budget,omitempty"`
}
type NodeView struct {
	GraphRevision     int64         `json:"graph_revision"`
	ID                string        `json:"id"`
	Kind              string        `json:"kind"`
	CurrentRevision   int           `json:"current_revision"`
	Revision          Revision      `json:"node"`
	Freshness         string        `json:"freshness"`
	Edges             []Edge        `json:"edges"`
	ResourceRefs      []ResourceRef `json:"resource_refs"`
	TotalRelated      int           `json:"total_related"`
	OmittedRelated    int           `json:"omitted_related"`
	NextRelatedCursor string        `json:"next_related_cursor,omitempty"`
}
type cursor struct {
	GraphID  string `json:"graph_id"`
	Revision int64  `json:"revision"`
	Query    string `json:"query"`
	Position int    `json:"position"`
}

func queryHash(v any) string { return fmt.Sprintf("%x", sha256.Sum256(raw(v))) }
func encodeCursor(g Graph, q string, pos int) string {
	return base64.RawURLEncoding.EncodeToString(raw(cursor{g.ID, g.Revision, q, pos}))
}
func cursorPosition(encoded string, g Graph, q string) (int, error) {
	if encoded == "" {
		return 0, nil
	}
	b, e := base64.RawURLEncoding.DecodeString(encoded)
	if e != nil {
		return 0, Err("invalid_argument", "Invalid cursor.")
	}
	var c cursor
	if e = strict(b, &c); e != nil {
		return 0, e
	}
	if c.GraphID != g.ID || c.Query != q {
		return 0, Err("invalid_argument", "Cursor query does not match.")
	}
	if c.Revision != g.Revision {
		return 0, Err("stale_cursor", "Graph changed between pages.")
	}
	if c.Position < 0 {
		return 0, Err("invalid_argument", "Invalid cursor position.")
	}
	return c.Position, nil
}

// read loads only what ordinary reads consult when the store supports scoping; a full
// Read is a superset and yields identical results.
func (s *Service) read(ctx context.Context, ns, id string, scope ReadScope) (*State, error) {
	if scoped, ok := s.store.(ScopedReader); ok {
		return scoped.ReadScoped(ctx, ns, id, scope)
	}
	return s.store.Read(ctx, ns, id)
}
func envelopeBytes(v any) int {
	b, _ := json.Marshal(v)
	var data map[string]json.RawMessage
	_ = json.Unmarshal(b, &data)
	rev := data["graph_revision"]
	delete(data, "graph_revision")
	b, _ = json.Marshal(struct {
		OK            bool            `json:"ok"`
		GraphRevision json.RawMessage `json:"graph_revision"`
		Data          any             `json:"data"`
	}{true, rev, data})
	return len(b)
}
func limitValue(v, def, max int) (int, error) {
	if v == 0 {
		return def, nil
	}
	if v < 1 || v > max {
		return 0, Err("limit_exceeded", "Limit is outside supported bounds.")
	}
	return v, nil
}
func validState(kind, state string) bool {
	states := map[string][]string{"origin": {"recorded"}, "goal": {"open", "met", "abandoned", "needs_review"}, "work_item": {"open", "running", "paused", "done", "blocked", "exhausted", "stopped"}, "observation": {"active", "retracted"}, "conclusion": {"active", "retracted"}, "hint": {"active", "consumed"}}
	if kind != "" {
		xs, ok := states[kind]
		if !ok {
			return false
		}
		if state == "" {
			return true
		}
		for _, x := range xs {
			if x == state {
				return true
			}
		}
		return false
	}
	if state == "" {
		return true
	}
	for _, xs := range states {
		for _, x := range xs {
			if x == state {
				return true
			}
		}
	}
	return false
}
func (s *Service) ListNodes(ctx context.Context, ns, id string, o ListOptions) (NodePage, error) {
	var out NodePage
	limit, e := limitValue(o.Limit, 20, 100)
	if e != nil {
		return out, e
	}
	if !validState(o.Kind, o.State) {
		return out, Err("invalid_argument", "Incompatible node kind/state filter.")
	}
	st, e := s.read(ctx, ns, id, ReadScope{})
	if e != nil {
		return out, e
	}
	ix := newIndex(st)
	q := queryHash(struct{ Kind, State string }{o.Kind, o.State})
	pos, e := cursorPosition(o.Cursor, st.Graph, q)
	if e != nil {
		return out, e
	}
	nodes := []*Node{}
	for i := range st.Nodes {
		n := &st.Nodes[i]
		if (o.Kind == "" || n.Kind == o.Kind) && (o.State == "" || n.Current().State == o.State) {
			nodes = append(nodes, n)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	if pos > len(nodes) {
		return out, Err("invalid_argument", "Cursor position exceeds result count.")
	}
	out = NodePage{GraphRevision: st.Graph.Revision, Nodes: []NodeSummary{}, Total: len(nodes)}
	end := pos + limit
	if end > len(nodes) {
		end = len(nodes)
	}
	for _, n := range nodes[pos:end] {
		out.Nodes = append(out.Nodes, ix.summary(n))
	}
	out.Omitted = len(nodes) - (end - pos)
	if end < len(nodes) {
		out.NextCursor = encodeCursor(st.Graph, q, end)
	}
	if envelopeBytes(out) > s.Limits.ResultBytes {
		return NodePage{}, Err("limit_exceeded", "List result exceeds configured output limit.")
	}
	return out, nil
}
func (s *Service) GetNode(ctx context.Context, ns, id string, o GetNodeOptions) (NodeView, error) {
	var out NodeView
	budget := o.ByteBudget
	if budget == 0 {
		budget = s.Limits.ResultBytes
	}
	if budget < 1 || budget > s.Limits.ResultBytes {
		return out, Err("limit_exceeded", "Invalid result budget.")
	}
	st, e := s.read(ctx, ns, id, ReadScope{DetailNodeID: o.NodeID})
	if e != nil {
		return out, e
	}
	ix := newIndex(st)
	n, e := ix.node(o.NodeID)
	if e != nil {
		return out, e
	}
	rev := o.Revision
	if rev == 0 {
		rev = n.CurrentRevision
	}
	if rev < 1 || rev > n.CurrentRevision {
		return out, Err("not_found", "Node revision not found.")
	}
	q := queryHash(struct {
		NodeID   string
		Revision int
	}{n.ID, rev})
	pos, e := cursorPosition(o.RelatedCursor, st.Graph, q)
	if e != nil {
		return out, e
	}
	out = NodeView{GraphRevision: st.Graph.Revision, ID: n.ID, Kind: n.Kind, CurrentRevision: n.CurrentRevision, Revision: n.Revisions[rev-1], Freshness: ix.freshness(n), Edges: []Edge{}, ResourceRefs: []ResourceRef{}}
	if rev != n.CurrentRevision && (n.Kind == "observation" || n.Kind == "conclusion") {
		out.Freshness = "stale"
	}
	type item struct {
		id string
		ed *Edge
		rs *ResourceRef
	}
	items := []item{}
	related := func(ed *Edge) bool {
		return (ed.SourceID == n.ID && (ed.SourceRevision == rev || ed.Relation == "depends_on")) || (ed.TargetID == n.ID && (ed.TargetRevision == rev || ed.Relation == "depends_on"))
	}
	for _, ed := range ix.out[n.ID] {
		if related(ed) {
			items = append(items, item{"e" + ed.ID, ed, nil})
		}
	}
	for _, ed := range ix.in[n.ID] {
		if ed.SourceID != n.ID && related(ed) {
			items = append(items, item{"e" + ed.ID, ed, nil})
		}
	}
	for i := range st.ResourceRefs {
		r := &st.ResourceRefs[i]
		if r.NodeID == n.ID && r.NodeRevision == rev {
			items = append(items, item{"r" + r.ID, nil, r})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].id < items[j].id })
	if pos > len(items) {
		return out, Err("invalid_argument", "Cursor position exceeds result count.")
	}
	out.TotalRelated = len(items)
	out.OmittedRelated = len(items)
	if pos < len(items) {
		out.NextRelatedCursor = encodeCursor(st.Graph, q, pos)
	}
	if envelopeBytes(out) > budget {
		return NodeView{}, Err("limit_exceeded", "Node payload exceeds result budget.")
	}
	for i := pos; i < len(items); i++ {
		candidate := out
		candidate.Edges = append([]Edge{}, out.Edges...)
		candidate.ResourceRefs = append([]ResourceRef{}, out.ResourceRefs...)
		if items[i].ed != nil {
			candidate.Edges = append(candidate.Edges, *items[i].ed)
		} else {
			candidate.ResourceRefs = append(candidate.ResourceRefs, *items[i].rs)
		}
		candidate.OmittedRelated = len(items) - (i - pos + 1)
		candidate.NextRelatedCursor = ""
		if i+1 < len(items) {
			candidate.NextRelatedCursor = encodeCursor(st.Graph, q, i+1)
		}
		if envelopeBytes(candidate) > budget {
			break
		}
		out = candidate
	}
	return out, nil
}
func (s *Service) GetRevision(ctx context.Context, ns, id string, r Ref) (NodeView, error) {
	return s.GetNode(ctx, ns, id, GetNodeOptions{NodeID: r.NodeID, Revision: r.Revision})
}
func (s *Service) Snapshot(ctx context.Context, ns, id string) (Snapshot, error) {
	st, e := s.store.Read(ctx, ns, id)
	if e != nil {
		return Snapshot{}, e
	}
	if len(raw(st.Snapshot)) > s.Limits.SnapshotBytes {
		return Snapshot{}, Err("limit_exceeded", "Snapshot exceeds configured size limit.")
	}
	return st.Snapshot, nil
}

type EventsPage struct {
	GraphRevision int64   `json:"graph_revision"`
	Events        []Event `json:"events"`
	NextRevision  int64   `json:"next_revision"`
}

func (s *Service) EventsAfter(ctx context.Context, ns, id string, after int64, limit int) (EventsPage, error) {
	var out EventsPage
	l, e := limitValue(limit, 100, 1000)
	if e != nil {
		return out, e
	}
	if after < 0 {
		return out, Err("invalid_argument", "Revision cannot be negative.")
	}
	store, ok := s.store.(EventStore)
	if !ok {
		return out, Err("storage_unavailable", "Store does not support event queries.")
	}
	events, rev, e := store.EventsAfter(ctx, ns, id, after, l)
	if e != nil {
		return out, e
	}
	next := after
	if len(events) > 0 {
		next = events[len(events)-1].Revision
	}
	return EventsPage{rev, events, next}, nil
}
func (s *Service) ListEligibleWork(ctx context.Context, ns, id string) (NodePage, error) {
	st, e := s.read(ctx, ns, id, ReadScope{})
	if e != nil {
		return NodePage{}, e
	}
	ix := newIndex(st)
	out := NodePage{GraphRevision: st.Graph.Revision, Nodes: []NodeSummary{}}
	for i := range st.Nodes {
		if ix.eligible(&st.Nodes[i]) {
			out.Nodes = append(out.Nodes, ix.summary(&st.Nodes[i]))
		}
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].ID < out.Nodes[j].ID })
	out.Total = len(out.Nodes)
	return out, nil
}

type GoalExplanation struct {
	GraphRevision int64        `json:"graph_revision"`
	Goal          Node         `json:"goal"`
	Acceptances   []Acceptance `json:"acceptances"`
	Supports      []Edge       `json:"supports"`
}

func (s *Service) ExplainGoal(ctx context.Context, ns, id, nodeID string) (GoalExplanation, error) {
	st, e := s.read(ctx, ns, id, ReadScope{DetailNodeID: nodeID})
	if e != nil {
		return GoalExplanation{}, e
	}
	n, e := findNode(st, nodeID)
	if e != nil {
		return GoalExplanation{}, e
	}
	if n.Kind != "goal" {
		return GoalExplanation{}, Err("invalid_relation", "Expected goal.")
	}
	out := GoalExplanation{st.Graph.Revision, *n, []Acceptance{}, []Edge{}}
	for _, a := range st.Acceptances {
		if a.GoalID == nodeID {
			out.Acceptances = append(out.Acceptances, a)
		}
	}
	for _, ed := range st.Edges {
		if ed.Relation == "supports" && ed.TargetID == nodeID {
			out.Supports = append(out.Supports, ed)
		}
	}
	return out, nil
}

type TraversalOptions struct {
	NodeID    string   `json:"node_id"`
	Relations []string `json:"relations"`
	Depth     int      `json:"depth,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Cursor    string   `json:"cursor,omitempty"`
}

func (s *Service) Neighbors(ctx context.Context, ns, id string, o TraversalOptions) (NodePage, error) {
	o.Depth = 1
	return s.traverse(ctx, ns, id, o, "both")
}
func (s *Service) Ancestors(ctx context.Context, ns, id string, o TraversalOptions) (NodePage, error) {
	return s.traverse(ctx, ns, id, o, "in")
}
func (s *Service) Descendants(ctx context.Context, ns, id string, o TraversalOptions) (NodePage, error) {
	return s.traverse(ctx, ns, id, o, "out")
}
func (s *Service) traverse(ctx context.Context, ns, id string, o TraversalOptions, direction string) (NodePage, error) {
	var out NodePage
	depth, e := limitValue(o.Depth, 8, 64)
	if e != nil {
		return out, e
	}
	limit, e := limitValue(o.Limit, 100, 1000)
	if e != nil {
		return out, e
	}
	if len(o.Relations) == 0 {
		return out, Err("invalid_argument", "Traversal must name relations.")
	}
	rels := map[string]bool{}
	for _, r := range o.Relations {
		if r != "spawns" && r != "derived_from" && r != "yields" && r != "supports" && r != "depends_on" {
			return out, Err("invalid_argument", "Unknown relation.")
		}
		rels[r] = true
	}
	st, e := s.read(ctx, ns, id, ReadScope{})
	if e != nil {
		return out, e
	}
	ix := newIndex(st)
	if _, e = ix.node(o.NodeID); e != nil {
		return out, e
	}
	q := queryHash(struct {
		Node, Direction string
		Depth           int
		Relations       []string
	}{o.NodeID, direction, depth, unique(o.Relations)})
	pos, e := cursorPosition(o.Cursor, st.Graph, q)
	if e != nil {
		return out, e
	}
	seen := map[string]bool{o.NodeID: true}
	front := []string{o.NodeID}
	found := []string{}
	for d := 0; d < depth && len(front) > 0; d++ {
		next := []string{}
		visit := func(target string) {
			if !seen[target] {
				seen[target] = true
				next = append(next, target)
				found = append(found, target)
			}
		}
		for _, x := range front {
			if direction != "in" {
				for _, ed := range ix.out[x] {
					if rels[ed.Relation] {
						visit(ed.TargetID)
					}
				}
			}
			if direction != "out" {
				for _, ed := range ix.in[x] {
					if rels[ed.Relation] {
						visit(ed.SourceID)
					}
				}
			}
		}
		front = next
	}
	sort.Strings(found)
	if pos > len(found) {
		return out, Err("invalid_argument", "Invalid cursor position.")
	}
	end := pos + limit
	if end > len(found) {
		end = len(found)
	}
	out = NodePage{GraphRevision: st.Graph.Revision, Nodes: []NodeSummary{}, Total: len(found), Omitted: len(found) - (end - pos)}
	for _, nid := range found[pos:end] {
		n, _ := ix.node(nid)
		out.Nodes = append(out.Nodes, ix.summary(n))
	}
	if end < len(found) {
		out.NextCursor = encodeCursor(st.Graph, q, end)
	}
	return out, nil
}
