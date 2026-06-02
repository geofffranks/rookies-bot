package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// UpdateBotConfigFile rewrites the bot config file at path, setting each
// key/value pair in updates. It round-trips through a yaml.Node so existing
// comments, formatting, and untouched keys (including secrets) are preserved.
// Missing keys are appended to the top-level mapping.
func UpdateBotConfigFile(path string, updates map[string]string) error {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied CLI config path
	if err != nil {
		return fmt.Errorf("failed reading %s: %w", path, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("failed parsing %s: %w", path, err)
	}

	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("config file %s is not a YAML mapping", path)
	}
	mapping := root.Content[0]

	for key, value := range updates {
		setMappingValue(mapping, key, value)
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("failed serializing updated config %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0600); err != nil {
		return fmt.Errorf("failed writing %s: %w", path, err)
	}
	return nil
}

// setMappingValue sets the scalar value for key in a YAML mapping node,
// appending the key/value pair if key is not already present. Mapping node
// Content is a flat slice of [key0, val0, key1, val1, ...].
func setMappingValue(mapping *yaml.Node, key, value string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			valNode := mapping.Content[i+1]
			valNode.Kind = yaml.ScalarNode
			valNode.Tag = "!!str"
			valNode.Style = 0
			valNode.Value = value
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}
