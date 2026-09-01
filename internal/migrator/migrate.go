package migrator

import (
	"errors"
	"fmt"
	"sort"
)

type MigrationResult struct {
	Program      Program
	OriginMap    []OriginPair
	Unknowns     []UnknownRecord
	Changed      bool
	Lossy        bool
	Reversible   bool
	UsedComments string
}

func ApplyMigration(meta MetaContract, source Program, item MigrationCase) (MigrationResult, error) {
	if err := source.Validate(); err != nil {
		return MigrationResult{}, err
	}
	if source.Dialect != item.SourceDialect {
		return MigrationResult{}, fmt.Errorf("source dialect %s does not match case declaration %s", source.Dialect, item.SourceDialect)
	}
	if _, ok := findString(meta.Dialects, item.TargetDialect); !ok {
		return MigrationResult{}, fmt.Errorf("target dialect %s is not declared by meta source", item.TargetDialect)
	}
	result := MigrationResult{Program: cloneProgram(source), OriginMap: []OriginPair{}, Unknowns: []UnknownRecord{}, Reversible: true}
	for index, operation := range item.Operations {
		declaration, ok := meta.Operation(operation.Kind)
		if !ok {
			return MigrationResult{}, fmt.Errorf("operation %q at index %d is not declared by meta source", operation.Kind, index)
		}
		if declaration.RequiresOrigin && len(operation.Origins) == 0 {
			result.Unknowns = append(result.Unknowns, unknown("migration", "resolve_origin", "typed operation has no explicit origin mapping", "MISSING_ORIGIN", "supply_origin_mapping", declaration.ID+".origins"))
		}
		if declaration.RequiresInverse && (operation.Inverse == nil || !*operation.Inverse) {
			result.Unknowns = append(result.Unknowns, unknown("migration", "resolve_inverse", "reversible typed operation has no confirmed inverse", "MISSING_INVERSE", "declare_inverse_operation", declaration.ID+".inverse"))
		}
		if operation.Lossy || declaration.Lossy {
			result.Lossy = true
			result.Unknowns = append(result.Unknowns, unknown("migration", "check_lossy_boundary", "capability migration crosses a declared lossy boundary", "LOSSY_CAPABILITY_MIGRATION", "obtain_lossless_capability_equivalence", declaration.ID+".lossy"))
		}
		if operation.Lossy && !declaration.Lossy {
			return MigrationResult{}, fmt.Errorf("operation %s marks an undeclared lossy boundary", declaration.ID)
		}
		if !declaration.Reversible {
			result.Reversible = false
		}
		if err := applyOperation(&result, operation); err != nil {
			return MigrationResult{}, fmt.Errorf("apply operation %s: %w", declaration.ID, err)
		}
		result.OriginMap = mergeOriginPairs(result.OriginMap, operation.originMap())
		if len(operation.Origins) > 0 {
			result.Changed = true
		}
	}
	result.Program.Dialect = item.TargetDialect
	if err := result.Program.Validate(); err != nil {
		return MigrationResult{}, err
	}
	result.OriginMap = normalizeOrigins(result.OriginMap)
	return result, nil
}

func applyOperation(result *MigrationResult, operation MigrationOperation) error {
	switch operation.Kind {
	case "NO_OP", "REPLAY":
		return nil
	case "COMMENT_ONLY":
		result.UsedComments = operation.Comment
		return nil
	case "RENAME_SYMBOL":
		return renameNode(&result.Program, operation.From, operation.To)
	case "SPLIT_NODE":
		return splitNode(&result.Program, operation)
	case "CAPABILITY_MIGRATION":
		return migrateCapability(&result.Program, operation)
	case "CHANGE_TERMINAL_REASON":
		if operation.Reason == "" {
			return errors.New("terminal reason operation has no reason")
		}
		result.Program.TerminalReason = operation.Reason
		return nil
	default:
		return fmt.Errorf("unsupported operation kind %q", operation.Kind)
	}
}

func renameNode(program *Program, from, to string) error {
	if from == "" || to == "" || from == to {
		return errors.New("rename requires distinct from and to node IDs")
	}
	index := -1
	for i, node := range program.Nodes {
		if node.ID == from {
			index = i
		}
		if node.ID == to {
			return fmt.Errorf("rename target %s already exists", to)
		}
	}
	if index < 0 {
		return fmt.Errorf("rename source %s does not exist", from)
	}
	program.Nodes[index].ID = to
	program.Nodes[index].Symbol = to
	if program.Entry == from {
		program.Entry = to
	}
	for i := range program.Edges {
		if program.Edges[i].From == from {
			program.Edges[i].From = to
		}
		if program.Edges[i].To == from {
			program.Edges[i].To = to
		}
	}
	return nil
}

