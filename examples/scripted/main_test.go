package main

import (
	"context"
	"reflect"
	"testing"
)

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
	if direct.GoalState != "met" || !direct.ResumedFromGraph || direct.ReviewedEvidence != 2 || direct.ReviewedResources != 1 || direct.GraphRevision != 9 {
		t.Fatalf("incomplete resume scenario %+v", direct)
	}
}
