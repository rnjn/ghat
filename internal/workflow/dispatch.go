// Package workflow reads what ghtui needs from GitHub Actions workflow files.
package workflow

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Input is one workflow_dispatch input. Type is string, boolean, choice,
// number or environment; Default is rendered as text.
type Input struct {
	Name        string
	Description string
	Type        string
	Default     string
	Required    bool
	Options     []string
}

// ParseDispatch reports whether a workflow can be triggered by
// workflow_dispatch and returns its inputs in file order.
func ParseDispatch(file []byte) (dispatchable bool, inputs []Input, err error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(file, &doc); err != nil {
		return false, nil, fmt.Errorf("parse workflow: %w", err)
	}
	if len(doc.Content) == 0 {
		return false, nil, nil
	}
	on := mapValue(doc.Content[0], "on", "true") // YAML 1.1 reads a bare on as true
	if on == nil {
		return false, nil, nil
	}
	switch on.Kind {
	case yaml.ScalarNode:
		return on.Value == "workflow_dispatch", nil, nil
	case yaml.SequenceNode:
		for _, n := range on.Content {
			if n.Value == "workflow_dispatch" {
				return true, nil, nil
			}
		}
		return false, nil, nil
	case yaml.MappingNode:
		wd, found := mapEntry(on, "workflow_dispatch")
		if !found {
			return false, nil, nil
		}
		in, err := parseInputs(mapValue(wd, "inputs"))
		return true, in, err
	}
	return false, nil, nil
}

func parseInputs(n *yaml.Node) ([]Input, error) {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, nil
	}
	var out []Input
	for i := 0; i+1 < len(n.Content); i += 2 {
		in := Input{Name: n.Content[i].Value, Type: "string"}
		spec := n.Content[i+1]
		if v := mapValue(spec, "description"); v != nil {
			in.Description = v.Value
		}
		if v := mapValue(spec, "type"); v != nil && v.Value != "" {
			in.Type = v.Value
		}
		if v := mapValue(spec, "default"); v != nil {
			in.Default = v.Value
		}
		if v := mapValue(spec, "required"); v != nil {
			if err := v.Decode(&in.Required); err != nil {
				return nil, fmt.Errorf("input %s: required: %w", in.Name, err)
			}
		}
		if v := mapValue(spec, "options"); v != nil {
			if err := v.Decode(&in.Options); err != nil {
				return nil, fmt.Errorf("input %s: options: %w", in.Name, err)
			}
		}
		out = append(out, in)
	}
	return out, nil
}

// mapEntry finds key in a mapping node; found is true even for a null value.
func mapEntry(n *yaml.Node, keys ...string) (*yaml.Node, bool) {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		for _, k := range keys {
			if n.Content[i].Value == k {
				return n.Content[i+1], true
			}
		}
	}
	return nil, false
}

func mapValue(n *yaml.Node, keys ...string) *yaml.Node {
	v, _ := mapEntry(n, keys...)
	return v
}
