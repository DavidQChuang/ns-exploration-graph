package memory

import (
 "context"
 "sync"
 "ns-exploration-graph/graph"
)
type Store struct { mu sync.RWMutex; states map[string]*graph.State }
func New() *Store {return &Store{states:map[string]*graph.State{}}}
func (s *Store) Update(ctx context.Context,ns,id string,create *graph.Graph,fn func(*graph.State)error)error {
 s.mu.Lock();defer s.mu.Unlock();if e:=ctx.Err();e!=nil{return e}
 old:=s.states[id];if old==nil {if create==nil{return graph.Err("not_found","Graph not found.")};old=graph.EmptyState(*create)}
 if old.Graph.Namespace!=ns{return graph.Err("permission_denied","Namespace does not match.")}
 v,e:=graph.CloneState(old);if e!=nil{return e};if e=fn(v);e!=nil{return e};if e=ctx.Err();e!=nil{return e};s.states[id]=v;return nil
}
func(s *Store) Read(ctx context.Context,ns,id string)(*graph.State,error){s.mu.RLock();defer s.mu.RUnlock();if e:=ctx.Err();e!=nil{return nil,e};v:=s.states[id];if v==nil{return nil,graph.Err("not_found","Graph not found.")};if v.Graph.Namespace!=ns{return nil,graph.Err("permission_denied","Namespace does not match.")};c,e:=graph.CloneState(v);if c!=nil{c.Events=nil};return c,e}
func(s *Store) EventsAfter(ctx context.Context,ns,id string,after int64,limit int)([]graph.Event,int64,error){s.mu.RLock();defer s.mu.RUnlock();if e:=ctx.Err();e!=nil{return nil,0,e};v:=s.states[id];if v==nil{return nil,0,graph.Err("not_found","Graph not found.")};if v.Graph.Namespace!=ns{return nil,0,graph.Err("permission_denied","Namespace does not match.")};c,e:=graph.CloneState(v);if e!=nil{return nil,0,e};out:=[]graph.Event{};for _,ev:=range c.Events{if ev.Revision>after&&len(out)<limit{out=append(out,ev)}};return out,c.Graph.Revision,nil}
