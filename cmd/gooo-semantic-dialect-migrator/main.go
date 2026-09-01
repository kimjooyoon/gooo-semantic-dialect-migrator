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
	default:
		usage()
		os.Exit(2)
	}
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
	fmt.Fprintln(os.Stderr, "usage: gooo-semantic-dialect-migrator migrate|conformance [flags]")
}
