package core

import (
	"context"
	"errors"
)

func expandEpisodes(input []Case) ([]Case, error) {
	out := []Case{}
	seen := map[string]bool{}
	episodeIDs := map[string]bool{}
	for _, c := range input {
		if c.Episode == nil && (c.Metadata["episode_id"] != nil || c.Metadata["turn_id"] != nil) {
			return nil, errors.New("episode identity is reserved for ordered Episode inputs")
		}
		turns := []Case{c}
		if c.Episode != nil {
			ep := c.Episode
			if episodeIDs[ep.ID] || len(ep.Turns) == 0 {
				return nil, errors.New("duplicate or empty episode")
			}
			episodeIDs[ep.ID] = true
			turnIDs := map[string]bool{}
			turns = nil
			for _, turn := range ep.Turns {
				if turn.ID == "" || turnIDs[turn.ID] || turn.Case.Episode != nil {
					return nil, errors.New("invalid or nested turn")
				}
				turnIDs[turn.ID] = true
				child := turn.Case
				child.ID = ep.ID + ":" + turn.ID
				if child.Metadata == nil {
					child.Metadata = map[string]any{}
				}
				child.Metadata["episode_id"] = ep.ID
				child.Metadata["turn_id"] = turn.ID
				turns = append(turns, child)
			}
		}
		for _, turn := range turns {
			if seen[turn.ID] {
				return nil, errors.New("duplicate normalized case id")
			}
			seen[turn.ID] = true
			out = append(out, turn)
			if len(out) > MaxCases {
				return nil, errors.New("expanded input exceeds 1000 cases")
			}
		}
	}
	return out, nil
}

func episodeIndex(r *Run, id string) int {
	for i := range r.Episodes {
		if r.Episodes[i].ID == id {
			return i
		}
	}
	r.Episodes = append(r.Episodes, EpisodeResult{ID: id, Status: "running", CaseIDs: []string{}})
	return len(r.Episodes) - 1
}

func (e *Engine) createSubjectSession(ctx context.Context, r *Run) (any, error) {
	a, err := e.asset(r, r.Effective.Plan.Subject, "adapter")
	if err != nil {
		return nil, err
	}
	path, err := e.extensionPath(r, a)
	if err != nil {
		return nil, err
	}
	if executableScript(a.Path) {
		if a.Descriptor["session_lifecycle"] != true {
			return nil, errors.New("Episode subject requires session lifecycle")
		}
		var session any
		err = e.workerFor(r).Call(ctx, "subject/create", map[string]any{"path": path, "context": map[string]any{}}, &session)
		return session, err
	}
	var cfg HTTPConfig
	if err = ReadDocument(path, &cfg); err != nil {
		return nil, err
	}
	if cfg.Type != "seven-workspace" {
		return nil, errors.New("Episode requires session-capable subject")
	}
	return createSevenSession(ctx, cfg)
}
