package core

import (
	"encoding/json"
	"errors"
	"strconv"
)

func validateReplayInput(raw map[string]any) error {
	if episode, ok := raw["episode"].(map[string]any); ok {
		turns, _ := episode["turns"].([]any)
		for _, turn := range turns {
			item, _ := turn.(map[string]any)
			child, _ := item["case"].(map[string]any)
			if err := validateReplayInput(child); err != nil {
				return err
			}
		}
		return nil
	}
	if _, ok := raw["response"]; !ok && raw["result"] == nil {
		return errors.New("replay requires response or result on every Case/Turn")
	}
	return nil
}

func validateCaseReferences(raw map[string]any) error {
	if episode, ok := raw["episode"].(map[string]any); ok {
		turns, _ := episode["turns"].([]any)
		for _, turn := range turns {
			item, _ := turn.(map[string]any)
			child, _ := item["case"].(map[string]any)
			if err := validateCaseReferences(child); err != nil {
				return err
			}
		}
	}
	refs, _ := raw["references"].(map[string]any)
	if qrels, present := refs["qrels"]; present {
		grades, ok := qrels.(map[string]any)
		if !ok {
			return errors.New("qrels must map document IDs to nonnegative relevance grades")
		}
		for id, v := range grades {
			if id == "" {
				return errors.New("empty qrel document ID")
			}
			n, ok := v.(json.Number)
			if !ok {
				return errors.New("qrel relevance must be numeric")
			}
			grade, err := strconv.Atoi(n.String())
			if err != nil || grade < 0 {
				return errors.New("qrel relevance must be nonnegative integer")
			}
		}
	}
	return nil
}
