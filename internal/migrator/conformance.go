package migrator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func RunConformance(meta MetaContract, metaRaw []byte, root, outputDir string) (ConformanceReport, error) {
	if err := ensureOutputOutsideRoot(root, outputDir); err != nil {
		return ConformanceReport{}, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return ConformanceReport{}, err
	}
	start := time.Now()
	result := ConformanceReport{Schema: ConformanceSchema, MetaDigest: DigestBytes(metaRaw), ExpectedPrecedence: append([]string(nil), meta.Precedence...), Cases: []ConformanceCase{}, FixedCaseVector: []string{}}
	metrics := Metrics{Schema: MetricsSchema, Cases: len(meta.Cases), Tests: TestMetrics{Total: len(meta.Cases), Selected: len(meta.Cases), Executed: 0, Reused: 0, Failed: 0, Unknown: 0}, RepositoryWrites: 0}
	sourcePaths := map[string]bool{}
	for _, declaration := range meta.Cases {
		casePath := filepath.Join(root, declaration.Fixture)
		caseOutput := filepath.Join(outputDir, "cases", declaration.ID)
		report, err := EvaluateCase(meta, casePath, root, caseOutput)
		if err != nil {
			return ConformanceReport{}, fmt.Errorf("case %s: %w", declaration.ID, err)
		}
		pass := report.Decision == declaration.Expected && report.ExpectedDecision == declaration.Expected
		item := ConformanceCase{Ordinal: declaration.Ordinal, CaseID: declaration.ID, Expected: declaration.Expected, Observed: report.Decision, Pass: pass, ReplayDigest: DigestValue(report.Replay), PredicateVector: report.PredicateVector, Unknowns: report.Unknowns}
		result.Cases = append(result.Cases, item)
		result.FixedCaseVector = append(result.FixedCaseVector, declaration.ID+":"+declaration.Expected+":"+report.Decision)
		metrics.Tests.Executed++
		sourcePaths[report.SourcePath] = true
		metrics.IRFiles += 2
		metrics.GeneratedFiles += len(report.GeneratedFiles)
		metrics.GeneratedBytes += report.GeneratedBytes
		if !pass {
			metrics.Tests.Failed++
		}
		switch report.Decision {
		case DecisionClosed:
			metrics.Closed++
		case DecisionUnknown:
			metrics.Unknown++
			metrics.Tests.Unknown++
		case DecisionRefuted:
			metrics.Refuted++
		}
		metrics.StageMetrics = append(metrics.StageMetrics, report.StageMetrics...)
	}
	result.ReplayDigest = conformanceReplayDigest(result.Cases)
	metrics.SourceFiles = len(sourcePaths)
	metrics.Inventory = InventoryFor(root)
	metrics.ApplyRunnerObservations(RunnerObservations{ConformanceWallMS: int(time.Since(start).Milliseconds()), ConformancePeakRSSKiB: peakRSSKiB()})
	metrics.StageMetrics = append(metrics.StageMetrics, StageMetric{Stage: "conformance", WallMS: int(time.Since(start).Milliseconds()), PeakRSSKiB: peakRSSKiB()})
	result.Metrics = metrics
	result.Decision = conformanceDecision(result.Cases)
	if err := writeJSON(filepath.Join(outputDir, "conformance-index.json"), result); err != nil {
		return ConformanceReport{}, err
	}
	if err := writeJSON(filepath.Join(outputDir, "metrics.json"), result.Metrics); err != nil {
		return ConformanceReport{}, err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "ci-summary.md"), []byte(RenderSummary(result)), 0o644); err != nil {
		return ConformanceReport{}, err
	}
	return result, nil
}

func AnnotateMetrics(indexPath, observationsPath string) (ConformanceReport, error) {
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		return ConformanceReport{}, err
	}
	var report ConformanceReport
	decoder := json.NewDecoder(bytes.NewReader(indexRaw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return ConformanceReport{}, fmt.Errorf("parse conformance index: %w", err)
	}
	if report.Schema != ConformanceSchema || report.ReplayDigest == "" || report.ReplayDigest != conformanceReplayDigest(report.Cases) {
		return ConformanceReport{}, errors.New("conformance replay digest is missing or inconsistent")
	}
	observationsRaw, err := os.ReadFile(observationsPath)
	if err != nil {
		return ConformanceReport{}, err
	}
	observations, err := ParseRunnerObservations(observationsRaw)
	if err != nil {
		return ConformanceReport{}, err
	}
	beforeReplayDigest := report.ReplayDigest
	report.Metrics.ApplyRunnerObservations(observations)
	if beforeReplayDigest != conformanceReplayDigest(report.Cases) {
		return ConformanceReport{}, errors.New("runner metric annotation changed replay digest")
	}
	metricsRaw, err := json.MarshalIndent(report.Metrics, "", "  ")
	if err != nil {
		return ConformanceReport{}, err
	}
	if _, err := ParseMetrics(metricsRaw); err != nil {
		return ConformanceReport{}, err
	}
	if err := writeJSON(indexPath, report); err != nil {
		return ConformanceReport{}, err
	}
	if err := writeJSON(filepath.Join(filepath.Dir(indexPath), "metrics.json"), report.Metrics); err != nil {
		return ConformanceReport{}, err
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(indexPath), "ci-summary.md"), []byte(RenderSummary(report)), 0o644); err != nil {
		return ConformanceReport{}, err
	}
	return report, nil
}

