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

func contextNode(st *State, n *Node) ContextNode {
	r := n.Current()
	return ContextNode{n.ID, n.Kind, n.CurrentRevision, r.State, r.Title, r.Payload, freshness(st, n)}
}
func (s *Service) ProjectContext(ctx context.Context, ns, id string, o ProjectionOptions) (Projection, error) {
	var out Projection
	budget := o.ByteBudget
	if budget == 0 {
		budget = 16 * 1024
	}
	if budget < 1024 || budget > s.Limits.ResultBytes {
		return out, Err("limit_exceeded", "Overview budget must be between 1024 and the configured maximum.")
	}
	st, e := s.store.Read(ctx, ns, id)
	if e != nil {
		return out, e
	}
	out = Projection{GraphRevision: st.Graph.Revision, Goals: []ContextNode{}, Work: []ContextNode{}, Evidence: []ContextNode{}, Hints: []ContextNode{}, Edges: []Edge{}, Totals: map[string]int{"origin": 0, "goal": 0, "work_item": 0, "observation": 0, "conclusion": 0, "hint": 0, "edges": len(st.Edges)}, Omitted: map[string]int{}, Retrieval: RetrievalRoutes{"graph_get_node", "graph_list_nodes", "host_snapshot"}}
	included := map[string]bool{}
	candidates := []*Node{}
	for i := range st.Nodes {
		n := &st.Nodes[i]
		out.Totals[n.Kind]++
		r := n.Current()
		switch {
		case n.Kind == "origin":
			out.Origin = contextNode(st, n)
			included[n.ID] = true
		case n.Kind == "goal" && r.State != "abandoned":
			out.Goals = append(out.Goals, contextNode(st, n))
			included[n.ID] = true
		case n.ID == o.AssignedWorkItemID:
			if n.Kind != "work_item" {
				return Projection{}, Err("invalid_argument", "Assigned item must be work.")
			}
			out.Work = append(out.Work, contextNode(st, n))
			included[n.ID] = true
		default:
			if (n.Kind == "hint" && r.State == "active") || (n.Kind == "work_item" && (r.State == "running" || eligible(st, n))) || ((n.Kind == "observation" || n.Kind == "conclusion") && r.State == "active") {
				candidates = append(candidates, n)
			}
		}
	}
	if o.AssignedWorkItemID != "" && !included[o.AssignedWorkItemID] {
		return Projection{}, Err("not_found", "Assigned work not found.")
	}
	sort.Slice(out.Goals, func(i, j int) bool { return out.Goals[i].ID < out.Goals[j].ID })
	update := func(p *Projection) {
		counts := map[string]int{"origin": 1, "goal": len(p.Goals), "work_item": len(p.Work), "hint": len(p.Hints), "edges": len(p.Edges)}
		for _, n := range p.Evidence {
			counts[n.Kind]++
		}
		p.Truncated = false
		for k, v := range p.Totals {
			p.Omitted[k] = v - counts[k]
			if p.Omitted[k] > 0 {
				p.Truncated = true
			}
		}
	}
	fits := func(p Projection) bool {
		if envelopeBytes(p) > budget {
			return false
		}
		return o.CountTokens == nil || o.TokenBudget <= 0 || o.CountTokens(raw(p)) <= o.TokenBudget
	}
	update(&out)
	if !fits(out) {
		e := Err("budget_too_small", "Mandatory context anchors exceed the budget.")
		e.Details = map[string]any{"required_bytes": envelopeBytes(out)}
		return Projection{}, e
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
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		ca, cb := category(a), category(b)
		if ca != cb {
			return ca < cb
		}
		if ca == 2 {
			var wa, wb WorkPayload
			_ = json.Unmarshal(a.Current().Payload, &wa)
			_ = json.Unmarshal(b.Current().Payload, &wb)
			if wa.Priority != wb.Priority {
				return wa.Priority > wb.Priority
			}
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			if ca == 3 {
				return a.CreatedAt.After(b.CreatedAt)
			}
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	})
	for _, n := range candidates {
		candidate := out
		candidate.Omitted = map[string]int{}
		switch n.Kind {
		case "hint":
			candidate.Hints = append(append([]ContextNode{}, out.Hints...), contextNode(st, n))
		case "work_item":
			candidate.Work = append(append([]ContextNode{}, out.Work...), contextNode(st, n))
		default:
			candidate.Evidence = append(append([]ContextNode{}, out.Evidence...), contextNode(st, n))
		}
		update(&candidate)
		if fits(candidate) {
			out = candidate
			included[n.ID] = true
		}
	}
	eds := append([]Edge{}, st.Edges...)
	sort.Slice(eds, func(i, j int) bool { return eds[i].ID < eds[j].ID })
	for _, ed := range eds {
		if !included[ed.SourceID] || !included[ed.TargetID] {
			continue
		}
		candidate := out
		candidate.Omitted = map[string]int{}
		candidate.Edges = append(append([]Edge{}, out.Edges...), ed)
		update(&candidate)
		if fits(candidate) {
			out = candidate
		}
	}
	return out, nil
}
