package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"ns-exploration-graph/spec"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

var schemaOnce sync.Once
var snapshotSchema *jsonschema.Schema
var snapshotResourceURISchema *jsonschema.Schema
var snapshotSchemaErr error

func portableSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		var doc any
		doc, snapshotSchemaErr = jsonschema.UnmarshalJSON(bytes.NewReader(spec.SnapshotSchema))
		if snapshotSchemaErr != nil {
			return
		}
		c := jsonschema.NewCompiler()
		spec.RegisterFormats(c)
		snapshotSchemaErr = c.AddResource("https://ns-exploration-graph.invalid/snapshot.schema.json", doc)
		if snapshotSchemaErr == nil {
			snapshotSchema, snapshotSchemaErr = c.Compile("https://ns-exploration-graph.invalid/snapshot.schema.json")
		}
		if snapshotSchemaErr == nil {
			snapshotResourceURISchema, snapshotSchemaErr = c.Compile("https://ns-exploration-graph.invalid/snapshot.schema.json#/$defs/resource/properties/uri")
		}
	})
	return snapshotSchema, snapshotSchemaErr
}

// Host writes use the portable contract's URI validator too, so an accepted
// reference cannot make its own exported snapshot fail format validation.
func validateResourceURI(uri string) error {
	if _, e := portableSchema(); e != nil {
		return Err("storage_unavailable", "Snapshot contract could not be loaded.")
	}
	if e := snapshotResourceURISchema.Validate(uri); e != nil {
		return Err("invalid_argument", "Invalid resource reference.")
	}
	return nil
}

// ValidateSnapshot validates shape with UUID/URI/date-time assertions before
// indexing revision histories, then checks cross-record semantic integrity.
func ValidateSnapshot(encoded []byte) (Snapshot, error) {
	var out Snapshot
	schema, e := portableSchema()
	if e != nil {
		return out, Err("storage_unavailable", "Snapshot contract could not be loaded.")
	}
	doc, e := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if e != nil {
		return out, Err("invalid_argument", "Invalid snapshot JSON.")
	}
	if e = schema.Validate(doc); e != nil {
		return out, Err("invalid_argument", "Snapshot does not match the portable schema.")
	}
	if e = strict(encoded, &out); e != nil {
		return out, e
	}
	canonicalizeSnapshotUUIDs(&out)
	if e = validateSnapshotSemantics(out); e != nil {
		return Snapshot{}, e
	}
	return out, nil
}

