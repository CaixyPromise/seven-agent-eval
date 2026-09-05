package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type boundedBuffer struct {
	bytes.Buffer
	max int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		return 0, errors.New("worker output limit exceeded")
	}
	return b.Buffer.Write(p)
}

type Worker struct {
	Home   string
	Python string
	Source string
}

func NewWorker(home string) Worker {
	python := os.Getenv("SEVEN_EVAL_PYTHON")
	if python == "" {
		python = filepath.Join(home, "sdk", "python", ".venv", "bin", "python")
	}
	return Worker{Home: home, Python: python}
}
func (w Worker) Call(ctx context.Context, method string, params any, out any) error {
	req := []map[string]any{
		{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}},
		{"jsonrpc": "2.0", "id": 2, "method": method, "params": params},
	}
	var input bytes.Buffer
	for _, v := range req {
		if e := json.NewEncoder(&input).Encode(v); e != nil {
			return e
		}
	}
	cmd := exec.CommandContext(ctx, w.Python, "-m", "seven_evaluate.worker")
	source := w.Source
	if source == "" {
		source = filepath.Join(w.Home, "sdk", "python", "src")
	}
	cmd.Env = append(os.Environ(), "PYTHONPATH="+source)
	cmd.Stdin = &input
	stdout := &boundedBuffer{max: 8 << 20}
	stderr := &boundedBuffer{max: 64 << 10}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = 2 * time.Second
	if e := cmd.Run(); e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("worker failed; verify Python environment or extension (output redacted)")
	}
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	for id := 1; id <= 2; id++ {
		var reply struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Result  json.RawMessage `json:"result"`
			Error   *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if e := decoder.Decode(&reply); e != nil {
			return errors.New("worker protocol: invalid JSON")
		}
		if reply.ID != id || reply.JSONRPC != "2.0" {
			return errors.New("worker protocol: response identity mismatch")
		}
		if reply.Error != nil {
			return fmt.Errorf("worker operation failed (code %d)", reply.Error.Code)
		}
		if id == 1 {
			var init struct {
				ProtocolVersion string `json:"protocol_version"`
			}
			if e := decodeJSON(reply.Result, &init); e != nil || init.ProtocolVersion != Version {
				return errors.New("worker version mismatch")
			}
		} else if e := decodeJSON(reply.Result, out); e != nil {
			return errors.New("worker result type mismatch")
		}
	}
	var tail any
	if e := decoder.Decode(&tail); e != io.EOF {
		return errors.New("worker protocol: trailing data")
	}
	return nil
}
