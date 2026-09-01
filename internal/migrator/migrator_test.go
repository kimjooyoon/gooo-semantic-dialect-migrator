package migrator

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMetaOwnsFixedProtocol(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".gooo", "migrator.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ParseMeta(raw)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Denominator.Cases != 8 || len(meta.Cases) != 8 || meta.Precedence[0] != DecisionRefuted {
		t.Fatalf("unexpected fixed protocol: %#v", meta)
	}
}

func TestRenamePreservesExecutionWithExplicitOrigin(t *testing.T) {
	metaRaw, err := os.ReadFile(filepath.Join("..", "..", ".gooo", "migrator.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ParseMeta(metaRaw)
	if err != nil {
		t.Fatal(err)
	}
	programRaw, err := os.ReadFile(filepath.Join("..", "..", "examples", "v1", "main.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	program, err := ParseProgram(programRaw)
	if err != nil {
		t.Fatal(err)
	}
	caseRaw, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "cases", "symbol-rename-origin.json"))
	if err != nil {
		t.Fatal(err)
	}
	caseInput, _, err := decodeCase(caseRaw, meta)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyMigration(meta, program, caseInput)
	if err != nil {
		t.Fatal(err)
	}
	before, err := Execute(program)
	if err != nil {
		t.Fatal(err)
	}
	after, err := Execute(result.Program)
	if err != nil {
		t.Fatal(err)
	}
	if before.TerminalReason != after.TerminalReason || !sameStrings(before.EffectTrace, after.EffectTrace) {
		t.Fatalf("execution changed: before=%#v after=%#v", before, after)
	}
}

func TestPrecedenceRefutesSemanticChange(t *testing.T) {
	result := MigrationResult{}
	decision := decide(MetaContract{Precedence: []string{DecisionRefuted, DecisionUnknown, DecisionClosed}}, []PredicateEvidence{{ID: "terminal_reason", ObservedPreserve: false}}, []UnknownRecord{{Stage: "migration", Step: "x", Reason: "y", UnknownClass: "z", NextOperation: "n", BlockedBy: []string{"b"}}}, result)
	if decision != DecisionRefuted {
		t.Fatalf("decision=%s", decision)
	}
}

func TestRunnerObservationsRejectNullAndString(t *testing.T) {
	values := map[string]any{}
	for _, field := range runnerMetricFields {
		values[field] = 1
	}
	values["local_test_executions"] = 0
	values["local_build_executions"] = 0
	values["local_vet_executions"] = 0
	values["local_conformance_executions"] = 0
	values["local_integration_executions"] = 0
	raw, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRunnerObservations(raw); err == nil {
		t.Fatal("non-zero local authority fields must be rejected")
	}
	values["compile_wall_ms"] = nil
	values["build_wall_ms"] = 0
	values["test_wall_ms"] = 0
	values["conformance_wall_ms"] = 0
	values["integration_wall_ms"] = 0
	raw, err = json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRunnerObservations(raw); err == nil {
		t.Fatal("null runner field must be rejected")
	}
	values["compile_wall_ms"] = "0"
	raw, err = json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRunnerObservations(raw); err == nil {
		t.Fatal("string runner field must be rejected")
	}
}

func decodeCase(raw []byte, meta MetaContract) (MigrationCase, []byte, error) {
	path := filepath.Join("..", "..", "fixtures", "cases", "symbol-rename-origin.json")
	_ = path
	var result MigrationCase
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return MigrationCase{}, nil, err
	}
	if err := result.Validate(meta); err != nil {
		return MigrationCase{}, nil, err
	}
	return result, raw, nil
}
