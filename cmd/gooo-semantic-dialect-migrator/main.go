package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kimjooyoon/gooo-semantic-dialect-migrator/internal/migrator"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "migrate":
		runMigrate(os.Args[2:])
	case "conformance":
		runConformance(os.Args[2:])
	case "annotate-metrics":
		runAnnotateMetrics(os.Args[2:])
	case "validate-metrics":
		runValidateMetrics(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func runAnnotateMetrics(args []string) {
	flags := flag.NewFlagSet("annotate-metrics", flag.ExitOnError)
	indexPath := flags.String("index", "", "conformance-index.json to annotate")
	observationsPath := flags.String("observations", "", "strict runner observation JSON")
	flags.Parse(args)
	if *indexPath == "" || *observationsPath == "" {
		fmt.Fprintln(os.Stderr, "annotate-metrics requires -index and -observations")
		os.Exit(2)
	}
	report, err := migrator.AnnotateMetrics(*indexPath, *observationsPath)
	if err != nil {
		fatal(err)
	}
	printJSON(report)
}

func runValidateMetrics(args []string) {
	flags := flag.NewFlagSet("validate-metrics", flag.ExitOnError)
	metricsPath := flags.String("metrics", "", "metrics.json to validate")
	flags.Parse(args)
	if *metricsPath == "" {
		fmt.Fprintln(os.Stderr, "validate-metrics requires -metrics")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*metricsPath)
	if err != nil {
		fatal(err)
	}
	if _, err := migrator.ParseMetrics(raw); err != nil {
		fatal(err)
	}
	fmt.Println("metrics_validation=CLOSED")
}

func runMigrate(args []string) {
	flags := flag.NewFlagSet("migrate", flag.ExitOnError)
	metaPath := flags.String("meta", ".gooo/migrator.gooo", "authoritative .gooo meta source")
	casePath := flags.String("case", "", "migration case JSON")
	root := flags.String("root", ".", "input repository root")
	out := flags.String("output-dir", "", "caller-owned output directory")
	flags.Parse(args)
	if *casePath == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "migrate requires -case and -output-dir")
		os.Exit(2)
	}
	meta, _, err := migrator.LoadMeta(resolve(*root, *metaPath))
	if err != nil {
		fatal(err)
	}
	report, err := migrator.EvaluateCase(meta, resolve(*root, *casePath), *root, *out)
	if err != nil {
		fatal(err)
	}
	printJSON(report)
}

func runConformance(args []string) {
	flags := flag.NewFlagSet("conformance", flag.ExitOnError)
	root := flags.String("root", ".", "input repository root")
	metaPath := flags.String("meta", ".gooo/migrator.gooo", "authoritative .gooo meta source")
	out := flags.String("output-dir", "", "caller-owned output directory")
	flags.Parse(args)
	if *out == "" {
		fmt.Fprintln(os.Stderr, "conformance requires -output-dir")
		os.Exit(2)
	}
	meta, raw, err := migrator.LoadMeta(resolve(*root, *metaPath))
	if err != nil {
		fatal(err)
	}
	report, err := migrator.RunConformance(meta, raw, *root, *out)
	if err != nil {
		fatal(err)
	}
	printJSON(report)
	if report.Decision != migrator.DecisionClosed {
		os.Exit(1)
	}
}

func resolve(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func printJSON(value any) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(raw))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: gooo-semantic-dialect-migrator migrate|conformance|annotate-metrics|validate-metrics [flags]")
}
