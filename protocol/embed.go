// Package protocol embeds the exchange contract used by both runtime and bundles.
package protocol

import (
	"embed"
	"encoding/json"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"sync"
)

//go:embed schemas/*.json
var Files embed.FS
var once sync.Once
var contract *jsonschema.Schema
var compileErr error

func Validate(kind string, value any) error {
	once.Do(func() {
		b, err := Files.ReadFile("schemas/exchange.json")
		if err != nil {
			compileErr = err
			return
		}
		var doc any
		if err = json.Unmarshal(b, &doc); err != nil {
			compileErr = err
			return
		}
		c := jsonschema.NewCompiler()
		compileErr = c.AddResource("https://seven.local/exchange.json", doc)
		if compileErr == nil {
			contract, compileErr = c.Compile("https://seven.local/exchange.json")
		}
	})
	if compileErr != nil {
		return compileErr
	}
	raw, err := json.Marshal(map[string]any{"kind": kind, "value": value})
	if err != nil {
		return err
	}
	var v any
	if err = json.Unmarshal(raw, &v); err != nil {
		return err
	}
	if err = contract.Validate(v); err != nil {
		return fmt.Errorf("invalid %s: %w", kind, err)
	}
	return nil
}
