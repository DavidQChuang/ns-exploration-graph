package graph

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
)

type stateStore struct {
	mu sync.Mutex
	st *State
}

func (s *stateStore) Update(_ context.Context, _, _ string, create *Graph, fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st == nil {
		s.st = EmptyState(*create)
	}
	v, e := CloneState(s.st)
	if e != nil {
		return e
	}
	if e = fn(v); e != nil {
		return e
	}
	s.st = v
	return nil
}
func (s *stateStore) Read(context.Context, string, string) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return CloneState(s.st)
}

// TestIncrementalProjectionMatchesExactEncoding checks that the incremental sizer
// predicts every envelope exactly and selects the same records as re-encoding.
func TestIncrementalProjectionMatchesExactEncoding(t *testing.T) {
	ctx := context.Background()
	alphabet := []string{"a", "é", "検", "🙂", "<", ">", "&", "\u2028", "\"", "\\", " ", "\n"}
	for seed := int64(1); seed <= 4; seed++ {
		rng := rand.New(rand.NewSource(seed))
		text := func(n int) string {
			var b strings.Builder
			for i := 0; i < n; i++ {
				b.WriteString(alphabet[rng.Intn(len(alphabet))])
			}
			return "t" + b.String()
		}
		s := NewService(&stateStore{})
		c := CommandContext{Namespace: "n", GraphID: NewID(), ActorID: "a"}
		next := func() CommandContext { v := c; v.RequestID = NewID(); return v }
		must := func(r Result, e error) Result {
			t.Helper()
			if e != nil {
				t.Fatal(e)
			}
			return r
		}
		must(s.CreateGraph(ctx, next(), CreateGraphInput{Title: text(8), Request: text(30)}))
		for i := 0; i < 1+rng.Intn(3); i++ {
			must(s.AddGoal(ctx, next(), AddGoalInput{Title: text(6), GoalPayload: GoalPayload{AcceptanceCriteria: []string{text(20 + rng.Intn(60))}}}))
		}
		wid := must(s.ProposeWork(ctx, next(), ProposeWorkInput{Title: text(5), WorkPayload: WorkPayload{Instruction: text(10)}})).Data["work_item_id"].(string)
		x := NewID()
		must(s.TransitionWork(ctx, next(), TransitionWorkInput{Node: Ref{wid, 1}, State: "running", ExecutionID: x}))
		var observations []string
		for i := 0; i < 8+rng.Intn(20); i++ {
			wc := next()
			wc.Execution = &Execution{wid, x}
			observations = append(observations, must(s.RecordObservation(ctx, wc, RecordObservationInput{Title: text(4), EvidencePayload: EvidencePayload{Statement: text(rng.Intn(120)), Evidence: text(3), EpistemicStatus: "observed"}})).Data["observation_id"].(string))
		}
		for i := 0; i < rng.Intn(4); i++ {
			must(s.RecordConclusion(ctx, next(), RecordConclusionInput{Title: text(4), EvidencePayload: EvidencePayload{Statement: text(40), EpistemicStatus: "inferred"}, Supports: []Ref{{observations[rng.Intn(len(observations))], 1}}}))
		}
		for i := 0; i < 12+rng.Intn(12); i++ {
			if rng.Intn(3) == 0 {
				must(s.AddHint(ctx, next(), AddHintInput{Title: text(4), HintPayload: HintPayload{Instruction: text(rng.Intn(50))}}))
			} else {
				must(s.ProposeWork(ctx, next(), ProposeWorkInput{Title: text(4), WorkPayload: WorkPayload{Instruction: text(rng.Intn(70)), Priority: rng.Intn(101)}}))
			}
		}
		st, e := s.Store().Read(ctx, c.Namespace, c.GraphID)
		if e != nil {
			t.Fatal(e)
		}
		full, _, e := project(st, ProjectionOptions{AssignedWorkItemID: wid}, 64*1024, true)
		if e != nil {
			t.Fatal(e)
		}
		top := envelopeBytes(full) + 40
		tokens := func(b []byte) int { return len(b) / 3 }
		for budget := 1024; budget <= top; budget += 1 + rng.Intn(149) {
			for _, o := range []ProjectionOptions{{AssignedWorkItemID: wid}, {}, {AssignedWorkItemID: wid, CountTokens: tokens, TokenBudget: budget / 4}} {
				fast, predicted, fe := project(st, o, budget, false)
				exact, _, ee := project(st, o, budget, true)
				name := fmt.Sprintf("seed %d budget %d tokens %v", seed, budget, o.CountTokens != nil)
				if ErrorCode(fe) != ErrorCode(ee) || (fe == nil) != (ee == nil) {
					t.Fatalf("%s: errors differ %v / %v", name, fe, ee)
				}
				if fe != nil {
					continue
				}
				if predicted != envelopeBytes(fast) {
					t.Fatalf("%s: predicted %d, encoded %d", name, predicted, envelopeBytes(fast))
				}
				if string(raw(fast)) != string(raw(exact)) {
					t.Fatalf("%s: incremental selection differs from exact selection", name)
				}
			}
		}
	}
}
