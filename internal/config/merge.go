package config

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// merge returns base with over merged into it, following the same rule as
// Waco's settings:
//
//   - a scalar in over replaces the one in base;
//   - lists are joined, base first, dropping items of over already present;
//   - dictionaries are merged key by key, recursively;
//   - a null in over (a key with no value) leaves base as it is.
//
// It works on yaml.Node values rather than decoded Go values, so scalars keep
// their exact text: an unquoted 3.10 stays "3.10" instead of becoming the
// number 3.1. Neither argument is modified. path names the position for error
// messages ("gnome.settings").
func merge(base, over *yaml.Node, path string) (*yaml.Node, error) {
	switch {
	case isNull(over):
		return base, nil
	case isNull(base):
		return over, nil
	case base.Kind != over.Kind:
		return nil, fmt.Errorf("%s: common.yml has %s, local.yml %s (line %d)",
			describePath(path), kindName(base), kindName(over), over.Line)
	}

	switch base.Kind {
	case yaml.MappingNode:
		return mergeMappings(base, over, path)
	case yaml.SequenceNode:
		return mergeSequences(base, over), nil
	default:
		return over, nil
	}
}

// mergeMappings merges two mapping nodes key by key. Keys keep the order of
// base, followed by the keys only over has, in their order.
func mergeMappings(base, over *yaml.Node, path string) (*yaml.Node, error) {
	result := shallowCopy(base)
	result.Content = append([]*yaml.Node(nil), base.Content...)

	// A mapping node's Content alternates keys and values.
	for i := 0; i < len(over.Content); i += 2 {
		key, value := over.Content[i], over.Content[i+1]
		j := findKey(result, key.Value)
		if j < 0 {
			result.Content = append(result.Content, key, value)
			continue
		}
		merged, err := merge(result.Content[j+1], value, joinPath(path, key.Value))
		if err != nil {
			return nil, err
		}
		result.Content[j+1] = merged
	}
	return result, nil
}

// mergeSequences joins two sequence nodes, leaving out the items of over
// that base already has.
func mergeSequences(base, over *yaml.Node) *yaml.Node {
	result := shallowCopy(base)
	result.Content = append([]*yaml.Node(nil), base.Content...)
	for _, item := range over.Content {
		if !containsNode(result.Content, item) {
			result.Content = append(result.Content, item)
		}
	}
	return result
}

func findKey(mapping *yaml.Node, key string) int {
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func containsNode(nodes []*yaml.Node, n *yaml.Node) bool {
	for _, m := range nodes {
		if equalNodes(m, n) {
			return true
		}
	}
	return false
}

// equalNodes compares two nodes by content, ignoring style, comments and
// position. Mappings are compared in order, which is enough for list items.
func equalNodes(a, b *yaml.Node) bool {
	if a.Kind != b.Kind {
		return false
	}
	if a.Kind == yaml.ScalarNode {
		return a.Value == b.Value
	}
	if len(a.Content) != len(b.Content) {
		return false
	}
	for i := range a.Content {
		if !equalNodes(a.Content[i], b.Content[i]) {
			return false
		}
	}
	return true
}

func isNull(n *yaml.Node) bool {
	return n == nil || (n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null")
}

func shallowCopy(n *yaml.Node) *yaml.Node {
	c := *n
	return &c
}

func kindName(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		return "a dictionary"
	case yaml.SequenceNode:
		return "a list"
	default:
		return "a single value"
	}
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func describePath(path string) string {
	if path == "" {
		return "the top level"
	}
	return path
}
