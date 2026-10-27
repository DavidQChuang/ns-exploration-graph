// graph-mcp serves one pre-existing authorized graph on local stdio.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	transport "ns-exploration-graph/adapters/mcp"
	"ns-exploration-graph/graph"
	"ns-exploration-graph/graph/store/postgres"
	"ns-exploration-graph/toolkit"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	ns := flag.String("namespace", "", "authorized graph namespace")
	id := flag.String("graph", "", "pre-existing graph UUID")
	actor := flag.String("actor", "", "attributable actor")
	role := flag.String("role", "planner", "planner, worker, or solo")
	work := flag.String("work", "", "host-assigned work UUID")
	execution := flag.String("execution", "", "host-issued execution UUID")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	dsn := os.Getenv("EXPLORATION_DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("EXPLORATION_DATABASE_URL is required")
	}
	store, e := postgres.Open(ctx, dsn)
	if e != nil {
		return fmt.Errorf("cannot open graph database")
	}
	defer store.Close()
	svc := graph.NewService(store)
	b := toolkit.Binding{Namespace: *ns, GraphID: *id, ActorID: *actor, Role: *role}
	if *work != "" || *execution != "" {
		if !graph.ValidID(*work) || !graph.ValidID(*execution) {
			return fmt.Errorf("both valid work and execution UUIDs are required")
		}
		b.Execution = &graph.Execution{WorkItemID: *work, ExecutionID: *execution}
	}
	a, e := toolkit.New(svc, toolkit.Config{Binding: b})
	if e != nil {
		return e
	}
	return transport.RunStdio(ctx, a)
}
