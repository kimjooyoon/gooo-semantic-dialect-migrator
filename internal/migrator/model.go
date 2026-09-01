package migrator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	MetaSchema        = "gooo/semantic-dialect-migrator/meta/v1"
	CaseSchema        = "gooo/semantic-dialect-migrator/case/v1"
	ProgramSchema     = "gooo/semantic-dialect-migrator/program/v1"
	IRSchema          = "gooo/semantic-dialect-migrator/semantic-ir/v1"
	BindingSchema     = "gooo/semantic-dialect-migrator/generated-binding/v1"
	ReportSchema      = "gooo/semantic-dialect-migrator/case-report/v1"
	ConformanceSchema = "gooo/semantic-dialect-migrator/conformance/v1"
	MetricsSchema     = "gooo/semantic-dialect-migrator/metrics/v1"
	DecisionClosed    = "CLOSED"
	DecisionUnknown   = "UNKNOWN"
	DecisionRefuted   = "REFUTED"
)

var RequiredUnknownFields = []string{"stage", "step", "reason", "unknown_class", "next_operation", "blocked_by"}
var Precedence = []string{DecisionRefuted, DecisionUnknown, DecisionClosed}

type MetaContract struct {
	Schema          string
	Authority       string
	Denominator     DenominatorDecl
	AuthorityPolicy map[string]string
	SourcePolicy    map[string]string
	Precedence      []string
	UnknownFields   []string
	Dialects        []string
	Predicates      []PredicateDecl
	Operations      []OperationDecl
	Cases           []CaseDecl
}

type DenominatorDecl struct {
	ID    string
	Cases int
	Unit  string
}

type PredicateDecl struct {
	Ordinal  int
	ID       string
	Kind     string
	Preserve bool
}

type OperationDecl struct {
	Ordinal         int
	ID              string
	Kind            string
	Reversible      bool
	Lossy           bool
	RequiresOrigin  bool
	RequiresInverse bool
	Preserves       []string
}

type CaseDecl struct {
	Ordinal  int
	ID       string
	Fixture  string
	Expected string
}

type Program struct {
	Schema         string `json:"schema"`
	ID             string `json:"id"`
	Dialect        string `json:"dialect"`
	Entry          string `json:"entry"`
	Nodes          []Node `json:"nodes"`
	Edges          []Edge `json:"edges"`
	TerminalReason string `json:"terminal_reason"`
}

type Node struct {
	ID           string   `json:"id"`
	Symbol       string   `json:"symbol"`
	Capabilities []string `json:"capabilities"`
	Effects      []string `json:"effects"`
}

type Edge struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

type MigrationCase struct {
	Schema           string               `json:"schema"`
	CaseID           string               `json:"case_id"`
	SourcePath       string               `json:"source_path"`
	SourceDialect    string               `json:"source_dialect"`
	TargetDialect    string               `json:"target_dialect"`
	ExpectedDecision string               `json:"expected_decision"`
	Operations       []MigrationOperation `json:"operations"`
	Replay           []ReplayInput        `json:"replay"`
}

type MigrationOperation struct {
	ID             string       `json:"id"`
	Kind           string       `json:"kind"`
	From           string       `json:"from,omitempty"`
	To             string       `json:"to,omitempty"`
	Into           []string     `json:"into,omitempty"`
	FromCapability string       `json:"from_capability,omitempty"`
	ToCapability   string       `json:"to_capability,omitempty"`
	Reason         string       `json:"reason,omitempty"`
	Comment        string       `json:"comment,omitempty"`
	Lossy          bool         `json:"lossy,omitempty"`
	Inverse        *bool        `json:"inverse,omitempty"`
	Origins        []OriginPair `json:"origins,omitempty"`
	Parts          []SplitPart  `json:"parts,omitempty"`
}

type OriginPair struct {
	From string   `json:"from"`
	To   []string `json:"to"`
}

type SplitPart struct {
	ID           string   `json:"id"`
	Symbol       string   `json:"symbol"`
	Capabilities []string `json:"capabilities"`
	Effects      []string `json:"effects"`
}

