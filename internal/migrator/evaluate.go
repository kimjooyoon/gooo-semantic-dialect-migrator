package migrator

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

func EvaluateCase(meta MetaContract, casePath, root, outputDir string) (CaseReport, error) {
	if err := ensureOutputOutsideRoot(root, outputDir); err != nil {
		return CaseReport{}, err
	}
	caseInput, _, err := LoadCase(casePath, meta)
	if err != nil {
		return CaseReport{}, err
	}
	sourcePath := caseInput.SourcePath
	if !filepath.IsAbs(sourcePath) {
		sourcePath = filepath.Join(root, sourcePath)
	}
	if !isWithinRoot(root, sourcePath) {
		return CaseReport{}, errors.New("source path escapes repository root")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return CaseReport{}, err
	}

	stage := stageRecorder{}
	source, sourceRaw, err := timedLoadProgram(sourcePath, &stage, "parse")
	if err != nil {
		return CaseReport{}, err
	}
	sourceIR, err := timedLower(source, sourcePath, sourceRaw, &stage)
	if err != nil {
		return CaseReport{}, err
	}
	result, err := timedMigrate(meta, source, caseInput, &stage)
	if err != nil {
		return CaseReport{}, err
	}

	migratedRaw, err := RenderProgram(result.Program, result.UsedComments)
	if err != nil {
		return CaseReport{}, err
	}
	migratedPath := filepath.Join(outputDir, "migrated.gooo")
	if err := os.WriteFile(migratedPath, migratedRaw, 0o644); err != nil {
		return CaseReport{}, err
	}
	target, targetRaw, err := timedLoadProgram(migratedPath, &stage, "parse_target")
	if err != nil {
		return CaseReport{}, err
	}
	targetIR, err := timedLower(target, migratedPath, targetRaw, &stage)
	if err != nil {
		return CaseReport{}, err
	}
	executeStart := time.Now()
	if _, err := Execute(source); err != nil {
		return CaseReport{}, err
	}
	if _, err := Execute(target); err != nil {
		return CaseReport{}, err
	}
	stage.record("execute", executeStart)

	report := CaseReport{
		Schema: ReportSchema, CaseID: caseInput.CaseID, ExpectedDecision: caseInput.ExpectedDecision,
		SourcePath: sourcePath, MigratedSourcePath: migratedPath, SourceIR: sourceIR, TargetIR: targetIR,
		OriginMap: normalizeOrigins(result.OriginMap), Operations: caseInput.Operations,
		Unknowns: append([]UnknownRecord(nil), result.Unknowns...), RepositoryWrites: 0,
	}
	verifyStart := time.Now()
	report.Replay = verifyReplay(sourceIR, targetIR, caseInput.Replay, &report.Unknowns)
	report.PredicateVector = verifyPredicates(meta, sourceIR, targetIR, report.OriginMap, result, caseInput.Operations, report.Replay)
	stage.record("verify", verifyStart)
	report.Decision = decide(meta, report.PredicateVector, report.Unknowns, result)
	for index := range report.Unknowns {
		if err := report.Unknowns[index].Validate(); err != nil {
			return CaseReport{}, err
		}
	}
	if report.Decision == DecisionRefuted {
		// The precedence is part of the metacode contract. A refutation is
		// terminal even when uncertainty evidence is also retained for audit.
	} else if report.Decision == DecisionUnknown && len(report.Unknowns) == 0 {
		report.Unknowns = append(report.Unknowns, unknown("verify", "record_predicate_vector", "a declared preservation predicate was not established", "PREDICATE_INCOMPLETE", "supply_exact_preservation_evidence", "predicate-vector"))
	}

	renderStart := time.Now()
	generatedFiles, generatedBytes, err := writeArtifacts(outputDir, sourceIR, targetIR)
	if err != nil {
		return CaseReport{}, err
	}
	generatedFiles = append(generatedFiles, "migrated.gooo")
	generatedBytes += len(migratedRaw)
	stage.record("render", renderStart)
	report.GeneratedFiles, report.GeneratedBytes = generatedFiles, generatedBytes
	report.StageMetrics = stage.finish()
	report = normalizeReport(report)
	if err := writeJSON(filepath.Join(outputDir, "case-report.json"), report); err != nil {
		return CaseReport{}, err
	}
	return report, nil
}

