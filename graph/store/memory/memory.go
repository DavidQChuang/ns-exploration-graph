package memory

import (
	"context"
	"encoding/json"
	"ns-exploration-graph/graph"
	"sync"
)

type Store struct {
	mu     sync.RWMutex
	states map[string]*graph.State
}

func New() *Store { return &Store{states: map[string]*graph.State{}} }
func (s *Store) Update(ctx context.Context, ns, id string, create *graph.Graph, fn func(*graph.State) error) error {
	return s.update(ctx, ns, id, create, nil, fn)
}

// UpdateRequest exposes only the committed event for requestID to the callback.
func (s *Store) UpdateRequest(ctx context.Context, ns, id string, create *graph.Graph, requestID string, fn func(*graph.State) error) error {
	return s.update(ctx, ns, id, create, &requestID, fn)
}
func (s *Store) update(ctx context.Context, ns, id string, create *graph.Graph, requestID *string, fn func(*graph.State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return e
	}
	old := s.states[id]
	if old == nil {
		if create == nil {
			return graph.Err("not_found", "Graph not found.")
		}
		old = graph.EmptyState(*create)
	}
	if old.Graph.Namespace != ns {
		return graph.Err("permission_denied", "Namespace does not match.")
	}
	visible := old
	if requestID != nil {
		scoped := *old
		scoped.Events = []graph.Event{}
		for _, ev := range old.Events {
			if ev.RequestID == *requestID {
				scoped.Events = append(scoped.Events, ev)
			}
		}
		visible = &scoped
	}
	v, e := graph.CloneState(visible)
	if e != nil {
		return e
	}
	if e = fn(v); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	committed, err := graph.CloneState(v)
	if err != nil {
		return err
	}
	if requestID != nil {
		appended := []graph.Event{}
		for _, ev := range committed.Events {
			if ev.Revision > old.Graph.Revision {
				appended = append(appended, ev)
			}
		}
		committed.Events = append(append([]graph.Event{}, old.Events...), appended...)
	}
	s.states[id] = committed
	return nil
}

// ReadScoped returns a detached copy limited to scope without encoding the full state.
func (s *Store) ReadScoped(ctx context.Context, ns, id string, scope graph.ReadScope) (*graph.State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	v := s.states[id]
	if v == nil {
		return nil, graph.Err("not_found", "Graph not found.")
	}
	if v.Graph.Namespace != ns {
		return nil, graph.Err("permission_denied", "Namespace does not match.")
	}
	out := graph.EmptyState(v.Graph)
	out.Graph.Metadata = nil
	if v.Graph.Metadata != nil {
		metadata, e := graph.CloneState(&graph.State{Snapshot: graph.Snapshot{Graph: graph.Graph{Metadata: v.Graph.Metadata}}})
		if e != nil {
			return nil, e
		}
		out.Graph.Metadata = metadata.Graph.Metadata
	}
	copyRevision := func(r graph.Revision) graph.Revision {
		r.Payload = append(json.RawMessage(nil), r.Payload...)
		return r
	}
	out.Nodes = make([]graph.Node, 0, len(v.Nodes))
	for _, n := range v.Nodes {
		c := n
		if n.ID == scope.DetailNodeID {
			c.Revisions = make([]graph.Revision, 0, len(n.Revisions))
			for _, r := range n.Revisions {
				c.Revisions = append(c.Revisions, copyRevision(r))
			}
		} else {
			c.Revisions = []graph.Revision{copyRevision(*n.Current())}
		}
		out.Nodes = append(out.Nodes, c)
	}
	out.Edges = append(out.Edges, v.Edges...)
	if scope.DetailNodeID != "" {
		for _, r := range v.ResourceRefs {
			if r.NodeID == scope.DetailNodeID {
				out.ResourceRefs = append(out.ResourceRefs, r)
			}
		}
		for _, a := range v.Acceptances {
			if a.GoalID == scope.DetailNodeID {
				if a.SupportEdgeIDs != nil {
					a.SupportEdgeIDs = append([]string{}, a.SupportEdgeIDs...)
				}
				if a.ReviewedResourceIDs != nil {
					a.ReviewedResourceIDs = append([]string{}, a.ReviewedResourceIDs...)
				}
				out.Acceptances = append(out.Acceptances, a)
			}
		}
	}
	out.Events = nil
	return out, nil
}
func (s *Store) Read(ctx context.Context, ns, id string) (*graph.State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	v := s.states[id]
	if v == nil {
		return nil, graph.Err("not_found", "Graph not found.")
	}
	if v.Graph.Namespace != ns {
		return nil, graph.Err("permission_denied", "Namespace does not match.")
	}
	readState := *v
	readState.Events = nil
	c, e := graph.CloneState(&readState)
	if c != nil {
		c.Events = nil
	}
	return c, e
}
func (s *Store) EventsAfter(ctx context.Context, ns, id string, after int64, limit int) ([]graph.Event, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e := ctx.Err(); e != nil {
		return nil, 0, e
	}
	v := s.states[id]
	if v == nil {
		return nil, 0, graph.Err("not_found", "Graph not found.")
	}
	if v.Graph.Namespace != ns {
		return nil, 0, graph.Err("permission_denied", "Namespace does not match.")
	}
	readState := graph.State{Snapshot: graph.Snapshot{Graph: v.Graph}, Events: []graph.Event{}}
	for _, ev := range v.Events {
		if ev.Revision > after && len(readState.Events) < limit {
			readState.Events = append(readState.Events, ev)
		}
	}
	c, e := graph.CloneState(&readState)
	if e != nil {
		return nil, 0, e
	}
	out := []graph.Event{}
	for _, ev := range c.Events {
		if ev.Revision > after && len(out) < limit {
			out = append(out, ev)
		}
	}
	return out, c.Graph.Revision, nil
}