type ReplayInput struct {
	InputID         string   `json:"input_id"`
	ExpectedReason  string   `json:"expected_terminal_reason"`
	ExpectedEffects []string `json:"expected_effect_trace"`
}

type SemanticIR struct {
	Schema         string   `json:"schema"`
	ProgramID      string   `json:"program_id"`
	Dialect        string   `json:"dialect"`
	SourcePath     string   `json:"source_path"`
	SourceDigest   string   `json:"source_digest"`
	SemanticDigest string   `json:"semantic_digest"`
	Nodes          []Node   `json:"nodes"`
	Edges          []Edge   `json:"edges"`
	Entry          string   `json:"entry"`
	TerminalReason string   `json:"terminal_reason"`
	CapabilitySet  []string `json:"capability_set"`
	EffectTrace    []string `json:"effect_trace"`
}

type GeneratedBinding struct {
	Schema         string `json:"schema"`
	ProgramID      string `json:"program_id"`
	Dialect        string `json:"dialect"`
	SourceDigest   string `json:"source_digest"`
	SemanticDigest string `json:"semantic_digest"`
}

type PredicateEvidence struct {
	ID               string `json:"id"`
	DeclaredPreserve bool   `json:"declared_preserve"`
	ObservedPreserve bool   `json:"observed_preserve"`
	BeforeDigest     string `json:"before_digest"`
	AfterDigest      string `json:"after_digest"`
	Detail           string `json:"detail"`
}

type UnknownRecord struct {
	Stage         string   `json:"stage"`
	Step          string   `json:"step"`
	Reason        string   `json:"reason"`
	UnknownClass  string   `json:"unknown_class"`
	NextOperation string   `json:"next_operation"`
	BlockedBy     []string `json:"blocked_by"`
}

type ReplayEvidence struct {
	InputID         string   `json:"input_id"`
	SourceReason    string   `json:"source_reason"`
	TargetReason    string   `json:"target_reason"`
	SourceEffects   []string `json:"source_effects"`
	TargetEffects   []string `json:"target_effects"`
	ExpectedReason  string   `json:"expected_reason"`
	ExpectedEffects []string `json:"expected_effects"`
	Exact           bool     `json:"exact"`
}

type StageMetric struct {
	Stage      string `json:"stage"`
	WallMS     int    `json:"wall_ms"`
	PeakRSSKiB int    `json:"peak_rss_kib"`
}

type CaseReport struct {
	Schema             string               `json:"schema"`
	CaseID             string               `json:"case_id"`
	ExpectedDecision   string               `json:"expected_decision"`
	Decision           string               `json:"decision"`
	SourcePath         string               `json:"source_path"`
	MigratedSourcePath string               `json:"migrated_source_path"`
	SourceIR           SemanticIR           `json:"source_ir"`
	TargetIR           SemanticIR           `json:"target_ir"`
	OriginMap          []OriginPair         `json:"origin_map"`
	PredicateVector    []PredicateEvidence  `json:"predicate_vector"`
	Replay             []ReplayEvidence     `json:"replay"`
	Unknowns           []UnknownRecord      `json:"unknowns"`
	Operations         []MigrationOperation `json:"operations"`
	StageMetrics       []StageMetric        `json:"stage_metrics"`
	GeneratedFiles     []string             `json:"generated_files"`
	GeneratedBytes     int                  `json:"generated_bytes"`
	RepositoryWrites   int                  `json:"repository_writes"`
}

type ConformanceCase struct {
	Ordinal         int                 `json:"ordinal"`
	CaseID          string              `json:"case_id"`
	Expected        string              `json:"expected"`
	Observed        string              `json:"observed"`
	Pass            bool                `json:"pass"`
	PredicateVector []PredicateEvidence `json:"predicate_vector"`
	Unknowns        []UnknownRecord     `json:"unknowns"`
}

type Inventory struct {
	DescendantDirs    int `json:"descendant_dirs"`
	RegularFiles      int `json:"regular_files"`
	GoFiles           int `json:"go_files"`
	GoPhysicalLines   int `json:"go_physical_lines"`
	GoooFiles         int `json:"gooo_files"`
	GoooPhysicalLines int `json:"gooo_physical_lines"`
}