func verifyPredicates(meta MetaContract, source, target SemanticIR, origins []OriginPair, result MigrationResult, operations []MigrationOperation, replay []ReplayEvidence) []PredicateEvidence {
	canonicalEqual := source.CanonicalDigest() == target.CanonicalDigest()
	originEqual := validateOriginMapFromIR(source, target, origins) == nil
	reversible := result.Reversible && !result.Lossy
	replayEqual := len(replay) > 0
	for _, item := range replay {
		if !item.Exact {
			replayEqual = false
		}
	}
	values := make([]PredicateEvidence, 0, len(meta.Predicates))
	for _, predicate := range meta.Predicates {
		observed := false
		detail := ""
		switch predicate.ID {
		case "terminal_reason":
			observed = source.TerminalReason == target.TerminalReason
			detail = "source and migrated terminal reasons are byte-equal"
		case "effect_trace":
			observed = sameStrings(source.EffectTrace, target.EffectTrace)
			detail = "source and migrated effect traces are sequence-equal"
		case "capability_set":
			observed = sameStrings(source.CapabilitySet, target.CapabilitySet)
			detail = "source and migrated capability sets are sorted-set-equal"
		case "origin_map":
			observed = originEqual
			detail = "every source node has an explicit target origin"
		case "reversible_boundary":
			observed = reversible
			detail = "all applied operations remain outside a lossy boundary and have inverse evidence"
		case "canonical_ir":
			observed = canonicalEqual
			detail = "semantic canonical IR digests are equal"
		case "replay":
			observed = replayEqual
			detail = "each declared replay input matches exact terminal reason and effect trace"
		}
		beforeDigest := predicateDigest(predicate.ID, source, origins, replay)
		afterDigest := predicateDigest(predicate.ID, target, origins, replay)
		if predicate.ID == "reversible_boundary" {
			beforeDigest = DigestValue(true)
			afterDigest = DigestValue(reversible)
		}
		values = append(values, PredicateEvidence{ID: predicate.ID, DeclaredPreserve: operationPreserves(meta, operations, predicate.ID), ObservedPreserve: observed, BeforeDigest: beforeDigest, AfterDigest: afterDigest, Detail: detail})
	}
	return values
}

func predicateDigest(id string, ir SemanticIR, origins []OriginPair, replay []ReplayEvidence) string {
	switch id {
	case "terminal_reason":
		return DigestValue(ir.TerminalReason)
	case "effect_trace":
		return DigestValue(ir.EffectTrace)
	case "capability_set":
		return DigestValue(ir.CapabilitySet)
	case "origin_map":
		return DigestValue(normalizeOrigins(origins))
	case "canonical_ir":
		return ir.CanonicalDigest()
	case "replay":
		return DigestValue(replay)
	default:
		return DigestValue(id)
	}
}

func decide(meta MetaContract, predicates []PredicateEvidence, unknowns []UnknownRecord, result MigrationResult) string {
	refuted := false
	for _, item := range predicates {
		if !item.ObservedPreserve && (item.ID == "terminal_reason" || item.ID == "effect_trace" || (item.ID == "capability_set" && !result.Lossy)) {
			refuted = true
		}
	}
	unknown := len(unknowns) > 0
	if !unknown {
		for _, item := range predicates {
			if item.DeclaredPreserve && !item.ObservedPreserve {
				unknown = true
				break
			}
		}
	}
	decision := DecisionClosed
	for _, candidate := range meta.Precedence {
		switch candidate {
		case DecisionRefuted:
			if refuted {
				return DecisionRefuted
			}
		case DecisionUnknown:
			if unknown {
				decision = DecisionUnknown
			}
		case DecisionClosed:
			if !refuted && !unknown {
				decision = DecisionClosed
			}
		}
	}
	return decision
}

func verifyReplay(source, target SemanticIR, inputs []ReplayInput, unknowns *[]UnknownRecord) []ReplayEvidence {
	if len(inputs) == 0 {
		*unknowns = append(*unknowns, unknown("execute", "replay", "no replay input was declared", "MISSING_REPLAY", "declare_replay_input", "replay"))
		return []ReplayEvidence{}
	}
	result := make([]ReplayEvidence, 0, len(inputs))
	seen := map[string]bool{}
	for _, input := range inputs {
		if input.InputID == "" || seen[input.InputID] {
			*unknowns = append(*unknowns, unknown("execute", "replay", "replay input identity is missing or duplicated", "AMBIGUOUS_REPLAY", "declare_unique_replay_input", "replay."+input.InputID))
			continue
		}
		seen[input.InputID] = true
		exact := source.TerminalReason == input.ExpectedReason && sameStrings(source.EffectTrace, input.ExpectedEffects) && target.TerminalReason == input.ExpectedReason && sameStrings(target.EffectTrace, input.ExpectedEffects)
		result = append(result, ReplayEvidence{InputID: input.InputID, SourceReason: source.TerminalReason, TargetReason: target.TerminalReason, SourceEffects: source.EffectTrace, TargetEffects: target.EffectTrace, ExpectedReason: input.ExpectedReason, ExpectedEffects: input.ExpectedEffects, Exact: exact})
		if !exact {
			*unknowns = append(*unknowns, unknown("execute", "replay", "replay input does not match both terminal traces exactly", "REPLAY_MISMATCH", "supply_matching_replay_or_refute_semantics", "replay."+input.InputID))
		}
	}
	return result
}

