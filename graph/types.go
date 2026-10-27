package graph

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "time"
)

type Error struct { Code string `json:"code"`; Message string `json:"message"`; Retryable bool `json:"retryable"`; Details map[string]any `json:"details,omitempty"` }
func (e *Error) Error() string { return e.Code+": "+e.Message }
func Err(code, message string) *Error { return &Error{Code:code,Message:message,Retryable:code=="storage_unavailable"} }
func ErrorCode(err error) string { if e,ok:=err.(*Error);ok{return e.Code};return "storage_unavailable" }
func NewID() string { var b [16]byte; if _,err:=rand.Read(b[:]);err!=nil {panic(err)}; b[6]=(b[6]&15)|64;b[8]=(b[8]&63)|128; s:=hex.EncodeToString(b[:]);return fmt.Sprintf("%s-%s-%s-%s-%s",s[:8],s[8:12],s[12:16],s[16:20],s[20:]) }

type Graph struct { ID string `json:"id"`; Namespace string `json:"namespace"`; ExternalRef string `json:"external_ref,omitempty"`; Description string `json:"description"`; State string `json:"state"`; Revision int64 `json:"revision"`; Metadata map[string]any `json:"metadata"`; CreatedAt time.Time `json:"created_at"`; UpdatedAt time.Time `json:"updated_at"` }
type Revision struct { Revision int `json:"revision"`; State string `json:"state"`; Title string `json:"title"`; Payload json.RawMessage `json:"payload"`; ActorID string `json:"actor_id"`; CreatedAt time.Time `json:"created_at"` }
type Node struct { ID string `json:"id"`; Kind string `json:"kind"`; CurrentRevision int `json:"current_revision"`; CreatedAt time.Time `json:"created_at"`; Revisions []Revision `json:"revisions"` }
func (n *Node) Current() *Revision { return &n.Revisions[n.CurrentRevision-1] }
type Ref struct { NodeID string `json:"node_id"`; Revision int `json:"revision"` }
type Edge struct { ID string `json:"id"`; SourceID string `json:"source_id"`; TargetID string `json:"target_id"`; SourceRevision int `json:"source_revision,omitempty"`; TargetRevision int `json:"target_revision,omitempty"`; Relation string `json:"relation"`; ActorID string `json:"actor_id"`; CreatedAt time.Time `json:"created_at"` }
type ResourceInput struct { ResourceType string `json:"resource_type"`; URI string `json:"uri"`; ExternalRevision string `json:"external_revision,omitempty"`; SHA256 string `json:"sha256,omitempty"` }
type ResourceRef struct { ID string `json:"id"`; NodeID string `json:"node_id"`; NodeRevision int `json:"node_revision"`; ResourceInput; ActorID string `json:"actor_id"`; CreatedAt time.Time `json:"created_at"` }
type Acceptance struct { ID string `json:"id"`; GoalID string `json:"goal_id"`; EvaluatedRevision int `json:"evaluated_revision"`; ResultingRevision int `json:"resulting_revision"`; SupportEdgeIDs []string `json:"support_edge_ids"`; ReviewedResourceIDs []string `json:"reviewed_resource_ids"`; ActorID string `json:"actor_id"`; Method string `json:"method"`; Rationale string `json:"rationale"`; State string `json:"state"`; StaleReason string `json:"stale_reason,omitempty"`; CreatedAt time.Time `json:"created_at"` }
type Snapshot struct { SchemaVersion int `json:"schema_version"`; Graph Graph `json:"graph"`; Nodes []Node `json:"nodes"`; Edges []Edge `json:"edges"`; ResourceRefs []ResourceRef `json:"resource_refs"`; Acceptances []Acceptance `json:"acceptances"` }
type Result struct { GraphRevision int64 `json:"graph_revision"`; Data map[string]any `json:"data"` }
type Event struct { Revision int64 `json:"revision"`; RequestID string `json:"request_id"`; RequestHash string `json:"request_hash"`; CommandKind string `json:"command_kind"`; ActorID string `json:"actor_id"`; Data json.RawMessage `json:"data"`; Result Result `json:"result"`; CreatedAt time.Time `json:"created_at"` }
type State struct { Snapshot; Events []Event `json:"events"` }
// Store serializes Update callbacks per graph and commits their complete detached state
// atomically. Failed callbacks persist nothing, including newly created graphs. Read
// returns a detached, consistent snapshot. Implementations check namespaces before callbacks.
type Store interface { Update(context.Context,string,string,*Graph,func(*State) error) error; Read(context.Context,string,string)(*State,error) }
func EmptyState(g Graph) *State { return &State{Snapshot:Snapshot{SchemaVersion:1,Graph:g,Nodes:[]Node{},Edges:[]Edge{},ResourceRefs:[]ResourceRef{},Acceptances:[]Acceptance{}},Events:[]Event{}} }
func CloneState(s *State) (*State,error) { b,e:=json.Marshal(s);if e!=nil{return nil,e};var v State;e=json.Unmarshal(b,&v);return &v,e }

