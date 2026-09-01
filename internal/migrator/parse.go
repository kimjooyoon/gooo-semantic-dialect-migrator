package migrator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func LoadMeta(path string) (MetaContract, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return MetaContract{}, nil, err
	}
	meta, err := ParseMeta(raw)
	if err != nil {
		return MetaContract{}, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return meta, raw, nil
}

func ParseMeta(raw []byte) (MetaContract, error) {
	var meta MetaContract
	for lineNumber, rawLine := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(stripComment(rawLine))
		if line == "" {
			continue
		}
		tokens, err := tokenize(line)
		if err != nil {
			return MetaContract{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		if len(tokens) == 0 {
			continue
		}
		values := map[string]string{}
		if tokens[0] != "gooo" && tokens[0] != "authority" && tokens[0] != "precedence" && tokens[0] != "unknown_fields" {
			values, err = parseKeyValues(tokens[1:])
			if err != nil {
				return MetaContract{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
			}
		}
		switch tokens[0] {
		case "gooo":
			if len(tokens) != 3 || tokens[1] != "semantic_dialect_migrator" || tokens[2] != "v1" {
				return MetaContract{}, fmt.Errorf("line %d: invalid meta header", lineNumber+1)
			}
			meta.Schema = MetaSchema
		case "authority":
			if len(tokens) != 2 {
				return MetaContract{}, fmt.Errorf("line %d: invalid authority", lineNumber+1)
			}
			meta.Authority = tokens[1]
		case "denominator":
			meta.Denominator = DenominatorDecl{ID: values["id"], Cases: parseInt(values, "cases"), Unit: values["unit"]}
		case "authority_policy":
			meta.AuthorityPolicy = values
		case "source_policy":
			meta.SourcePolicy = values
		case "precedence":
			if len(tokens) != 2 {
				return MetaContract{}, fmt.Errorf("line %d: invalid precedence", lineNumber+1)
			}
			meta.Precedence = strings.Split(tokens[1], ">")
		case "unknown_fields":
			if len(tokens) != 2 {
				return MetaContract{}, fmt.Errorf("line %d: invalid unknown field declaration", lineNumber+1)
			}
			meta.UnknownFields = strings.Split(tokens[1], ",")
		case "dialects":
			if len(tokens) != 2 || values["versions"] == "" {
				return MetaContract{}, fmt.Errorf("line %d: invalid dialect declaration", lineNumber+1)
			}
			meta.Dialects = splitList(values["versions"])
		case "predicate":
			meta.Predicates = append(meta.Predicates, PredicateDecl{Ordinal: parseInt(values, "ordinal"), ID: values["id"], Kind: values["kind"], Preserve: parseBool(values, "preserve")})
		case "operation":
			meta.Operations = append(meta.Operations, OperationDecl{
				Ordinal: parseInt(values, "ordinal"), ID: values["id"], Kind: values["kind"],
				Reversible: parseBool(values, "reversible"), Lossy: parseBool(values, "lossy"),
				RequiresOrigin: parseBool(values, "requires_origin"), RequiresInverse: parseBool(values, "requires_inverse"),
				Preserves: splitList(values["preserves"]),
			})
		case "case":
			meta.Cases = append(meta.Cases, CaseDecl{Ordinal: parseInt(values, "ordinal"), ID: values["id"], Fixture: values["fixture"], Expected: values["expected"]})
		default:
			return MetaContract{}, fmt.Errorf("line %d: unsupported meta declaration %q", lineNumber+1, tokens[0])
		}
	}
	if err := meta.Validate(); err != nil {
		return MetaContract{}, err
	}
	return meta, nil
}

func (m MetaContract) Validate() error {
	if m.Schema != MetaSchema || m.Authority != "metacode" {
		return errors.New("meta source is not the authoritative migrator declaration")
	}
	if m.Denominator.ID == "" || m.Denominator.Cases <= 0 || m.Denominator.Unit == "" || len(m.Cases) != m.Denominator.Cases {
		return errors.New("meta denominator cardinality is invalid")
	}
	if len(m.Precedence) == 0 || len(m.UnknownFields) == 0 {
		return errors.New("meta precedence or UNKNOWN tuple is empty")
	}
	for _, key := range []string{"repository_writes", "local_test_executions", "local_build_executions", "local_vet_executions", "local_conformance_executions", "local_integration_executions", "cross_project_required_gates", "automatic_commit", "automatic_push", "automatic_merge", "automatic_release"} {
		if m.AuthorityPolicy[key] != "0" {
			return fmt.Errorf("authority policy %s must be zero", key)
		}
	}
	if m.SourcePolicy["input_repository_writes"] != "0" || m.SourcePolicy["outputs"] != "caller_owned_only" || m.SourcePolicy["overwrite_source"] != "never" {
		return errors.New("source write policy is not closed")
	}
	if len(m.Dialects) < 2 || len(m.Predicates) == 0 || len(m.Operations) == 0 {
		return errors.New("meta dialect, predicate, or operation declaration is empty")
	}
	caseIDs := map[string]bool{}
	for index, item := range m.Cases {
		if item.Ordinal != index+1 || item.ID == "" || caseIDs[item.ID] || item.Fixture == "" || !allowedDecision(item.Expected) {
			return fmt.Errorf("invalid fixed case at ordinal %d", index+1)
		}
		caseIDs[item.ID] = true
	}
	predicateIDs := map[string]bool{}
	for index, item := range m.Predicates {
		if item.Ordinal != index+1 || item.ID == "" || predicateIDs[item.ID] || !item.Preserve {
			return fmt.Errorf("invalid predicate at ordinal %d", index+1)
		}
		predicateIDs[item.ID] = true
	}
	operationIDs := map[string]bool{}
	for index, item := range m.Operations {
		if item.Ordinal != index+1 || item.ID == "" || operationIDs[item.ID] || item.Kind == "" || (item.Lossy && item.Reversible) {
			return fmt.Errorf("invalid operation at ordinal %d", index+1)
		}
		operationIDs[item.ID] = true
	}
	return nil
}

func LoadCase(path string, meta MetaContract) (MigrationCase, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return MigrationCase{}, nil, err
	}
	var item MigrationCase
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&item); err != nil {
		return MigrationCase{}, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := item.Validate(meta); err != nil {
		return MigrationCase{}, nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return item, raw, nil
}

func LoadProgram(path string) (Program, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Program{}, nil, err
	}
	program, err := ParseProgram(raw)
	if err != nil {
		return Program{}, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return program, raw, nil
}

func ParseProgram(raw []byte) (Program, error) {
	program := Program{Schema: ProgramSchema, Nodes: []Node{}, Edges: []Edge{}}
	graphSeen := false
	programSeen := false
	for lineNumber, rawLine := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(stripComment(rawLine))
		if line == "" {
			continue
		}
		tokens, err := tokenize(line)
		if err != nil {
			return Program{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		if len(tokens) == 0 {
			continue
		}
		values := map[string]string{}
		if tokens[0] != "gooo" {
			values, err = parseKeyValues(tokens[1:])
			if err != nil {
				return Program{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
			}
		}
		switch tokens[0] {
		case "gooo":
			if len(tokens) != 3 || tokens[1] != "program" || tokens[2] != "v1" {
				return Program{}, fmt.Errorf("line %d: invalid program header", lineNumber+1)
			}
			graphSeen = true
		case "program":
			if programSeen {
				return Program{}, fmt.Errorf("line %d: duplicate program declaration", lineNumber+1)
			}
			program.ID, program.Dialect, program.Entry = values["id"], values["dialect"], values["entry"]
			programSeen = true
		case "node":
			program.Nodes = append(program.Nodes, Node{ID: values["id"], Symbol: values["symbol"], Capabilities: splitList(values["capabilities"]), Effects: splitList(values["effects"])})
		case "edge":
			program.Edges = append(program.Edges, Edge{From: values["from"], To: values["to"]})
		case "terminal":
			program.TerminalReason = values["reason"]
		default:
			return Program{}, fmt.Errorf("line %d: unsupported program declaration %q", lineNumber+1, tokens[0])
		}
	}
	if !graphSeen || !programSeen {
		return Program{}, errors.New("program header or program declaration is missing")
	}
	program = program.canonical()
	if err := program.Validate(); err != nil {
		return Program{}, err
	}
	return program, nil
}

func parseKeyValues(tokens []string) (map[string]string, error) {
	values := map[string]string{}
	for _, token := range tokens {
		key, value, ok := strings.Cut(token, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid key/value token %q", token)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		values[key] = value
	}
	return values, nil
}

func tokenize(line string) ([]string, error) {
	var result []string
	var current strings.Builder
	inQuote := false
	escaped := false
	for _, r := range line {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && inQuote {
			escaped = true
			continue
		}
		if r == '"' {
			inQuote = !inQuote
			continue
		}
		if (r == ' ' || r == '\t') && !inQuote {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(r)
	}
	if escaped || inQuote {
		return nil, errors.New("unterminated quoted value")
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result, nil
}

func stripComment(value string) string {
	inQuote := false
	for index, r := range value {
		if r == '"' {
			inQuote = !inQuote
		}
		if !inQuote && (r == '#' || (r == '/' && index+1 < len(value) && value[index+1] == '/')) {
			return value[:index]
		}
	}
	return value
}

func splitList(value string) []string {
	if value == "" || value == "-" {
		return []string{}
	}
	return sortedUnique(strings.FieldsFunc(value, func(r rune) bool { return r == '|' || r == ',' }))
}

func parseInt(values map[string]string, key string) int {
	value, err := strconv.Atoi(values[key])
	if err != nil {
		return 0
	}
	return value
}

func parseBool(values map[string]string, key string) bool { return values[key] == "true" }
