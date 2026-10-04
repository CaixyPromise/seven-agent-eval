package core

import (
	"encoding/json"
	"errors"
	"math"
	"seven-agent-eval/protocol"
	"strconv"
	"strings"
)

type ReportKPIs struct {
	Scored    *float64
	Coverage  *float64
	Anomalies *float64
	Conflict  bool
}

func reportPointer(data any, pointer string) (any, error) {
	if pointer == "" {
		return data, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, errors.New("invalid report pointer")
	}
	for _, part := range strings.Split(pointer[1:], "/") {
		for i := 0; i < len(part); i++ {
			if part[i] == '~' {
				if i+1 >= len(part) || (part[i+1] != '0' && part[i+1] != '1') {
					return nil, errors.New("invalid report escape")
				}
				i++
			}
		}
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch v := data.(type) {
		case map[string]any:
			var ok bool
			data, ok = v[part]
			if !ok {
				return nil, errors.New("report field missing")
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(v) || strconv.Itoa(index) != part {
				return nil, errors.New("invalid report index")
			}
			data = v[index]
		default:
			return nil, errors.New("report field missing")
		}
	}
	return data, nil
}

func reportNumber(value any, key string) (*float64, error) {
	if value == nil {
		return nil, nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var raw any
	if err = decodeJSON(b, &raw); err != nil {
		return nil, err
	}
	n, ok := raw.(json.Number)
	if !ok {
		return nil, errors.New("report KPI must be numeric or null")
	}
	f, err := n.Float64()
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return nil, errors.New("invalid report KPI")
	}
	if key == "coverage" {
		if f > 1 {
			return nil, errors.New("invalid coverage")
		}
	} else if math.Trunc(f) != f || f > 9007199254740991 {
		return nil, errors.New("invalid report count")
	}
	return &f, nil
}

func mappedKPIs(output ProcessorOutput) (ReportKPIs, error) {
	var result ReportKPIs
	if output.Report == nil {
		return result, nil
	}
	for key, path := range output.Report.KPIs {
		value, err := reportPointer(output.Data, path)
		if err != nil {
			return result, err
		}
		n, err := reportNumber(value, key)
		if err != nil {
			return result, err
		}
		switch key {
		case "scored":
			result.Scored = n
		case "coverage":
			result.Coverage = n
		case "anomalies":
			result.Anomalies = n
		default:
			return result, errors.New("unknown report KPI")
		}
	}
	return result, nil
}

func validateProcessorOutput(output ProcessorOutput) error {
	if err := protocol.Validate("processor_output", output); err != nil {
		return err
	}
	_, err := mappedKPIs(output)
	return err
}

// ResolveReportKPIs uses only explicitly declared providers.
func ResolveReportKPIs(outputs []ProcessorOutput) ReportKPIs {
	var providers []ProcessorOutput
	for _, o := range outputs {
		if o.Report != nil {
			providers = append(providers, o)
		}
	}
	if len(providers) > 1 {
		return ReportKPIs{Conflict: true}
	}
	if len(providers) == 1 {
		r, _ := mappedKPIs(providers[0])
		return r
	}
	return ReportKPIs{}
}