type TestMetrics struct {
	Total    int `json:"total"`
	Selected int `json:"selected"`
	Executed int `json:"executed"`
	Reused   int `json:"reused"`
	Failed   int `json:"failed"`
	Unknown  int `json:"unknown"`
}

type Metrics struct {
	Schema           string        `json:"schema"`
	Cases            int           `json:"cases"`
	Closed           int           `json:"closed"`
	Unknown          int           `json:"unknown"`
	Refuted          int           `json:"refuted"`
	SourceFiles      int           `json:"source_files"`
	IRFiles          int           `json:"ir_files"`
	GeneratedFiles   int           `json:"generated_files"`
	GeneratedBytes   int           `json:"generated_bytes"`
	Inventory        Inventory     `json:"inventory"`
	Tests            TestMetrics   `json:"tests"`
	StageMetrics     []StageMetric `json:"stage_metrics"`
	RepositoryWrites int           `json:"repository_writes"`
}

type ConformanceReport struct {
	Schema             string            `json:"schema"`
	MetaDigest         string            `json:"meta_digest"`
	Decision           string            `json:"decision"`
	ExpectedPrecedence []string          `json:"expected_precedence"`
	Cases              []ConformanceCase `json:"cases"`
	FixedCaseVector    []string          `json:"fixed_case_vector"`
	Metrics            Metrics           `json:"metrics"`
}

type Execution struct {
	TerminalReason string   `json:"terminal_reason"`
	EffectTrace    []string `json:"effect_trace"`
	CapabilitySet  []string `json:"capability_set"`
}

func DigestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func DigestValue(value any) string {
	raw, _ := json.Marshal(value)
	return DigestBytes(raw)
}

func (ir SemanticIR) CanonicalDigest() string {
	canonical := struct {
		ProgramID      string   `json:"program_id"`
		Entry          string   `json:"entry"`
		Nodes          []Node   `json:"nodes"`
		Edges          []Edge   `json:"edges"`
		TerminalReason string   `json:"terminal_reason"`
		CapabilitySet  []string `json:"capability_set"`
		EffectTrace    []string `json:"effect_trace"`
	}{ir.ProgramID, ir.Entry, ir.Nodes, ir.Edges, ir.TerminalReason, ir.CapabilitySet, ir.EffectTrace}
	return DigestValue(canonical)
}

func (p Program) canonical() Program {
	result := p
	result.Nodes = append([]Node(nil), p.Nodes...)
	result.Edges = append([]Edge(nil), p.Edges...)
	for i := range result.Nodes {
		result.Nodes[i].Capabilities = sortedUnique(result.Nodes[i].Capabilities)
		result.Nodes[i].Effects = append([]string(nil), result.Nodes[i].Effects...)
	}
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].ID < result.Nodes[j].ID })
	sort.Slice(result.Edges, func(i, j int) bool {
		if result.Edges[i].From != result.Edges[j].From {
			return result.Edges[i].From < result.Edges[j].From
		}
		return result.Edges[i].To < result.Edges[j].To
	})
	return result
}

func (ir SemanticIR) canonical() SemanticIR {
	result := ir
	result.Nodes = append([]Node(nil), ir.Nodes...)
	result.Edges = append([]Edge(nil), ir.Edges...)
	for i := range result.Nodes {
		result.Nodes[i].Capabilities = sortedUnique(result.Nodes[i].Capabilities)
	}
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].ID < result.Nodes[j].ID })
	sort.Slice(result.Edges, func(i, j int) bool { return result.Edges[i].ID < result.Edges[j].ID })
	result.CapabilitySet = sortedUnique(result.CapabilitySet)
	return result
}

