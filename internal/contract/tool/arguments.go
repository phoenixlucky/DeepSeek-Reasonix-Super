package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// ArgumentPreparer handles a proxy's target envelope without executing or
// connecting the target. Unknown target schemas leave arguments unchanged.
type ArgumentPreparer interface {
	PrepareArguments(json.RawMessage) (json.RawMessage, bool)
}

// PrepareArguments removes only schema-proven redundant envelopes.
func PrepareArguments(t Tool, args json.RawMessage) (json.RawMessage, bool) {
	if effective, changed := UnwrapArguments(t.Schema(), args); changed {
		return effective, true
	}
	if proxy, ok := t.(ArgumentPreparer); ok {
		effective, changed := proxy.PrepareArguments(args)
		if !changed {
			return args, false
		}
		if _, err := uniqueJSON(args); err != nil {
			return args, false
		}
		return effective, true
	}
	return args, false
}

// UnwrapArguments accepts one redundant envelope only when the original misses
// required fields and the inner object validates against an unambiguous schema.
func UnwrapArguments(schema, args json.RawMessage) (json.RawMessage, bool) {
	unchanged := func() (json.RawMessage, bool) { return args, false }
	var envelope map[string]json.RawMessage
	if json.Unmarshal(args, &envelope) != nil || len(envelope) != 1 || envelope["arguments"] == nil {
		return unchanged()
	}
	outerValue, err := uniqueJSON(args)
	if err != nil {
		return unchanged()
	}
	outer, ok := outerValue.(map[string]any)
	if !ok || len(outer) != 1 {
		return unchanged()
	}
	if _, ok := outer["arguments"]; !ok {
		return unchanged()
	}
	doc, err := uniqueJSON(schema)
	if err != nil {
		return unchanged()
	}
	root, ok := doc.(map[string]any)
	if !ok || root["type"] != "object" {
		return unchanged()
	}
	for key := range root {
		switch key {
		case "type", "properties", "required", "additionalProperties", "description", "title", "$schema", "$defs", "definitions":
		default:
			return unchanged()
		}
	}
	properties, ok := root["properties"].(map[string]any)
	if !ok {
		return unchanged()
	}
	if _, declared := properties["arguments"]; declared {
		return unchanged()
	}
	required, ok := root["required"].([]any)
	if !ok || len(required) == 0 {
		return unchanged()
	}
	for _, field := range required {
		name, ok := field.(string)
		if !ok || name == "arguments" {
			return unchanged()
		}
		if _, declared := properties[name]; !declared {
			return unchanged()
		}
	}
	inner := envelope["arguments"]
	if encoded, ok := outer["arguments"].(string); ok {
		inner = json.RawMessage(encoded)
	}
	value, err := uniqueJSON(inner)
	if err != nil {
		return unchanged()
	}
	if _, ok := value.(map[string]any); !ok {
		return unchanged()
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(nil)
	compiler.DefaultDraft(jsonschema.Draft7)
	const resource = "urn:reasonix:argument-envelope"
	if compiler.AddResource(resource, doc) != nil {
		return unchanged()
	}
	compiled, err := compiler.Compile(resource)
	if err != nil || compiled.Validate(outer) == nil || compiled.Validate(value) != nil {
		return unchanged()
	}
	return inner, true
}

// uniqueJSON rejects duplicate keys before a map decoder could erase them.
func uniqueJSON(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := uniqueJSONValue(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing JSON")
	}
	return value, nil
}

func uniqueJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("JSON nesting limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		object := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("invalid object key")
			}
			if _, duplicate := object[name]; duplicate {
				return nil, fmt.Errorf("duplicate object key")
			}
			value, err := uniqueJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		_, err := decoder.Token()
		return object, err
	case json.Delim('['):
		array := []any{}
		for decoder.More() {
			value, err := uniqueJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		_, err := decoder.Token()
		return array, err
	default:
		return token, nil
	}
}
