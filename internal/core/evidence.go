package core

// Stable evidence IDs allow observations to resolve even an empty retrieval list.
func structuredEvidence(id string, result SubjectResult) []Evidence {
	out := []Evidence{{ID: id + ":retrieval", Kind: "retrieval", Value: result.Retrieval}}
	if result.Tools != nil {
		out = append(out, Evidence{ID: id + ":tools", Kind: "tool", Value: result.Tools})
	}
	if result.Citations != nil {
		out = append(out, Evidence{ID: id + ":citations", Kind: "citation", Value: result.Citations})
	}
	if result.Trace != nil {
		out = append(out, Evidence{ID: id + ":trace", Kind: "trace", Value: result.Trace})
	}
	return out
}
