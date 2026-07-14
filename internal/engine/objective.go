package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/summerchaserwwz/summer-harness/internal/continuity"
	"github.com/summerchaserwwz/summer-harness/internal/ledger"
)

const (
	maxDoneItems       = 8
	maxNextItems       = 3
	maxValidationItems = 8
	maxBlockerItems    = 5
	maxMustReadItems   = 5
)

type objectiveState struct {
	objectives map[string]Objective
	activeID   string
}

func foldObjectives(transactions []ledger.Transaction) (objectiveState, error) {
	state := objectiveState{objectives: make(map[string]Objective)}
	for _, transaction := range transactions {
		for _, event := range transaction.Events {
			if event.Kind != "ObjectiveStarted" && event.Kind != "ObjectiveSaved" && event.Kind != "ObjectiveImported" {
				continue
			}
			var objective Objective
			if event.Kind == "ObjectiveImported" {
				var imported legacyImportedRecord
				if err := decodeStrictPayload(event.Data, &imported); err != nil || imported.Objective == nil {
					return objectiveState{}, fmt.Errorf("decode %s event: imported objective is missing or invalid", event.Kind)
				}
				objective = *imported.Objective
			} else if err := json.Unmarshal(event.Data, &objective); err != nil {
				return objectiveState{}, fmt.Errorf("decode %s event: %w", event.Kind, err)
			}
			normalizeObjective(&objective)
			if objective.ObjectiveID == "" || objective.ObjectiveID != event.EntityID || objective.ProjectID != transaction.ProjectID {
				return objectiveState{}, fmt.Errorf("%s event has inconsistent objective identity", event.Kind)
			}
			switch event.Kind {
			case "ObjectiveStarted":
				if _, exists := state.objectives[objective.ObjectiveID]; exists {
					return objectiveState{}, errors.New("canonical ledger starts the same root objective more than once")
				}
				if state.activeID != "" {
					return objectiveState{}, errors.New("canonical ledger has multiple active root objectives")
				}
				if objective.Revision != 1 || objective.Status != ObjectiveActive {
					return objectiveState{}, errors.New("ObjectiveStarted event has an invalid initial state")
				}
			case "ObjectiveImported":
				if _, exists := state.objectives[objective.ObjectiveID]; exists {
					return objectiveState{}, errors.New("canonical ledger imports the same root objective more than once")
				}
				if objective.Revision == 0 || !validObjectiveStatus(objective.Status) {
					return objectiveState{}, errors.New("ObjectiveImported event has an invalid state")
				}
				if isNonterminalObjectiveStatus(objective.Status) && state.activeID != "" {
					return objectiveState{}, errors.New("canonical ledger imports multiple nonterminal root objectives")
				}
			case "ObjectiveSaved":
				current, exists := state.objectives[objective.ObjectiveID]
				if !exists || state.activeID != objective.ObjectiveID {
					return objectiveState{}, errors.New("ObjectiveSaved event does not target the active root objective")
				}
				if objective.Revision != current.Revision+1 {
					return objectiveState{}, errors.New("ObjectiveSaved event skips the objective revision")
				}
				if objective.ProjectID != current.ProjectID || objective.Title != current.Title || objective.Goal != current.Goal || objective.Profile != current.Profile || objective.Risk != current.Risk || !equalStringSlices(objective.Acceptance, current.Acceptance) || !equalStringSlices(objective.ResidualRisks, current.ResidualRisks) {
					return objectiveState{}, errors.New("ObjectiveSaved event changes immutable objective fields")
				}
				if objective.Status != ObjectiveActive && objective.Status != ObjectiveBlocked {
					return objectiveState{}, errors.New("ObjectiveSaved event has an invalid status")
				}
			}
			state.objectives[objective.ObjectiveID] = objective
			if isNonterminalObjectiveStatus(objective.Status) {
				state.activeID = objective.ObjectiveID
			}
		}
	}
	return state, nil
}

func validObjectiveStatus(status ObjectiveStatus) bool {
	switch status {
	case ObjectiveActive, ObjectiveBlocked, ObjectiveReview, ObjectiveCompleted, ObjectiveCancelled:
		return true
	default:
		return false
	}
}

func isNonterminalObjectiveStatus(status ObjectiveStatus) bool {
	return status == ObjectiveActive || status == ObjectiveBlocked || status == ObjectiveReview
}

