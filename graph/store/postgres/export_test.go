package postgres

import (
	"context"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// StatementLog records every statement pgx sends, including BEGIN and COMMIT.
type StatementLog struct {
	mu  sync.Mutex
	sql []string
}

func (l *StatementLog) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	l.mu.Lock()
	l.sql = append(l.sql, strings.TrimSpace(data.SQL))
	l.mu.Unlock()
	return ctx
}
func (l *StatementLog) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// Take returns and clears the recorded statements.
func (l *StatementLog) Take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.sql
	l.sql = nil
	return out
}

// OpenTraced opens a store whose statements are recorded in log.
func OpenTraced(ctx context.Context, dsn string, log *StatementLog, options ...func(*Store)) (*Store, error) {
	config, e := pgx.ParseConfig(dsn)
	if e != nil {
		return nil, e
	}
	config.Tracer = log
	s := &Store{db: stdlib.OpenDB(*config)}
	for _, option := range options {
		option(s)
	}
	if e = s.db.PingContext(ctx); e != nil {
		s.db.Close()
		return nil, e
	}
	return s, nil
}
