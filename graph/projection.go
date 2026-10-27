package graph

import (
	"context"
	"encoding/json"
	"sort"
)

type ProjectionOptions struct {
	ByteBudget         int              `json:"byte_budget,omitempty"`
	AssignedWorkItemID string           `json:"assigned_work_item_id,omitempty"`
	TokenBudget        int              `json:"-"`
	CountTokens        func([]byte) int `json:"-"`
}
type ContextNode struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Revision  int             `json:"revision"`
	State     string          `json:"state"`
	Title     string          `json:"title"`
	Payload   json.RawMessage `json:"payload"`
	Freshness string          `json:"freshness"`
}
type RetrievalRoutes struct {
	GetNode   string `json:"get_node"`
	ListNodes string `json:"list_nodes"`
	Snapshot  string `json:"snapshot"`
}
type Projection struct {
	GraphRevision int64           `json:"graph_revision"`
	Origin        ContextNode     `json:"origin"`
	Goals         []ContextNode   `json:"goals"`
	Work          []ContextNode   `json:"work"`
	Evidence      []ContextNode   `json:"evidence"`
	Hints         []ContextNode   `json:"hints"`
	Edges         []Edge          `json:"edges"`
	Totals        map[string]int  `json:"totals"`
	Omitted       map[string]int  `json:"omitted"`
	Truncated     bool            `json:"truncated"`
	Retrieval     RetrievalRoutes `json:"retrieval"`
}

const (
	listWork = iota
	listEvidence
	listHints
	listEdges
)

// projectionItem is one optional record with its serialized size computed once.
type projectionItem struct {
	list  int
	kind  string
	node  ContextNode
	edge  Edge
	bytes int
}

func (it projectionItem) appendTo(p *Projection) {
	switch it.list {
	case listWork:
		p.Work = append(p.Work, it.node)
	case listEvidence:
		p.Evidence = append(p.Evidence, it.node)
	case listHints:
		p.Hints = append(p.Hints, it.node)
	default:
		p.Edges = append(p.Edges, it.edge)
	}
}
func projectionCounts(p *Projection) map[string]int {
	counts := map[string]int{"origin": 1, "goal": len(p.Goals), "work_item": len(p.Work), "hint": len(p.Hints), "edges": len(p.Edges)}
	for _, n := range p.Evidence {
		counts[n.Kind]++
	}
	return counts
}
func updateOmitted(p *Projection) {
	counts := projectionCounts(p)
	p.Omitted = map[string]int{}
	p.Truncated = false
	for k, v := range p.Totals {
		p.Omitted[k] = v - counts[k]
		if p.Omitted[k] > 0 {
			p.Truncated = true
		}
	}
}

// projectionSizer predicts envelopeBytes of a projection whose mandatory content is
// fixed, so that each candidate costs its own encoding instead of a full re-encoding.
// Arrays add element bytes plus one comma per additional element; omitted counts and
// the truncation flag are re-encoded because their digits change.
type projectionSizer struct {
	fixed  int
	totals map[string]int
	counts map[string]int
	lists  [4]struct{ n, bytes int }
}

func newProjectionSizer(p Projection) *projectionSizer {
	z := &projectionSizer{totals: p.Totals, counts: projectionCounts(&p)}
	for _, n := range p.Work {
		z.lists[listWork].n++
		z.lists[listWork].bytes += len(raw(n))
	}
	p.Work, p.Evidence, p.Hints, p.Edges = []ContextNode{}, []ContextNode{}, []ContextNode{}, []Edge{}
	p.Omitted, p.Truncated = map[string]int{}, false
	z.fixed = envelopeBytes(p) - len(`{}`) - len(`false`)
	return z
}
func (z *projectionSizer) size(extra *projectionItem) int {
	omitted := make(map[string]int, len(z.totals))
	truncated := false
	for k, v := range z.totals {
		c := z.counts[k]
		if extra != nil && extra.kind == k {
			c++
		}
		omitted[k] = v - c
		if omitted[k] > 0 {
			truncated = true
		}
	}
	total := z.fixed + len(raw(omitted)) + len(raw(truncated))
	for i, l := range z.lists {
		n, b := l.n, l.bytes
		if extra != nil && extra.list == i {
			n++
			b += extra.bytes
		}
		if n > 1 {
			b += n - 1
		}
		total += b
	}
	return total
}
func (z *projectionSizer) accept(it *projectionItem) {
	z.counts[it.kind]++
	z.lists[it.list].n++
	z.lists[it.list].bytes += it.bytes
}

func (s *Service) ProjectContext(ctx context.Context, ns, id string, o ProjectionOptions) (Projection, error) {
	return s.projectContext(ctx, ns, id, o, false)
}
func (s *Service) projectContext(ctx context.Context, ns, id string, o ProjectionOptions, exact bool) (Projection, error) {
	budget := o.ByteBudget
	if budget == 0 {
		budget = 16 * 1024
	}
	if budget < 1024 || budget > s.Limits.ResultBytes {
		return Projection{}, Err("limit_exceeded", "Overview budget must be between 1024 and the configured maximum.")
	}
	st, e := s.read(ctx, ns, id, ReadScope{})
	if e != nil {
		return Projection{}, e
	}
	out, predicted, e := project(st, o, budget, exact)
	if e == nil && !exact && envelopeBytes(out) != predicted {
		out, _, e = project(st, o, budget, true)
	}
	return out, e
}