// UUID letter case is a representation choice, not an identity distinction.
// Canonicalize the contract's UUID fields before semantic checks and storage so
// UUID columns and UUID references carried in JSON retain the same identities.
// Opaque metadata, payload extensions, URIs and attributable actor strings are
// intentionally not interpreted as UUIDs.
func canonicalizeSnapshotUUIDs(s *Snapshot) {
	s.Graph.ID = strings.ToLower(s.Graph.ID)
	for i := range s.Nodes {
		n := &s.Nodes[i]
		n.ID = strings.ToLower(n.ID)
		if n.Kind != "work_item" {
			continue
		}
		for j := range n.Revisions {
			r := &n.Revisions[j]
			var payload map[string]json.RawMessage
			if json.Unmarshal(r.Payload, &payload) != nil {
				continue
			}
			var executionID string
			if json.Unmarshal(payload["execution_id"], &executionID) != nil || executionID == strings.ToLower(executionID) {
				continue
			}
			payload["execution_id"] = raw(strings.ToLower(executionID))
			r.Payload = raw(payload)
		}
	}
	for i := range s.Edges {
		e := &s.Edges[i]
		e.ID, e.SourceID, e.TargetID = strings.ToLower(e.ID), strings.ToLower(e.SourceID), strings.ToLower(e.TargetID)
	}
	for i := range s.ResourceRefs {
		r := &s.ResourceRefs[i]
		r.ID, r.NodeID = strings.ToLower(r.ID), strings.ToLower(r.NodeID)
	}
	for i := range s.Acceptances {
		a := &s.Acceptances[i]
		a.ID, a.GoalID = strings.ToLower(a.ID), strings.ToLower(a.GoalID)
		for j, id := range a.SupportEdgeIDs {
			a.SupportEdgeIDs[j] = strings.ToLower(id)
		}
		for j, id := range a.ReviewedResourceIDs {
			a.ReviewedResourceIDs[j] = strings.ToLower(id)
		}
	}
}
func snapshotInvalid(message string) error { return Err("invalid_argument", message) }
func validateSnapshotSemantics(snap Snapshot) error {
	st := State{Snapshot: snap}
	ids := map[string]bool{}
	nodes := map[string]*Node{}
	originCount := 0
	executionOwners := map[string]string{}
	claimID := func(id string) bool {
		if ids[id] {
			return false
		}
		ids[id] = true
		return true
	}
	for i := range st.Nodes {
		n := &st.Nodes[i]
		if !claimID(n.ID) {
			return snapshotInvalid("Duplicate snapshot identity.")
		}
		nodes[n.ID] = n
		if n.CurrentRevision != len(n.Revisions) {
			return snapshotInvalid("Current revision must be the maximum contiguous revision.")
		}
		if n.Kind == "origin" {
			originCount++
			if len(n.Revisions) != 1 {
				return snapshotInvalid("Origin is immutable.")
			}
		}
		for j, r := range n.Revisions {
			if r.Revision != j+1 {
				return snapshotInvalid("Node history has a gap or duplicate revision.")
			}
			if e := validatePayload(n.Kind, r.Payload); e != nil {
				return e
			}
			if !validState(n.Kind, r.State) {
				return snapshotInvalid("Invalid node state.")
			}
			if j == 0 {
				initial := map[string]string{"origin": "recorded", "goal": "open", "work_item": "open", "observation": "active", "conclusion": "active", "hint": "active"}
				if r.State != initial[n.Kind] {
					return snapshotInvalid("Invalid initial node state.")
				}
			}
			if n.Kind == "work_item" {
				var w WorkPayload
				_ = json.Unmarshal(r.Payload, &w)
				if r.State == "running" {
					if !ValidID(w.ExecutionID) {
						return snapshotInvalid("Running revision lacks execution identity.")
					}
					if prev, ok := executionOwners[w.ExecutionID]; ok && prev != fmt.Sprintf("%s:%d", n.ID, r.Revision) {
						return snapshotInvalid("Execution identity was reused.")
					}
					executionOwners[w.ExecutionID] = fmt.Sprintf("%s:%d", n.ID, r.Revision)
				}
				if j > 0 {
					var prev WorkPayload
					_ = json.Unmarshal(n.Revisions[j-1].Payload, &prev)
					if r.State != "running" && w.ExecutionID != prev.ExecutionID {
						return snapshotInvalid("Work history changed execution identity outside a claim.")
					}
				}
			}
			if j > 0 && !validHistoryTransition(n.Kind, n.Revisions[j-1], r) {
				return snapshotInvalid("Invalid lifecycle transition in node history.")
			}
		}
		if n.Current().State == "running" {
			return snapshotInvalid("Restore rejects running work; stop or pause the source execution first.")
		}
	}
	if originCount != 1 {
		return snapshotInvalid("Snapshot must contain exactly one origin.")
	}
	resolve := func(id string, rev int) (*Node, *Revision, error) {
		n := nodes[id]
		if n == nil || rev < 1 || rev > len(n.Revisions) {
			return nil, nil, snapshotInvalid("Invalid revision reference.")
		}
		return n, &n.Revisions[rev-1], nil
	}
	links := map[string]bool{}
	for _, ed := range st.Edges {
		if !claimID(ed.ID) {
			return snapshotInvalid("Duplicate snapshot identity.")
		}
		a, b := nodes[ed.SourceID], nodes[ed.TargetID]
		if a == nil || b == nil || a.ID == b.ID || !endpointKinds(a.Kind, b.Kind, ed.Relation) {
			return snapshotInvalid("Invalid relation endpoints.")
		}
		key := fmt.Sprintf("%s:%s:%s:%d:%d", ed.Relation, ed.SourceID, ed.TargetID, ed.SourceRevision, ed.TargetRevision)
		if links[key] {
			return snapshotInvalid("Duplicate relation.")
		}
		links[key] = true
		if ed.Relation == "depends_on" {
			if ed.SourceRevision != 0 || ed.TargetRevision != 0 {
				return snapshotInvalid("Dependencies cannot pin revisions.")
			}
		} else {
			_, ar, e := resolve(ed.SourceID, ed.SourceRevision)
			if e != nil {
				return e
			}
			_, br, e := resolve(ed.TargetID, ed.TargetRevision)
			if e != nil {
				return e
			}
			if ed.Relation == "yields" && ar.State != "running" {
				return snapshotInvalid("Output producer revision must be running.")
			}
			if ed.Relation == "supports" && (ar.State != "active" || !(br.State == "active" || br.State == "open" || br.State == "needs_review")) {
				return snapshotInvalid("Support endpoint states are invalid.")
			}
			if ed.Relation == "derived_from" && a.Kind != "origin" && ar.State != "active" {
				return snapshotInvalid("Derived work has inactive evidence parent.")
			}
		}
	}
	for _, rel := range []string{"supports", "depends_on"} {
		for _, ed := range st.Edges {
			if ed.Relation == rel && hasPath(&st, ed.TargetID, ed.SourceID, rel) {
				return snapshotInvalid("Snapshot contains a relation cycle.")
			}
		}
	}
	resourcesByID := map[string]*ResourceRef{}
	for i, r := range st.ResourceRefs {
		if !claimID(r.ID) {
			return snapshotInvalid("Duplicate snapshot identity.")
		}
		resourcesByID[r.ID] = &st.ResourceRefs[i]
		if _, _, e := resolve(r.NodeID, r.NodeRevision); e != nil {
			return e
		}
		if e := resourceValid(r.ResourceInput); e != nil {
			return e
		}
	}
	for _, n := range st.Nodes {
		if n.Kind == "observation" {
			for _, r := range n.Revisions {
				var p EvidencePayload
				_ = json.Unmarshal(r.Payload, &p)
				if r.State == "active" && !nonempty(p.Evidence) {
					found := false
					for _, rr := range st.ResourceRefs {
						if rr.NodeID == n.ID && rr.NodeRevision == r.Revision {
							found = true
						}
					}
					if !found {
						return snapshotInvalid("Observation revision lacks evidence.")
					}
				}
			}
		}
		relation := ""
		switch n.Kind {
		case "goal":
			relation = "spawns"
		case "work_item":
			relation = "derived_from"
		case "observation":
			relation = "yields"
		case "conclusion":
			relation = "supports"
		}
		if relation != "" {
			found := false
			for _, ed := range st.Edges {
				if ed.TargetID == n.ID && ed.TargetRevision == 1 && ed.Relation == relation {
					found = true
				}
			}
			if !found {
				return snapshotInvalid("Initial node is missing required lineage.")
			}
		}
	}
	currentAcceptances := map[string]int{}
	for _, a := range st.Acceptances {
		if !claimID(a.ID) {
			return snapshotInvalid("Duplicate snapshot identity.")
		}
		g, evaluated, e := resolve(a.GoalID, a.EvaluatedRevision)
		if e != nil {
			return e
		}
		_, result, e := resolve(a.GoalID, a.ResultingRevision)
		if e != nil {
			return e
		}
		if g.Kind != "goal" || a.ResultingRevision != a.EvaluatedRevision+1 || (evaluated.State != "open" && evaluated.State != "needs_review") || result.State != "met" || !jsonEqual(evaluated.Payload, result.Payload) || evaluated.Title != result.Title {
			return snapshotInvalid("Acceptance goal revisions are inconsistent.")
		}
		if result.ActorID != a.ActorID {
			return snapshotInvalid("Acceptance actor differs from goal transition actor.")
		}
		stored := map[string]bool{}
		for _, eid := range a.SupportEdgeIDs {
			if ed := edgeByID(&st, eid); ed == nil || ed.Relation != "supports" {
				return snapshotInvalid("Invalid reviewed support reference.")
			}
			stored[eid] = true
		}
		expected := map[string]bool{}
		// evidence maps each cited revision to whether it is covered without a
		// listed resource (inline observation evidence, or a conclusion).
		evidence := map[Ref]bool{}
		var walk func(string, int, map[string]bool) error
		walk = func(id string, rev int, vis map[string]bool) error {
			n, r, e := resolve(id, rev)
			if e != nil {
				return e
			}
			if (n.Kind != "observation" && n.Kind != "conclusion") || r.State != "active" {
				return snapshotInvalid("Acceptance cites invalid evidence.")
			}
			if a.State == "current" && (n.CurrentRevision != rev || n.Current().State != "active") {
				return snapshotInvalid("Current acceptance cites stale evidence.")
			}
			if vis[id] {
				return snapshotInvalid("Acceptance support cycle.")
			}
			if n.Kind == "observation" {
				var p EvidencePayload
				_ = json.Unmarshal(r.Payload, &p)
				evidence[Ref{id, rev}] = evidence[Ref{id, rev}] || nonempty(p.Evidence)
				return nil
			}
			evidence[Ref{id, rev}] = true
			vis[id] = true
			defer delete(vis, id)
			found := false
			for _, ed := range st.Edges {
				if stored[ed.ID] && ed.TargetID == id && ed.TargetRevision == rev {
					found = true
					expected[ed.ID] = true
					if e := walk(ed.SourceID, ed.SourceRevision, vis); e != nil {
						return e
					}
				}
			}
			if !found {
				return snapshotInvalid("Accepted conclusion has no reviewed support.")
			}
			return nil
		}
		direct := 0
		for _, eid := range a.SupportEdgeIDs {
			ed := edgeByID(&st, eid)
			if ed.TargetID == a.GoalID && ed.TargetRevision == a.EvaluatedRevision {
				direct++
				expected[eid] = true
				if e := walk(ed.SourceID, ed.SourceRevision, map[string]bool{}); e != nil {
					return e
				}
			}
		}
		if direct == 0 || len(expected) != len(a.SupportEdgeIDs) {
			return snapshotInvalid("Acceptance support closure is incomplete or unrelated.")
		}
		for _, eid := range a.SupportEdgeIDs {
			if !expected[eid] {
				return snapshotInvalid("Unrelated acceptance support.")
			}
		}
		// Resources may be attached after acceptance, so only listed ones are
		// checked; a reference-only observation always had one at decision time.
		for _, rid := range a.ReviewedResourceIDs {
			r := resourcesByID[rid]
			if r == nil {
				return snapshotInvalid("Reviewed resource is not pinned to reviewed evidence.")
			}
			if _, ok := evidence[Ref{r.NodeID, r.NodeRevision}]; !ok {
				return snapshotInvalid("Reviewed resource is not pinned to reviewed evidence.")
			}
			evidence[Ref{r.NodeID, r.NodeRevision}] = true
		}
		for _, reviewed := range evidence {
			if !reviewed {
				return snapshotInvalid("Reviewed resources omit reference-only evidence.")
			}
		}
		if a.State == "current" {
			currentAcceptances[g.ID]++
			if g.CurrentRevision != a.ResultingRevision || g.Current().State != "met" {
				return snapshotInvalid("Current acceptance does not match current goal.")
			}
		}
	}
	for _, n := range st.Nodes {
		if n.Kind == "goal" {
			count := currentAcceptances[n.ID]
			if (n.Current().State == "met" && count != 1) || (n.Current().State != "met" && count != 0) {
				return snapshotInvalid("Goal state and current acceptance disagree.")
			}
			for _, r := range n.Revisions {
				if r.State == "met" {
					found := false
					for _, a := range st.Acceptances {
						if a.GoalID == n.ID && a.ResultingRevision == r.Revision {
							found = true
						}
					}
					if !found {
						return snapshotInvalid("Met revision lacks an acceptance.")
					}
				}
			}
		}
	}
	return nil
}
func jsonEqual(a, b []byte) bool {
	var av, bv any
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}
func validHistoryTransition(kind string, from, to Revision) bool {
	a, b := from.State, to.State
	if a == b {
		switch kind {
		case "goal":
			return a == "open" || a == "needs_review"
		case "work_item":
			return a == "open"
		case "observation", "conclusion", "hint":
			return a == "active"
		}
		return false
	}
	switch kind {
	case "goal":
		return (a == "open" && (b == "met" || b == "abandoned")) || (a == "met" && (b == "needs_review" || b == "open" || b == "abandoned")) || (a == "needs_review" && (b == "met" || b == "open" || b == "abandoned")) || (a == "abandoned" && b == "open")
	case "work_item":
		allowed := map[string]map[string]bool{"open": {"running": true, "stopped": true}, "running": {"paused": true, "done": true, "blocked": true, "exhausted": true, "stopped": true}, "paused": {"open": true, "stopped": true}, "blocked": {"open": true}, "exhausted": {"open": true}, "stopped": {"open": true}}
		return allowed[a][b]
	case "observation", "conclusion":
		return a == "active" && b == "retracted"
	case "hint":
		return a == "active" && b == "consumed"
	}
	return false
}

