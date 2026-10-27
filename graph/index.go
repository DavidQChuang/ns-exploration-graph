package graph

// graphIndex is a lookup over one detached state. Adjacency lists keep st.Edges order
// so traversals report the same first error as a linear scan. Rebuild it after the
// state's node or edge slices change.
type graphIndex struct {
	st    *State
	nodes map[string]*Node
	out   map[string][]*Edge
	in    map[string][]*Edge
	fresh map[string]string
}

func newIndex(st *State) *graphIndex {
	ix := &graphIndex{st: st, nodes: make(map[string]*Node, len(st.Nodes)), out: make(map[string][]*Edge, len(st.Nodes)), in: make(map[string][]*Edge, len(st.Nodes)), fresh: map[string]string{}}
	for i := range st.Nodes {
		if _, ok := ix.nodes[st.Nodes[i].ID]; !ok {
			ix.nodes[st.Nodes[i].ID] = &st.Nodes[i]
		}
	}
	for i := range st.Edges {
		ed := &st.Edges[i]
		ix.out[ed.SourceID] = append(ix.out[ed.SourceID], ed)
		ix.in[ed.TargetID] = append(ix.in[ed.TargetID], ed)
	}
	return ix
}
func (ix *graphIndex) node(id string) (*Node, error) {
	if n := ix.nodes[id]; n != nil {
		return n, nil
	}
	return nil, Err("not_found", "Node not found in graph.")
}
func (ix *graphIndex) current(r Ref) (*Node, error) {
	n, e := ix.node(r.NodeID)
	if e != nil {
		return nil, e
	}
	if r.Revision < 1 || n.CurrentRevision != r.Revision {
		return nil, Err("revision_conflict", "Node revision changed.")
	}
	return n, nil
}
func (ix *graphIndex) closure(r Ref, seen map[string]bool) ([]string, error) {
	n, e := ix.current(r)
	if e != nil {
		return nil, Err("stale_evidence", "Evidence revision is no longer current.")
	}
	if (n.Kind != "observation" && n.Kind != "conclusion") || n.Current().State != "active" {
		return nil, Err("stale_evidence", "Evidence is not active.")
	}
	if seen[n.ID] {
		return nil, Err("cycle_detected", "Support cycle detected.")
	}
	if n.Kind == "observation" {
		return []string{}, nil
	}
	seen[n.ID] = true
	defer delete(seen, n.ID)
	ids := []string{}
	count := 0
	for _, ed := range ix.in[n.ID] {
		if ed.Relation == "supports" && ed.TargetRevision == r.Revision {
			count++
			sub, e := ix.closure(Ref{ed.SourceID, ed.SourceRevision}, seen)
			if e != nil {
				return nil, e
			}
			ids = append(ids, ed.ID)
			ids = append(ids, sub...)
		}
	}
	if count == 0 {
		return nil, Err("stale_evidence", "Conclusion has no support for this revision.")
	}
	return unique(ids), nil
}
func (ix *graphIndex) freshness(n *Node) string {
	if n.Kind != "observation" && n.Kind != "conclusion" {
		return "not_applicable"
	}
	if f, ok := ix.fresh[n.ID]; ok {
		return f
	}
	f := "fresh"
	if _, e := ix.closure(ref(n), map[string]bool{}); e != nil {
		f = "stale"
	}
	ix.fresh[n.ID] = f
	return f
}
func (ix *graphIndex) eligible(n *Node) bool {
	if n.Kind != "work_item" || n.Current().State != "open" {
		return false
	}
	for _, ed := range ix.out[n.ID] {
		if ed.Relation == "depends_on" {
			p, e := ix.node(ed.TargetID)
			if e != nil || p.Current().State != "done" {
				return false
			}
		}
	}
	return true
}
func (ix *graphIndex) summary(n *Node) NodeSummary {
	r := n.Current()
	return NodeSummary{n.ID, n.Kind, n.CurrentRevision, r.State, r.Title, ix.freshness(n)}
}
func (ix *graphIndex) contextNode(n *Node) ContextNode {
	r := n.Current()
	return ContextNode{n.ID, n.Kind, n.CurrentRevision, r.State, r.Title, r.Payload, ix.freshness(n)}
}
