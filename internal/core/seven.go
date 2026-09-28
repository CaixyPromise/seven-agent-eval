package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

type subjectSessionKey struct{}

func sevenRequest(ctx context.Context, cfg HTTPConfig, path string, payload any) (*http.Response, error) {
	base, err := url.Parse(cfg.Endpoint)
	if err != nil || base.Host == "" || base.User != nil || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, errors.New("invalid Seven endpoint")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(cfg.Endpoint, "/")+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.AuthEnv != "" {
		secret := os.Getenv(cfg.AuthEnv)
		if secret == "" {
			return nil, errors.New("missing Seven credential")
		}
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.New("Seven transport failed")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return nil, fmt.Errorf("Seven status %d", response.StatusCode)
	}
	return response, nil
}

func createSevenSession(ctx context.Context, cfg HTTPConfig) (string, error) {
	if cfg.WorkspaceID == "" || strings.ContainsAny(cfg.WorkspaceID, "/?#") {
		return "", errors.New("invalid workspace id")
	}
	response, err := sevenRequest(ctx, cfg, "/workspaces/"+cfg.WorkspaceID+"/sessions", map[string]any{"title": "seven-eval " + NewID(), "scopeType": "private"})
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	b, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var envelope struct {
		Data struct {
			ID any `json:"id"`
		} `json:"data"`
	}
	if err = decodeJSON(b, &envelope); err != nil {
		return "", errors.New("Seven session response invalid")
	}
	id := fmt.Sprint(envelope.Data.ID)
	if id == "<nil>" || id == "" || strings.ContainsAny(id, "/?#") {
		return "", errors.New("Seven session identity missing")
	}
	return id, nil
}

func invokeSeven(ctx context.Context, cfg HTTPConfig, c Case) (SubjectResult, error) {
	result := SubjectResult{Response: "", Tools: []ToolEvidence{}}
	session, _ := ctx.Value(subjectSessionKey{}).(string)
	var err error
	if session == "" {
		session, err = createSevenSession(ctx, cfg)
		if err != nil {
			return result, err
		}
	}
	inputs, ok := c.Inputs.(map[string]any)
	if !ok {
		return result, errors.New("Seven inputs require text")
	}
	text, ok := inputs["text"].(string)
	if !ok {
		return result, errors.New("Seven inputs require text")
	}
	start := time.Now()
	response, err := sevenRequest(ctx, cfg, "/workspaces/"+cfg.WorkspaceID+"/sessions/"+session+"/conversation/stream", map[string]any{"text": text, "branchId": "main"})
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		return result, errors.New("Seven response is not SSE")
	}
	scan := bufio.NewScanner(io.LimitReader(response.Body, (8<<20)+1))
	scan.Buffer(make([]byte, 4096), 1<<20)
	event := ""
	data := []string{}
	parts := map[string]string{}
	primaryMessage := ""
	tools := map[string]ToolEvidence{}
	terminal := false
	count := 0
	consume := func() error {
		if len(data) == 0 {
			return nil
		}
		count++
		if count > 10000 {
			return errors.New("Seven event limit exceeded")
		}
		var payload map[string]any
		if er := decodeJSON([]byte(strings.Join(data, "\n")), &payload); er != nil {
			return errors.New("invalid Seven event")
		}
		result.Trace = append(result.Trace, map[string]any{"event": event})
		if event == "conversation.failed" || event == "conversation.cancelled" || event == "conversation.paused" {
			return errors.New("Seven conversation failed")
		}
		if event == "conversation.completed" {
			terminal = true
		}
		ops, _ := payload["ops"].([]any)
		for _, raw := range ops {
			op, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			switch op["op"] {
			case "part.replace":
				part, ok := op["part"].(map[string]any)
				if !ok {
					continue
				}
				id, _ := part["id"].(string)
				if part["type"] == "message" {
					primaryMessage = id
					parts[id], _ = part["text"].(string)
				}
				if part["type"] == "tool" {
					tid, _ := part["toolCallId"].(string)
					name, _ := part["toolName"].(string)
					arguments, _ := part["input"].(map[string]any)
					if encoded, ok := part["input"].(string); ok {
						_ = decodeJSON([]byte(encoded), &arguments)
					}
					state, _ := part["state"].(string)
					status := "pending"
					if state == "completed" {
						status = "completed"
					}
					if state == "failed" || state == "error" {
						status = "error"
					}
					tools[tid] = ToolEvidence{ID: tid, Name: name, Arguments: arguments, Status: status, Result: part["output"]}
				}
			case "part.append_text":
				id, _ := op["partId"].(string)
				delta, _ := op["text"].(string)
				parts[id] += delta
			}
		}
		return nil
	}
	finish := func() {
		// Match Seven's current runner: the latest message part is the answer;
		// earlier tool-round messages must not be concatenated into it.
		result.Response = parts[primaryMessage]
		ids := []string{}
		for id := range tools {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			result.Tools = append(result.Tools, tools[id])
		}
		latency := float64(time.Since(start).Microseconds()) / 1000
		result.LatencyMS = &latency
	}
	for scan.Scan() {
		line := scan.Text()
		if line == "" {
			err = consume()
			data = nil
			event = ""
			if err != nil || terminal {
				break
			}
		} else if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(line[6:])
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(line[5:]))
		}
	}
	if err == nil && len(data) > 0 {
		err = consume()
	}
	finish()
	if err != nil {
		return result, err
	}
	if er := scan.Err(); er != nil {
		return result, errors.New("Seven stream interrupted")
	}
	if !terminal {
		return result, errors.New("Seven terminal event missing")
	}
	return result, nil
}
