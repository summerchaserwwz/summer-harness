package continuity

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type legacyDecision struct {
	Schema    string   `json:"schema"`
	Kind      string   `json:"kind"`
	ID        string   `json:"id"`
	TaskID    string   `json:"task_id"`
	Title     string   `json:"title"`
	Question  string   `json:"question"`
	Chosen    string   `json:"chosen"`
	Rejected  []string `json:"rejected"`
	WhyNot    []string `json:"why_not"`
	Source    string   `json:"source"`
	CreatedAt string   `json:"created_at"`
	CreatedNS int64    `json:"created_ns"`
	CreatedBy string   `json:"created_by"`
}

type legacyFact struct {
	Schema      string   `json:"schema"`
	Kind        string   `json:"kind"`
	ID          string   `json:"id"`
	TaskID      string   `json:"task_id"`
	Statement   string   `json:"statement"`
	Source      string   `json:"source"`
	Confidence  string   `json:"confidence"`
	Tags        []string `json:"tags"`
	MemoryClass string   `json:"memory_class"`
	ObservedAt  string   `json:"observed_at"`
	CreatedNS   int64    `json:"created_ns"`
	Session     string   `json:"session"`
}

type legacyFactInvalidation struct {
	Schema      string `json:"schema"`
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	Invalidates string `json:"invalidates"`
	Reason      string `json:"reason"`
	ObservedAt  string `json:"observed_at"`
	CreatedNS   int64  `json:"created_ns"`
	Session     string `json:"session"`
}

func (m *Module) readLegacyMemory(taskID string) ([]map[string]any, []map[string]any, error) {
	decisions, err := m.readLegacyDecisions(taskID)
	if err != nil {
		return nil, nil, err
	}
	facts, err := m.readLegacyFacts(taskID)
	if err != nil {
		return nil, nil, err
	}
	return decisions, facts, nil
}

func (m *Module) readLegacyDecisions(taskID string) ([]map[string]any, error) {
	directory := filepath.Join(m.root, ".agent", "ledger", "decisions")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []map[string]any{}, nil
	}
	if err != nil {
		return nil, wrap(CodeHandoffInvalid, "read decisions", ".agent/ledger/decisions", err)
	}
	if len(entries) > 10_000 {
		return nil, wrap(CodeHandoffInvalid, "read decisions", ".agent/ledger/decisions", errors.New("decision directory exceeds entry limit"))
	}
	values := make([]legacyDecision, 0, len(entries))
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil, wrap(CodeUnsafeReference, "read decisions", entry.Name(), errors.New("decision entry is not a regular file"))
		}
		raw, err := readRegularFile(filepath.Join(directory, entry.Name()), 128<<10)
		if err != nil {
			return nil, wrap(CodeHandoffInvalid, "read decision", entry.Name(), err)
		}
		metaRaw, err := splitMarkdown(raw)
		if err != nil {
			return nil, wrap(CodeHandoffInvalid, "parse decision", entry.Name(), err)
		}
		var decision legacyDecision
		if err := decodeStrictJSON(metaRaw, &decision); err != nil {
			return nil, wrap(CodeHandoffInvalid, "decode decision", entry.Name(), err)
		}
		if decision.Schema != legacySchema || decision.Kind != "decision" || decision.ID == "" || decision.Title == "" || decision.Question == "" || decision.Chosen == "" || decision.Source == "" {
			return nil, wrap(CodeHandoffInvalid, "validate decision", entry.Name(), errors.New("decision is incomplete"))
		}
		expected, err := renderLegacyDecision(decision)
		if err != nil {
			return nil, wrap(CodeHandoffInvalid, "render decision", entry.Name(), err)
		}
		if !bytes.Equal(raw, expected) {
			return nil, wrap(CodeHandoffDrift, "validate decision body", entry.Name(), errors.New("decision frontmatter and Markdown body differ"))
		}
		if decision.TaskID == taskID {
			values = append(values, decision)
		}
	}
	sort.Slice(values, func(left, right int) bool {
		if values[left].CreatedNS != values[right].CreatedNS {
			return values[left].CreatedNS < values[right].CreatedNS
		}
		if values[left].CreatedAt != values[right].CreatedAt {
			return values[left].CreatedAt < values[right].CreatedAt
		}
		return values[left].ID < values[right].ID
	})
	result := make([]map[string]any, 0, len(values))
	for _, decision := range values {
		value, err := structToMap(decision)
		if err != nil {
			return nil, wrap(CodeHandoffInvalid, "encode decision", decision.ID, err)
		}
		result = append(result, value)
	}
	return result, nil
}