func validateOriginMapFromIR(source, target SemanticIR, origins []OriginPair) error {
	sp := Program{Schema: ProgramSchema, ID: source.ProgramID, Dialect: source.Dialect, Entry: source.Entry, Nodes: source.Nodes, Edges: source.Edges, TerminalReason: source.TerminalReason}
	tp := Program{Schema: ProgramSchema, ID: target.ProgramID, Dialect: target.Dialect, Entry: target.Entry, Nodes: target.Nodes, Edges: target.Edges, TerminalReason: target.TerminalReason}
	return validateOriginMap(sp, tp, origins)
}

func writeArtifacts(outputDir string, sourceIR, targetIR SemanticIR) ([]string, int, error) {
	artifacts := map[string][]byte{}
	var err error
	artifacts["source.semantic-ir.json"], err = RenderIR(sourceIR)
	if err != nil {
		return nil, 0, err
	}
	artifacts["target.semantic-ir.json"], err = RenderIR(targetIR)
	if err != nil {
		return nil, 0, err
	}
	artifacts["source.gooo.go"], err = RenderBinding(sourceIR, "generated")
	if err != nil {
		return nil, 0, err
	}
	artifacts["target.gooo.go"], err = RenderBinding(targetIR, "generated")
	if err != nil {
		return nil, 0, err
	}
	keys := make([]string, 0, len(artifacts))
	for key := range artifacts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	bytesWritten := 0
	for _, key := range keys {
		if err := os.WriteFile(filepath.Join(outputDir, key), artifacts[key], 0o644); err != nil {
			return nil, 0, err
		}
		bytesWritten += len(artifacts[key])
	}
	return keys, bytesWritten, nil
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func normalizeReport(report CaseReport) CaseReport {
	report.OriginMap = normalizeOrigins(report.OriginMap)
	sort.Slice(report.GeneratedFiles, func(i, j int) bool { return report.GeneratedFiles[i] < report.GeneratedFiles[j] })
	return report
}

type stageRecorder struct {
	values []StageMetric
}

func (s *stageRecorder) record(stage string, start time.Time) {
	usage := syscall.Rusage{}
	peak := 0
	if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) == nil {
		peak = maxRSSKiB(usage.Maxrss)
	}
	s.values = append(s.values, StageMetric{Stage: stage, WallMS: int(time.Since(start).Milliseconds()), PeakRSSKiB: peak})
}

func (s *stageRecorder) finish() []StageMetric {
	result := append([]StageMetric(nil), s.values...)
	return result
}

func timedLoadProgram(path string, stage *stageRecorder, name string) (Program, []byte, error) {
	start := time.Now()
	program, raw, err := LoadProgram(path)
	stage.record(name, start)
	return program, raw, err
}

func timedLower(program Program, path string, raw []byte, stage *stageRecorder) (SemanticIR, error) {
	start := time.Now()
	ir, err := Lower(program, path, raw)
	stage.record("lower", start)
	return ir, err
}

func timedMigrate(meta MetaContract, program Program, item MigrationCase, stage *stageRecorder) (MigrationResult, error) {
	start := time.Now()
	result, err := ApplyMigration(meta, program, item)
	stage.record("migrate", start)
	return result, err
}

func ensureOutputOutsideRoot(root, output string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	outputAbs, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if isWithinRoot(rootAbs, outputAbs) {
		return errors.New("output directory must be caller-owned and outside the input repository")
	}
	return nil
}

func isWithinRoot(root, path string) bool {
	rootAbs, err1 := filepath.Abs(root)
	pathAbs, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func peakRSSKiB() int {
	usage := syscall.Rusage{}
	if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) != nil {
		return 0
	}
	return maxRSSKiB(usage.Maxrss)
}

func maxRSSKiB(value int64) int {
	if runtime.GOOS == "darwin" {
		return int(value / 1024)
	}
	return int(value)
}
