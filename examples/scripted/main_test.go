package main

import (
	"context"
	"reflect"
	"testing"
)

// Fixed reviewed golden state, independent of the implementation-generated reports.
var expected = Report{
	GraphRevision: 14,
	GoalState:     "met",
	Nodes: []string{
		"conclusion:Parser input fix verified:active",
		"conclusion:Parser rejection verified:active",
		"goal:Corrected parser input verified:met",
		"observation:Corrected input verification passed:active",
		"observation:Malformed input rejection observed:active",
		"origin:Parser investigation:recorded",
		"work_item:Reproduce malformed input:done",
		"work_item:Verify parser input fix:done",
	},
	Lifecycle: []string{
		"conclusion:Parser input fix verified:active",
		"conclusion:Parser rejection verified:active",
		"goal:Corrected parser input verified:open>met",
		"observation:Corrected input verification passed:active",
		"observation:Malformed input rejection observed:active",
		"origin:Parser investigation:recorded",
		"work_item:Reproduce malformed input:open>running>done",
		"work_item:Verify parser input fix:open>running>done",
	},
	Relations: []string{
		"derived_from:Parser investigation→Reproduce malformed input",
		"derived_from:Parser rejection verified→Verify parser input fix",
		"spawns:Parser investigation→Corrected parser input verified",
		"supports:Corrected input verification passed→Parser input fix verified",
		"supports:Malformed input rejection observed→Parser rejection verified",
		"supports:Parser input fix verified→Corrected parser input verified",
		"supports:Parser rejection verified→Parser input fix verified",
		"yields:Reproduce malformed input→Malformed input rejection observed",
		"yields:Reproduce malformed input→Parser rejection verified",
		"yields:Verify parser input fix→Corrected input verification passed",
		"yields:Verify parser input fix→Parser input fix verified",
	},
	Events:            []string{"CreateGraph", "AddGoal", "ProposeWork", "TransitionWork", "RecordObservation", "RecordConclusion", "TransitionWork", "ProposeWork", "TransitionWork", "RecordObservation", "RecordConclusion", "TransitionWork", "AddSupport", "AcceptGoal"},
	Acceptances:       []string{"validator:current:Host reviewed the stored failure and passing verification logs.:goal=Corrected parser input verified@1:edges=4"},
	ReviewedEvidence:  4,
	ReviewedResources: 2,
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