// ImportSnapshot restores portable history into a caller-authorized, unused graph ID.
// Node identities and revisions remain intact; request/event history does not transfer.
func (s *Service) ImportSnapshot(ctx context.Context, c CommandContext, encoded []byte) (Result, error) {
	if len(encoded) > s.Limits.SnapshotBytes {
		return Result{}, Err("limit_exceeded", "Snapshot exceeds configured size limit.")
	}
	snapshot, e := ValidateSnapshot(encoded)
	if e != nil {
		return Result{}, e
	}
	if _, exists := snapshot.Graph.Metadata["import_source"]; exists {
		return Result{}, Err("invalid_argument", "Metadata import_source already exists.")
	}
	source := map[string]any{"graph_id": snapshot.Graph.ID, "revision": snapshot.Graph.Revision}
	snapshot.Graph.ID = c.GraphID
	snapshot.Graph.Namespace = c.Namespace
	snapshot.Graph.Revision = 0
	snapshot.Graph.Metadata["import_source"] = source
	now := time.Now().UTC()
	snapshot.Graph.CreatedAt = now
	snapshot.Graph.UpdatedAt = now
	g := snapshot.Graph
	g.State = "active"
	return s.mutateWithLimit(ctx, c, "ImportSnapshot", json.RawMessage(encoded), &g, s.Limits.SnapshotBytes+4096, func(st *State) (map[string]any, error) {
		st.Snapshot = snapshot
		return map[string]any{"graph_id": c.GraphID, "import_source": source, "node_count": len(snapshot.Nodes)}, nil
	})
}
func (s *Service) ExportSnapshot(ctx context.Context, ns, id string) ([]byte, error) {
	snapshot, e := s.Snapshot(ctx, ns, id)
	if e != nil {
		return nil, e
	}
	sort.Slice(snapshot.Nodes, func(i, j int) bool { return snapshot.Nodes[i].ID < snapshot.Nodes[j].ID })
	sort.Slice(snapshot.Edges, func(i, j int) bool { return snapshot.Edges[i].ID < snapshot.Edges[j].ID })
	sort.Slice(snapshot.ResourceRefs, func(i, j int) bool { return snapshot.ResourceRefs[i].ID < snapshot.ResourceRefs[j].ID })
	sort.Slice(snapshot.Acceptances, func(i, j int) bool { return snapshot.Acceptances[i].ID < snapshot.Acceptances[j].ID })
	return json.Marshal(snapshot)
}
