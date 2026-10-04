package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"
)

func validateCandidates(p Plan, d Discovery) error {
	if len(p.Candidates) == 0 && len(p.TargetParameters) == 0 {
		return nil
	}
	if p.Mode != "invoke" {
		return errors.New("target parameters require invoke mode")
	}
	if len(p.Candidates) > 0 {
		if len(p.Candidates) < 2 || len(p.Candidates) > 8 || len(p.TargetParameters) > 0 {
			return errors.New("comparison requires 2-8 explicit candidates without shared target parameters")
		}
		seen := map[string]bool{}
		for _, candidate := range p.Candidates {
			if b, err := json.Marshal(candidate.TargetParameters); err != nil || len(b) > 64<<10 {
				return errors.New("candidate parameters exceed 64 KiB")
			}
			if !validID.MatchString(candidate.ID) || len(candidate.ID) > 80 || seen[candidate.ID] || len(candidate.TargetParameters) == 0 {
				return errors.New("invalid or duplicate comparison candidate")
			}
			seen[candidate.ID] = true
		}
	}
	target, err := Lookup(d, p.Subject, "adapter")
	if err != nil {
		return err
	}
	if !executableScript(target.Path) {
		if target.Descriptor["type"] != "http" {
			return errors.New("target parameter comparison requires HTTP or executable subject")
		}
		if !hasParameterMapping(target.Descriptor["request"]) {
			return errors.New("HTTP target parameters require explicit $parameters request mapping")
		}
	}
	return nil
}

func hasParameterMapping(value any) bool {
	switch v := value.(type) {
	case string:
		return v == "$parameters" || strings.HasPrefix(v, "$parameters.")
	case map[string]any:
		for _, x := range v {
			if hasParameterMapping(x) {
				return true
			}
		}
	case []any:
		for _, x := range v {
			if hasParameterMapping(x) {
				return true
			}
		}
	}
	return false
}

// mapRequest keeps legacy $.field bound to inputs; only $parameters exposes
// explicit target configuration. References never enter this context.
func mapRequest(template, input any, parameters map[string]any) (any, error) {
	switch v := template.(type) {
	case string:
		if v == "$parameters" {
			return parameters, nil
		}
		if strings.HasPrefix(v, "$parameters.") {
			return selectValue(parameters, "$"+strings.TrimPrefix(v, "$parameters"))
		}
	case map[string]any:
		result := map[string]any{}
		for k, x := range v {
			y, err := mapRequest(x, input, parameters)
			if err != nil {
				return nil, err
			}
			result[k] = y
		}
		return result, nil
	case []any:
		result := make([]any, len(v))
		for i, x := range v {
			y, err := mapRequest(x, input, parameters)
			if err != nil {
				return nil, err
			}
			result[i] = y
		}
		return result, nil
	}
	return mapValue(template, input)
}

func cloneMap(value map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range value {
		out[k] = v
	}
	return out
}

func targetParameters(p Plan, c Case) map[string]any {
	for _, candidate := range p.Candidates {
		if c.Metadata["seven_eval_candidate"] == candidate.ID {
			return candidate.TargetParameters
		}
	}
	return p.TargetParameters
}

func parameterReceipt(requested, applied map[string]any) *ParameterReceipt {
	receipt := &ParameterReceipt{Requested: requested, Applied: applied, State: "unconfirmed"}
	if applied != nil {
		receipt.State = "target_confirmed"
		for k, v := range requested {
			if actual, ok := applied[k]; !ok || !equalParameter(v, actual) {
				receipt.State = "mismatch"
				break
			}
		}
	}
	return receipt
}

func equalParameter(a, b any) bool {
	normalize := func(v any) any {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var value any
		if decodeJSON(raw, &value) != nil {
			return nil
		}
		return value
	}
	var equal func(any, any) bool
	equal = func(a, b any) bool {
		switch x := a.(type) {
		case json.Number:
			y, ok := b.(json.Number)
			if !ok {
				return false
			}
			bounded := func(n json.Number) bool {
				parts := strings.FieldsFunc(string(n), func(r rune) bool { return r == 'e' || r == 'E' })
				if len(parts) < 2 {
					return true
				}
				exp, err := strconv.Atoi(parts[1])
				return err == nil && exp >= -10000 && exp <= 10000
			}
			if !bounded(x) || !bounded(y) {
				return x == y
			}
			left, ok := new(big.Rat).SetString(string(x))
			if !ok {
				return false
			}
			right, ok := new(big.Rat).SetString(string(y))
			return ok && left.Cmp(right) == 0
		case map[string]any:
			y, ok := b.(map[string]any)
			if !ok || len(x) != len(y) {
				return false
			}
			for k, v := range x {
				if w, ok := y[k]; !ok || !equal(v, w) {
					return false
				}
			}
			return true
		case []any:
			y, ok := b.([]any)
			if !ok || len(x) != len(y) {
				return false
			}
			for i, v := range x {
				if !equal(v, y[i]) {
					return false
				}
			}
			return true
		default:
			return reflect.DeepEqual(a, b)
		}
	}
	return equal(normalize(a), normalize(b))
}

func (e *Engine) preflightParameters(r *Run, cases []Case) error {
	if len(r.Effective.Plan.Candidates) == 0 && len(r.Effective.Plan.TargetParameters) == 0 {
		return nil
	}
	for _, c := range cases {
		params := targetParameters(r.Effective.Plan, c)
		if Digest(params) != Digest(e.redactForRun(r, params)) {
			return errors.New("target parameters must not contain credentials; use adapter environment references")
		}
	}
	target, err := Lookup(Discovery{Assets: r.Effective.Assets}, r.Effective.Plan.Subject, "adapter")
	if err != nil {
		return err
	}
	if executableScript(target.Path) {
		return nil
	}
	path, err := SafePath(e.Root, target.Path)
	if err != nil {
		return err
	}
	var cfg HTTPConfig
	if err = ReadDocument(path, &cfg); err != nil {
		return err
	}
	for _, c := range cases {
		if _, err = mapRequest(cfg.Request, c.Inputs, targetParameters(r.Effective.Plan, c)); err != nil {
			return fmt.Errorf("target parameter request mapping invalid: %w", err)
		}
	}
	return nil
}
