package main

import (
	"context"
	"reflect"
	"testing"
)

// The normalized report replaces generated IDs/timestamps with titles, lifecycles and counts.
// Direct and in-process MCP runs must agree on nodes, relations, revisions, lifecycle,
// command history and acceptance closure, and both must equal this reviewed expectation.
var expected = Report{
	GraphRevision: 9,
	GoalState:     "met",
	Nodes: []string{
		"conclusion:Parser rejection verified:active",
		"goal:Malformed input rejected:met",
		"observation:Malformed input rejection observed:active",
		"origin:Parser investigation:recorded",
		"work_item:Reproduce malformed input:done",
	},
	Lifecycle: []string{
		"conclusion:Parser rejection verified:active",
		"goal:Malformed input rejected:open>met",
		"observation:Malformed input rejection observed:active",
		"origin:Parser investigation:recorded",
		"work_item:Reproduce malformed input:open>running>done",
	},
	Relations: []string{
		"derived_from:Parser investigation→Reproduce malformed input",
		"spawns:Parser investigation→Malformed input rejected",
		"supports:Malformed input rejection observed→Parser rejection verified",
		"supports:Parser rejection verified→Malformed input rejected",
		"yields:Reproduce malformed input→Malformed input rejection observed",
		"yields:Reproduce malformed input→Parser rejection verified",
	},
	Events:            []string{"CreateGraph", "AddGoal", "ProposeWork", "TransitionWork", "RecordObservation", "RecordConclusion", "TransitionWork", "AddSupport", "AcceptGoal"},
	Acceptances:       []string{"validator:current:Host reviewed the stored parser rejection log.:goal=Malformed input rejected@1:edges=2"},
	ReviewedEvidence:  2,
	ReviewedResources: 1,
	ResumedFromGraph:  true,
}

func TestGraphOnlyResumeAndTransportParity(t *testing.T) {
	direct, e := run(context.Background(), "direct")
	if e != nil {
		t.Fatal(e)
	}
	mcp, e := run(context.Background(), "mcp")
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(direct, mcp) {
		t.Fatalf("normalized direct/MCP scenario mismatch\ndirect: %+v\nmcp: %+v", direct, mcp)
	}
	if !reflect.DeepEqual(direct, expected) {
		t.Fatalf("scenario state differs from the reviewed expectation\n got: %+v\nwant: %+v", direct, expected)
	}
}