func splitNode(program *Program, operation MigrationOperation) error {
	if operation.From == "" || len(operation.Into) < 2 {
		return errors.New("split requires a source and at least two target IDs")
	}
	index := -1
	var original Node
	for i, node := range program.Nodes {
		if node.ID == operation.From {
			index = i
			original = node
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("split source %s does not exist", operation.From)
	}
	parts := make([]Node, 0, len(operation.Into))
	for partIndex, id := range operation.Into {
		if id == "" {
			return errors.New("split target ID is empty")
		}
		for _, existing := range program.Nodes {
			if existing.ID == id && existing.ID != operation.From {
				return fmt.Errorf("split target %s already exists", id)
			}
		}
		part := Node{ID: id, Symbol: id, Capabilities: append([]string(nil), original.Capabilities...), Effects: []string{}}
		if partIndex == len(operation.Into)-1 {
			part.Effects = append(part.Effects, original.Effects...)
		}
		for _, declared := range operation.Parts {
			if declared.ID == id {
				part.Symbol = declared.Symbol
				if part.Symbol == "" {
					part.Symbol = id
				}
				part.Capabilities = append([]string(nil), declared.Capabilities...)
				part.Effects = append([]string(nil), declared.Effects...)
			}
		}
		parts = append(parts, part)
	}
	for i, edge := range program.Edges {
		if edge.From == operation.From {
			program.Edges[i].From = operation.Into[len(operation.Into)-1]
		}
		if edge.To == operation.From {
			program.Edges[i].To = operation.Into[0]
		}
	}
	if program.Entry == operation.From {
		program.Entry = operation.Into[0]
	}
	program.Nodes = append(append(append([]Node{}, program.Nodes[:index]...), parts...), program.Nodes[index+1:]...)
	for i := 0; i < len(operation.Into)-1; i++ {
		program.Edges = append(program.Edges, Edge{From: operation.Into[i], To: operation.Into[i+1]})
	}
	return nil
}

func migrateCapability(program *Program, operation MigrationOperation) error {
	if operation.From == "" || operation.FromCapability == "" || operation.ToCapability == "" {
		return errors.New("capability migration requires node, from capability, and to capability")
	}
	for i := range program.Nodes {
		if program.Nodes[i].ID != operation.From {
			continue
		}
		found := false
		for index, capability := range program.Nodes[i].Capabilities {
			if capability == operation.FromCapability {
				program.Nodes[i].Capabilities[index] = operation.ToCapability
				found = true
			}
		}
		if !found {
			return fmt.Errorf("capability %s is not granted by node %s", operation.FromCapability, operation.From)
		}
		return nil
	}
	return fmt.Errorf("capability migration node %s does not exist", operation.From)
}

func mergeOriginPairs(existing, added []OriginPair) []OriginPair {
	values := map[string][]string{}
	for _, pair := range existing {
		values[pair.From] = append(values[pair.From], pair.To...)
	}
	for _, pair := range added {
		values[pair.From] = append(values[pair.From], pair.To...)
	}
	result := make([]OriginPair, 0, len(values))
	for from, to := range values {
		result = append(result, OriginPair{From: from, To: sortedUnique(to)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].From < result[j].From })
	return result
}

func validateOriginMap(source Program, target Program, pairs []OriginPair) error {
	sourceIDs := map[string]bool{}
	targetIDs := map[string]bool{}
	for _, node := range source.Nodes {
		sourceIDs[node.ID] = true
	}
	for _, node := range target.Nodes {
		targetIDs[node.ID] = true
	}
	seen := map[string]bool{}
	for _, pair := range pairs {
		if !sourceIDs[pair.From] || seen[pair.From] || len(pair.To) == 0 {
			return errors.New("origin map is missing, duplicate, or references an unknown source")
		}
		seen[pair.From] = true
		for _, targetID := range pair.To {
			if !targetIDs[targetID] {
				return errors.New("origin map references an unknown target")
			}
		}
	}
	for sourceID := range sourceIDs {
		if !seen[sourceID] {
			return fmt.Errorf("origin map has no entry for %s", sourceID)
		}
	}
	return nil
}

func findString(values []string, wanted string) (string, bool) {
	for _, value := range values {
		if value == wanted {
			return value, true
		}
	}
	return "", false
}

func cloneProgram(source Program) Program {
	result := source
	result.Nodes = make([]Node, len(source.Nodes))
	for index, node := range source.Nodes {
		result.Nodes[index] = node
		result.Nodes[index].Capabilities = append([]string(nil), node.Capabilities...)
		result.Nodes[index].Effects = append([]string(nil), node.Effects...)
	}
	result.Edges = append([]Edge(nil), source.Edges...)
	return result
}
