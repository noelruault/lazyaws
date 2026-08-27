package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// renamedAwayKeys is deliberately closed so unknown keys from newer versions survive.
var renamedAwayKeys = []string{
	"amazonQ",
}

func SetBoolSetting(path []string, value bool) error {
	text := "false"
	if value {
		text = "true"
	}

	return setScalarSetting(path, text, "!!bool")
}

func SetStringSetting(path []string, value string) error {
	return setScalarSetting(path, value, "!!str")
}

// SetIntSetting writes a number, not a quoted one: an int field unmarshals from !!int and rejects the !!str a string write would leave behind, so the next load of the file would fail on the key the Settings screen had just saved.
func SetIntSetting(path []string, value int) error {
	return setScalarSetting(path, strconv.Itoa(value), "!!int")
}

// setScalarSetting edits YAML nodes so comments, ordering, and unknown keys survive.
func setScalarSetting(path []string, value, tag string) error {
	if len(path) == 0 {
		return errors.New("empty setting path")
	}

	root, err := readConfigNode()
	if err != nil {
		return err
	}

	mapping := root.Content[0]
	for _, key := range path[:len(path)-1] {
		mapping = childNode(mapping, key, yaml.MappingNode, "!!map")
	}

	leaf := childNode(mapping, path[len(path)-1], yaml.ScalarNode, tag)
	leaf.Kind = yaml.ScalarNode
	leaf.Tag = tag
	leaf.Style = 0
	leaf.Content = nil
	leaf.Value = value

	dropRenamedKeys(root.Content[0])

	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}

	return writeConfigFile(out)
}

// dropRenamedKeys preserves a removed key's head comment because yaml.v3 may attach the file header to it.
func dropRenamedKeys(mapping *yaml.Node) {
	for _, dead := range renamedAwayKeys {
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value != dead {
				continue
			}

			comment := mapping.Content[i].HeadComment
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			if comment != "" && i < len(mapping.Content) {
				next := mapping.Content[i]
				if next.HeadComment == "" {
					next.HeadComment = comment
				} else {
					next.HeadComment = comment + "\n" + next.HeadComment
				}
			}
			break
		}
	}
}

func readConfigNode() (*yaml.Node, error) {
	empty := &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}},
	}

	data, err := os.ReadFile(ConfigFilename())
	if err != nil {
		if os.IsNotExist(err) {
			return empty, nil
		}
		return nil, err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	// An empty or comment-only file unmarshals to a node with nothing in it.
	if root.Kind == 0 || len(root.Content) == 0 {
		return empty, nil
	}
	if root.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("config file is not a YAML mapping")
	}

	return &root, nil
}

// childNode steps by two because yaml.Node mappings alternate keys and values.
func childNode(mapping *yaml.Node, key string, kind yaml.Kind, tag string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			value := mapping.Content[i+1]
			// A key that exists but holds the wrong shape (say `amazonQ: true` where a mapping belongs) is replaced rather than walked into.
			if value.Kind != kind {
				value.Kind = kind
				value.Tag = tag
				value.Value = ""
				value.Content = nil
			}
			return value
		}
	}

	value := &yaml.Node{Kind: kind, Tag: tag}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)

	return value
}

// writeConfigFile renames within one directory so interrupted writes cannot leave partial configuration.
func writeConfigFile(data []byte) error {
	path := ConfigFilename()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".config.yml.*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}

	return os.Rename(temp.Name(), path)
}