func validateSaveObjective(save SaveObjective) error {
	if err := validateBoundedText(save.ObjectiveID, "objective id", 200); err != nil {
		return err
	}
	if save.ExpectedObjectiveRevision == 0 {
		return errors.New("expected objective revision is required")
	}
	if err := validateValues(save.Done, "done item", maxDoneItems); err != nil {
		return err
	}
	if save.Next != nil {
		if err := validateValues(*save.Next, "next item", maxNextItems); err != nil {
			return err
		}
	}
	if err := validateValues(save.Validation, "validation item", maxValidationItems); err != nil {
		return err
	}
	if save.Blockers != nil {
		if err := validateValues(*save.Blockers, "blocker", maxBlockerItems); err != nil {
			return err
		}
	}
	if save.MustRead != nil {
		if err := validateValues(*save.MustRead, "must-read reference", maxMustReadItems); err != nil {
			return err
		}
	}
	return nil
}

func validateValues(values []string, label string, limit int) error {
	values = cleanStrings(values)
	if len(values) > limit {
		return fmt.Errorf("%s exceeds %d items", label, limit)
	}
	for _, value := range values {
		if err := validateBoundedText(value, label, maxObjectiveTextChars); err != nil {
			return err
		}
	}
	return nil
}

func applyCheckpoint(current Objective, save SaveObjective) (Objective, error) {
	next := cloneObjective(current)
	var err error
	if save.ReplaceDone {
		next.Done = cleanStrings(save.Done)
	} else {
		if next.Done, err = appendUniqueBounded(next.Done, save.Done, maxDoneItems, "done"); err != nil {
			return Objective{}, err
		}
	}
	if save.Next != nil {
		next.Next = cleanStrings(*save.Next)
	}
	if save.ReplaceValidation {
		next.Validation = cleanStrings(save.Validation)
	} else {
		if next.Validation, err = appendUniqueBounded(next.Validation, save.Validation, maxValidationItems, "validation"); err != nil {
			return Objective{}, err
		}
	}
	if save.Blockers != nil {
		next.Blockers = cleanStrings(*save.Blockers)
	}
	if save.MustRead != nil {
		next.MustRead = cleanStrings(*save.MustRead)
	}
	if len(next.Blockers) > 0 {
		next.Status = ObjectiveBlocked
	} else {
		next.Status = ObjectiveActive
	}
	next.Revision++
	return next, nil
}

func appendUniqueBounded(existing, incoming []string, limit int, label string) ([]string, error) {
	merged := cleanStrings(append(append([]string(nil), existing...), incoming...))
	if len(merged) > limit {
		return nil, fmt.Errorf("%s exceeds %d items; replace it with a bounded summary", label, limit)
	}
	return merged, nil
}

func continuityState(objective Objective, builtAt time.Time) continuity.State {
	return continuity.State{
		ProjectID: objective.ProjectID, ObjectiveID: objective.ObjectiveID,
		ObjectiveStatus: string(objective.Status), ObjectiveRevision: objective.Revision,
		Goal: objective.Goal, Profile: objective.Profile, Acceptance: cloneStrings(objective.Acceptance),
		Done: cloneStrings(objective.Done), Next: cloneStrings(objective.Next),
		Validation: cloneStrings(objective.Validation), Blockers: cloneStrings(objective.Blockers),
		MustRead: cloneStrings(objective.MustRead), ResumeCommand: "summer resume", BuiltAt: builtAt,
	}
}

func normalizeObjective(objective *Objective) {
	objective.Title = strings.TrimSpace(objective.Title)
	objective.Goal = strings.TrimSpace(objective.Goal)
	objective.Profile = strings.TrimSpace(objective.Profile)
	objective.Acceptance = cleanStrings(objective.Acceptance)
	objective.Done = cleanStrings(objective.Done)
	objective.Next = cleanStrings(objective.Next)
	objective.Validation = cleanStrings(objective.Validation)
	objective.Blockers = cleanStrings(objective.Blockers)
	objective.MustRead = cleanStrings(objective.MustRead)
	objective.ResidualRisks = cleanStrings(objective.ResidualRisks)
}

func cloneObjective(objective Objective) Objective {
	copy := objective
	copy.Acceptance = cloneStrings(objective.Acceptance)
	copy.Done = cloneStrings(objective.Done)
	copy.Next = cloneStrings(objective.Next)
	copy.Validation = cloneStrings(objective.Validation)
	copy.Blockers = cloneStrings(objective.Blockers)
	copy.MustRead = cloneStrings(objective.MustRead)
	copy.ResidualRisks = cloneStrings(objective.ResidualRisks)
	return copy
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func valuesFromPointers(values ...*[]string) []string {
	var result []string
	for _, value := range values {
		if value != nil {
			result = append(result, (*value)...)
		}
	}
	return result
}