func (p Program) Validate() error {
	if p.Schema != ProgramSchema || p.ID == "" || p.Dialect == "" || p.Entry == "" || p.TerminalReason == "" {
		return errors.New("program identity or terminal declaration is incomplete")
	}
	nodes := map[string]bool{}
	for _, node := range p.Nodes {
		if node.ID == "" || node.Symbol == "" || nodes[node.ID] {
			return fmt.Errorf("invalid or duplicate node %q", node.ID)
		}
		nodes[node.ID] = true
	}
	if !nodes[p.Entry] || len(nodes) == 0 {
		return errors.New("program entry is not a node")
	}
	edges := map[string]bool{}
	for _, edge := range p.Edges {
		if edge.From == "" || edge.To == "" || !nodes[edge.From] || !nodes[edge.To] {
			return errors.New("edge references an unknown node")
		}
		edge.ID = edgeID(edge.From, edge.To)
		if edges[edge.ID] {
			return errors.New("duplicate edge")
		}
		edges[edge.ID] = true
	}
	return nil
}

func (c MigrationCase) Validate(meta MetaContract) error {
	if c.Schema != CaseSchema || c.CaseID == "" || c.SourcePath == "" || c.SourceDialect == "" || c.TargetDialect == "" || !allowedDecision(c.ExpectedDecision) {
		return errors.New("case identity is incomplete")
	}
	if c.SourceDialect == c.TargetDialect {
		return errors.New("source and target dialect must differ")
	}
	if _, ok := meta.Case(c.CaseID); !ok {
		return fmt.Errorf("case %q is not declared by the .gooo meta source", c.CaseID)
	}
	if len(c.Operations) == 0 {
		return errors.New("case has no typed migration operation")
	}
	for _, operation := range c.Operations {
		if operation.Kind == "" {
			return errors.New("operation kind is empty")
		}
	}
	return nil
}

func (m MetaContract) Case(id string) (CaseDecl, bool) {
	for _, item := range m.Cases {
		if item.ID == id {
			return item, true
		}
	}
	return CaseDecl{}, false
}

func (m MetaContract) Operation(kind string) (OperationDecl, bool) {
	for _, item := range m.Operations {
		if item.Kind == kind || item.ID == kind {
			return item, true
		}
	}
	return OperationDecl{}, false
}

func (m MetaContract) Predicate(id string) (PredicateDecl, bool) {
	for _, item := range m.Predicates {
		if item.ID == id {
			return item, true
		}
	}
	return PredicateDecl{}, false
}

func (u UnknownRecord) Validate() error {
	if u.Stage == "" || u.Step == "" || u.Reason == "" || u.UnknownClass == "" || u.NextOperation == "" || len(u.BlockedBy) == 0 {
		return errors.New("UNKNOWN record must contain the six required fields")
	}
	return nil
}

func (o MigrationOperation) originMap() []OriginPair {
	result := make([]OriginPair, len(o.Origins))
	copy(result, o.Origins)
	for i := range result {
		result[i].To = sortedUnique(result[i].To)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].From < result[j].From })
	return result
}

func normalizeOrigins(values []OriginPair) []OriginPair {
	result := make([]OriginPair, len(values))
	copy(result, values)
	for i := range result {
		result[i].To = sortedUnique(result[i].To)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].From < result[j].From })
	return result
}

func sortedUnique(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	output := result[:0]
	for _, value := range result {
		if value != "" && (len(output) == 0 || output[len(output)-1] != value) {
			output = append(output, value)
		}
	}
	return output
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func edgeID(from, to string) string { return from + "->" + to }

func allowedDecision(value string) bool {
	return value == DecisionClosed || value == DecisionUnknown || value == DecisionRefuted
}

func unknown(stage, step, reason, class, next string, blocked ...string) UnknownRecord {
	return UnknownRecord{Stage: stage, Step: step, Reason: reason, UnknownClass: class, NextOperation: next, BlockedBy: sortedUnique(blocked)}
}

func operationPreserves(meta MetaContract, operations []MigrationOperation, predicate string) bool {
	if len(operations) == 0 {
		return false
	}
	for _, operation := range operations {
		decl, ok := meta.Operation(operation.Kind)
		if !ok {
			return false
		}
		found := false
		for _, preserved := range decl.Preserves {
			if preserved == predicate {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func stringsJoin(values []string) string { return strings.Join(values, ",") }