type Execution struct { WorkItemID string `json:"work_item_id"`; ExecutionID string `json:"execution_id"` }
type CommandContext struct { Namespace string `json:"namespace"`; GraphID string `json:"graph_id"`; RequestID string `json:"request_id"`; ActorID string `json:"actor_id"`; ExpectedGraphRevision *int64 `json:"expected_graph_revision,omitempty"`; Execution *Execution `json:"execution,omitempty"` }
type OriginPayload struct { Request string `json:"request"`; Extensions map[string]any `json:"extensions,omitempty"` }
type GoalPayload struct { AcceptanceCriteria []string `json:"acceptance_criteria"`; Extensions map[string]any `json:"extensions,omitempty"` }
type WorkPayload struct { Instruction string `json:"instruction"`; Priority int `json:"priority"`; OwnerRef string `json:"owner_ref,omitempty"`; ExecutionID string `json:"execution_id,omitempty"`; Reason string `json:"reason,omitempty"`; Extensions map[string]any `json:"extensions,omitempty"` }
type EvidencePayload struct { Statement string `json:"statement"`; EpistemicStatus string `json:"epistemic_status"`; Polarity string `json:"polarity,omitempty"`; Detail string `json:"detail,omitempty"`; Evidence string `json:"evidence,omitempty"`; Extensions map[string]any `json:"extensions,omitempty"` }
type HintPayload struct { Instruction string `json:"instruction"`; Extensions map[string]any `json:"extensions,omitempty"` }
type CreateGraphInput struct { Title string `json:"title"`; Request string `json:"request"`; Description string `json:"description"`; ExternalRef string `json:"external_ref,omitempty"`; Metadata map[string]any `json:"metadata,omitempty"` }
type AddGoalInput struct { Title string `json:"title"`; GoalPayload }
type ProposeWorkInput struct { Title string `json:"title"`; WorkPayload; Parents []Ref `json:"parents,omitempty"` }
type RecordObservationInput struct { Title string `json:"title"`; EvidencePayload; Resources []ResourceInput `json:"resource_refs,omitempty"` }
type RecordConclusionInput struct { Title string `json:"title"`; EvidencePayload; Supports []Ref `json:"supports"`; Resources []ResourceInput `json:"resource_refs,omitempty"` }
type AddSupportInput struct { Source Ref `json:"source"`; Target Ref `json:"target"` }
type TransitionWorkInput struct { Node Ref `json:"node"`; State string `json:"state"`; ExecutionID string `json:"execution_id,omitempty"`; Reason string `json:"reason,omitempty"` }
type ReviseNodeInput struct { Node Ref `json:"node"`; Title string `json:"title"`; Payload json.RawMessage `json:"payload"` }
type AcceptGoalInput struct { Goal Ref `json:"goal"`; SupportEdgeIDs []string `json:"support_edge_ids"`; Method string `json:"method"`; Rationale string `json:"rationale"` }
type DependencyInput struct { Dependent Ref `json:"dependent"`; PrerequisiteID string `json:"prerequisite_id"` }
type NodeActionInput struct { Node Ref `json:"node"`; Reason string `json:"reason,omitempty"` }
type AddHintInput struct { Title string `json:"title"`; HintPayload }
type AttachResourceInput struct { Node Ref `json:"node"`; Resource ResourceInput `json:"resource"` }
type SetGraphStateInput struct { State string `json:"state"` }
// EventStore provides bounded durable event reads independently of ordinary snapshots.
type EventStore interface { EventsAfter(context.Context,string,string,int64,int)([]Event,int64,error) }