// project selects records greedily in the specified order. Unless exact is set, each
// candidate is measured incrementally; exact measures every candidate by encoding the
// complete candidate projection. The predicted final size is returned for the check.
func project(st *State, o ProjectionOptions, budget int, exact bool) (Projection, int, error) {
	ix := newIndex(st)
	out := Projection{GraphRevision: st.Graph.Revision, Goals: []ContextNode{}, Work: []ContextNode{}, Evidence: []ContextNode{}, Hints: []ContextNode{}, Edges: []Edge{}, Totals: map[string]int{"origin": 0, "goal": 0, "work_item": 0, "observation": 0, "conclusion": 0, "hint": 0, "edges": len(st.Edges)}, Omitted: map[string]int{}, Retrieval: RetrievalRoutes{"graph_get_node", "graph_list_nodes", "host_snapshot"}}
	included := map[string]bool{}
	candidates := []*Node{}
	for i := range st.Nodes {
		n := &st.Nodes[i]
		out.Totals[n.Kind]++
		r := n.Current()
		switch {
		case n.Kind == "origin":
			out.Origin = ix.contextNode(n)
			included[n.ID] = true
		case n.Kind == "goal" && r.State != "abandoned":
			out.Goals = append(out.Goals, ix.contextNode(n))
			included[n.ID] = true
		case n.ID == o.AssignedWorkItemID:
			if n.Kind != "work_item" {
				return Projection{}, 0, Err("invalid_argument", "Assigned item must be work.")
			}
			out.Work = append(out.Work, ix.contextNode(n))
			included[n.ID] = true
		default:
			if (n.Kind == "hint" && r.State == "active") || (n.Kind == "work_item" && (r.State == "running" || ix.eligible(n))) || ((n.Kind == "observation" || n.Kind == "conclusion") && r.State == "active") {
				candidates = append(candidates, n)
			}
		}
	}
	if o.AssignedWorkItemID != "" && !included[o.AssignedWorkItemID] {
		return Projection{}, 0, Err("not_found", "Assigned work not found.")
	}
	sort.Slice(out.Goals, func(i, j int) bool { return out.Goals[i].ID < out.Goals[j].ID })
	tokensFit := func(p Projection) bool {
		return o.CountTokens == nil || o.TokenBudget <= 0 || o.CountTokens(raw(p)) <= o.TokenBudget
	}
	updateOmitted(&out)
	if envelopeBytes(out) > budget || !tokensFit(out) {
		e := Err("budget_too_small", "Mandatory context anchors exceed the budget.")
		e.Details = map[string]any{"required_bytes": envelopeBytes(out)}
		return Projection{}, 0, e
	}
	category := func(n *Node) int {
		if n.Kind == "hint" {
			return 0
		}
		if n.Kind == "work_item" {
			if n.Current().State == "running" {
				return 1
			}
			return 2
		}
		return 3
	}
	priority := make(map[string]int, len(candidates))
	for _, n := range candidates {
		if category(n) == 2 {
			var w WorkPayload
			_ = json.Unmarshal(n.Current().Payload, &w)
			priority[n.ID] = w.Priority
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		ca, cb := category(a), category(b)
		if ca != cb {
			return ca < cb
		}
		if ca == 2 && priority[a.ID] != priority[b.ID] {
			return priority[a.ID] > priority[b.ID]
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			if ca == 3 {
				return a.CreatedAt.After(b.CreatedAt)
			}
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	})
	sizer := newProjectionSizer(out)
	consider := func(it projectionItem) bool {
		candidate := out
		if exact {
			it.appendTo(&candidate)
			updateOmitted(&candidate)
			if envelopeBytes(candidate) > budget || !tokensFit(candidate) {
				return false
			}
		} else {
			if sizer.size(&it) > budget {
				return false
			}
			if o.CountTokens != nil && o.TokenBudget > 0 {
				candidate.Work, candidate.Evidence, candidate.Hints, candidate.Edges = out.Work[:len(out.Work):len(out.Work)], out.Evidence[:len(out.Evidence):len(out.Evidence)], out.Hints[:len(out.Hints):len(out.Hints)], out.Edges[:len(out.Edges):len(out.Edges)]
				it.appendTo(&candidate)
				updateOmitted(&candidate)
				if !tokensFit(candidate) {
					return false
				}
			}
			sizer.accept(&it)
		}
		it.appendTo(&out)
		return true
	}
	for _, n := range candidates {
		it := projectionItem{list: listEvidence, kind: n.Kind, node: ix.contextNode(n)}
		switch n.Kind {
		case "hint":
			it.list = listHints
		case "work_item":
			it.list = listWork
		}
		if !exact {
			it.bytes = len(raw(it.node))
		}
		if consider(it) {
			included[n.ID] = true
		}
	}
	eds := make([]*Edge, 0, len(st.Edges))
	for i := range st.Edges {
		if included[st.Edges[i].SourceID] && included[st.Edges[i].TargetID] {
			eds = append(eds, &st.Edges[i])
		}
	}
	sort.Slice(eds, func(i, j int) bool { return eds[i].ID < eds[j].ID })
	for _, ed := range eds {
		it := projectionItem{list: listEdges, kind: "edges", edge: *ed}
		if !exact {
			it.bytes = len(raw(it.edge))
		}
		consider(it)
	}
	updateOmitted(&out)
	return out, sizer.size(nil), nil
}