func conformanceReplayDigest(cases []ConformanceCase) string {
	digests := make([]string, len(cases))
	for index, item := range cases {
		digests[index] = item.CaseID + ":" + item.ReplayDigest
	}
	return DigestValue(digests)
}

func conformanceDecision(cases []ConformanceCase) string {
	for _, item := range cases {
		if !item.Pass {
			return DecisionRefuted
		}
	}
	return DecisionClosed
}

func InventoryFor(root string) Inventory {
	result := Inventory{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() && path != root && (info.Name() == ".git" || info.Name() == "_artifact" || info.Name() == "dist") {
			return filepath.SkipDir
		}
		if path == filepath.Join(root, "README.md") {
			return nil
		}
		if strings.Contains(filepath.Clean(path), string(filepath.Separator)+".git"+string(filepath.Separator)) {
			return nil
		}
		if info.IsDir() {
			if path != root {
				result.DescendantDirs++
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		result.RegularFiles++
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".go" {
			result.GoFiles++
			result.GoPhysicalLines += physicalLines(path)
		}
		if ext == ".gooo" {
			result.GoooFiles++
			result.GoooPhysicalLines += physicalLines(path)
		}
		return nil
	})
	return result
}

func physicalLines(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return 0
	}
	count := strings.Count(string(raw), "\n")
	if raw[len(raw)-1] != '\n' {
		count++
	}
	return count
}

func RenderSummary(report ConformanceReport) string {
	var out strings.Builder
	out.WriteString("# gooo semantic dialect migrator CI summary\n\n")
	out.WriteString("The fixed-case vector is recorded exactly; no score, weighting, average, or percentage is derived.\n\n")
	out.WriteString("| ordinal | case | expected | observed | pass |\n|---:|---|---|---|---|\n")
	for _, item := range report.Cases {
		out.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %t |\n", item.Ordinal, item.CaseID, item.Expected, item.Observed, item.Pass))
	}
	out.WriteString("\n## Exact integer observations\n\n")
	out.WriteString(fmt.Sprintf("- cases: %d\n- closed: %d\n- unknown: %d\n- refuted: %d\n- source_files: %d\n- ir_files: %d\n- generated_files: %d\n- generated_bytes: %d\n- descendant_dirs: %d\n- regular_files: %d\n- go_files: %d\n- go_physical_lines: %d\n- gooo_files: %d\n- gooo_physical_lines: %d\n- tests_total: %d\n- tests_selected: %d\n- tests_executed: %d\n- tests_reused: %d\n- tests_failed: %d\n- tests_unknown: %d\n- repository_writes: %d\n- compile_wall_ms: %d\n- compile_peak_rss_kib: %d\n- build_wall_ms: %d\n- build_peak_rss_kib: %d\n- test_wall_ms: %d\n- test_peak_rss_kib: %d\n- conformance_wall_ms: %d\n- conformance_peak_rss_kib: %d\n- integration_wall_ms: %d\n- integration_peak_rss_kib: %d\n- local_test_executions: %d\n- local_build_executions: %d\n- local_vet_executions: %d\n- local_conformance_executions: %d\n- local_integration_executions: %d\n\n", report.Metrics.Cases, report.Metrics.Closed, report.Metrics.Unknown, report.Metrics.Refuted, report.Metrics.SourceFiles, report.Metrics.IRFiles, report.Metrics.GeneratedFiles, report.Metrics.GeneratedBytes, report.Metrics.Inventory.DescendantDirs, report.Metrics.Inventory.RegularFiles, report.Metrics.Inventory.GoFiles, report.Metrics.Inventory.GoPhysicalLines, report.Metrics.Inventory.GoooFiles, report.Metrics.Inventory.GoooPhysicalLines, report.Metrics.Tests.Total, report.Metrics.Tests.Selected, report.Metrics.Tests.Executed, report.Metrics.Tests.Reused, report.Metrics.Tests.Failed, report.Metrics.Tests.Unknown, report.Metrics.RepositoryWrites, report.Metrics.CompileWallMS, report.Metrics.CompilePeakRSSKiB, report.Metrics.BuildWallMS, report.Metrics.BuildPeakRSSKiB, report.Metrics.TestWallMS, report.Metrics.TestPeakRSSKiB, report.Metrics.ConformanceWallMS, report.Metrics.ConformancePeakRSSKiB, report.Metrics.IntegrationWallMS, report.Metrics.IntegrationPeakRSSKiB, report.Metrics.LocalTestExecutions, report.Metrics.LocalBuildExecutions, report.Metrics.LocalVetExecutions, report.Metrics.LocalConformanceExecutions, report.Metrics.LocalIntegrationExecutions))
	out.WriteString("## Stage metrics\n\n| stage | wall_ms | peak_rss_kib |\n|---|---:|---:|\n")
	metrics := append([]StageMetric(nil), report.Metrics.StageMetrics...)
	sort.SliceStable(metrics, func(i, j int) bool { return metrics[i].Stage < metrics[j].Stage })
	for _, item := range metrics {
		out.WriteString(fmt.Sprintf("| %s | %d | %d |\n", item.Stage, item.WallMS, item.PeakRSSKiB))
	}
	out.WriteString("\n## Fixed-case vector\n\n")
	for _, item := range report.FixedCaseVector {
		out.WriteString("- " + item + "\n")
	}
	return out.String()
}