func renderLegacyDecision(decision legacyDecision) ([]byte, error) {
	meta, err := structToMap(decision)
	if err != nil {
		return nil, err
	}
	return renderMarkdown(meta, decision.Title, []section{
		{heading: "问题", values: []string{decision.Question}},
		{heading: "选择", values: []string{decision.Chosen}},
		{heading: "拒绝", values: decision.Rejected},
		{heading: "为什么不选", values: decision.WhyNot},
		{heading: "来源", values: []string{decision.Source}},
	})
}

func (m *Module) readLegacyFacts(taskID string) ([]map[string]any, error) {
	path := filepath.Join(m.root, ".agent", "ledger", "facts", taskID+".jsonl")
	raw, err := readRegularFile(path, 8<<20)
	if errors.Is(err, os.ErrNotExist) {
		return []map[string]any{}, nil
	}
	if err != nil {
		return nil, wrap(CodeHandoffInvalid, "read facts", filepath.ToSlash(filepath.Join(".agent", "ledger", "facts", taskID+".jsonl")), err)
	}
	invalidated := make(map[string]struct{})
	facts := make([]legacyFact, 0)
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var identity struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(line, &identity); err != nil {
			return nil, wrap(CodeHandoffInvalid, "decode fact", fmt.Sprintf("line %d", lineNumber), err)
		}
		switch identity.Kind {
		case "fact":
			var fact legacyFact
			if err := decodeStrictJSON(line, &fact); err != nil {
				return nil, wrap(CodeHandoffInvalid, "decode fact", fmt.Sprintf("line %d", lineNumber), err)
			}
			if fact.Schema != legacySchema || fact.ID == "" || fact.TaskID != taskID || strings.TrimSpace(fact.Statement) == "" || strings.TrimSpace(fact.Source) == "" || len(fact.Tags) > 8 {
				return nil, wrap(CodeHandoffInvalid, "validate fact", fmt.Sprintf("line %d", lineNumber), errors.New("fact is incomplete"))
			}
			facts = append(facts, fact)
		case "fact_invalidation":
			var invalidation legacyFactInvalidation
			if err := decodeStrictJSON(line, &invalidation); err != nil {
				return nil, wrap(CodeHandoffInvalid, "decode fact invalidation", fmt.Sprintf("line %d", lineNumber), err)
			}
			if invalidation.Schema != legacySchema || invalidation.TaskID != taskID || invalidation.Invalidates == "" || strings.TrimSpace(invalidation.Reason) == "" {
				return nil, wrap(CodeHandoffInvalid, "validate fact invalidation", fmt.Sprintf("line %d", lineNumber), errors.New("fact invalidation is incomplete"))
			}
			invalidated[invalidation.Invalidates] = struct{}{}
		default:
			return nil, wrap(CodeHandoffInvalid, "decode fact", fmt.Sprintf("line %d", lineNumber), fmt.Errorf("unsupported fact kind %q", identity.Kind))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, wrap(CodeHandoffInvalid, "scan facts", path, err)
	}
	result := make([]map[string]any, 0, len(facts))
	for _, fact := range facts {
		if _, stale := invalidated[fact.ID]; stale {
			continue
		}
		value, err := structToMap(fact)
		if err != nil {
			return nil, wrap(CodeHandoffInvalid, "encode fact", fact.ID, err)
		}
		result = append(result, value)
	}
	return result, nil
}

func structToMap(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var result map[string]any
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func limitLegacyMemory(capsule Capsule, decisions, facts []map[string]any) (Capsule, error) {
	selectedDecisions := append([]map[string]any(nil), decisions...)
	if len(selectedDecisions) > 3 {
		selectedDecisions = selectedDecisions[len(selectedDecisions)-3:]
	}
	selectedFacts := append([]map[string]any(nil), facts...)
	if len(selectedFacts) > 12 {
		selectedFacts = selectedFacts[len(selectedFacts)-12:]
	}
	omittedDecisions := len(decisions) - len(selectedDecisions)
	omittedFacts := len(facts) - len(selectedFacts)
	for {
		capsule.Decisions = &selectedDecisions
		capsule.Facts = &selectedFacts
		capsule.Omitted = &Omitted{Decisions: omittedDecisions, Facts: omittedFacts}
		raw, err := json.MarshalIndent(capsule, "", "  ")
		if err != nil {
			return Capsule{}, wrap(CodeHandoffInvalid, "encode capsule", "", err)
		}
		if len(raw) <= CapsuleLimit {
			return capsule, nil
		}
		if len(selectedFacts) > 0 {
			selectedFacts = selectedFacts[1:]
			omittedFacts++
			continue
		}
		if len(selectedDecisions) > 0 {
			selectedDecisions = selectedDecisions[1:]
			omittedDecisions++
			continue
		}
		return Capsule{}, wrap(CodeCapsuleTooLarge, "encode capsule", "", fmt.Errorf("base capsule exceeds %d bytes", CapsuleLimit))
	}
}
