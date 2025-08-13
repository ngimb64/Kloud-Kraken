package yamlutils

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Finds or creates mapping nodes along path and
// sets the final value as a scalar node.
//
// @Parameters
// - The node to modifiy
// - The node path how it is mapped in yaml
// - The value to set in the node content
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func setNodeValue(node *yaml.Node, path []string, value string) error {
    if len(path) == 0 {
        return errors.New("empty path")
    }

    // If current node is a DocumentNode, descend into Content[0]
    if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
        node = node.Content[0]
    }

    // If at mapping level, we search keys
    if node.Kind != yaml.MappingNode {
        // convert to mapping node (will lose previous non-mapping content at this branch)
        node.Kind = yaml.MappingNode
        node.Tag = "!!map"
        node.Content = []*yaml.Node{}
    }

    curr := node

    for i, key := range path {
        // Check to see if current iteration is the last
        isLast := i == len(path)-1
        foundIdx := -1

        // Find key in current mapping node content
        for j := 0; j < len(curr.Content); j += 2 {
            k := curr.Content[j]
            if k.Kind == yaml.ScalarNode && k.Value == key {
                foundIdx = j
                break
            }
        }

        // If the key was not found
        if foundIdx == -1 {
            // Create key and value nodes
            keyNode := &yaml.Node{
                Kind:  yaml.ScalarNode,
                Tag:   "!!str",
                Value: key,
            }

            var valNode *yaml.Node

            // If the node is the last
            if isLast {
                valNode = &yaml.Node{
                    Kind:  yaml.ScalarNode,
                    Tag:   "!!str",
                    Value: value,
                }

                curr.Content = append(curr.Content, keyNode, valNode)
                return nil
            }

            // Create an inner mapping node
            valNode = &yaml.Node{
                Kind:    yaml.MappingNode,
                Tag:     "!!map",
                Content: []*yaml.Node{},
            }

            curr.Content = append(curr.Content, keyNode, valNode)
            curr = valNode
            continue
        }

        // Key found meaning value node sits at foundIdx+1
        valNode := curr.Content[foundIdx+1]
        // If the node is the last
        if isLast {
            // Replace value node with scalar node containing new value
            *valNode = yaml.Node{
                Kind:  yaml.ScalarNode,
                Tag:   "!!str",
                Value: value,
            }

            return nil
        }

        // not last: descend. If the value node is not a mapping, convert it to mapping.
        if valNode.Kind != yaml.MappingNode {
            // replace the node with an empty mapping to allow nested keys
            *valNode = yaml.Node{
                Kind:    yaml.MappingNode,
                Tag:     "!!map",
                Content: []*yaml.Node{},
            }
        }

        curr = valNode
    }

    return nil
}


// Splits on '.' except backslash for keys that contain dots.
// e.g. "a.b\.c.d" -> ["a","b.c","d"]
//
// @Parameters
// - The yaml path to split
//
// @Returns
// - A slice of the split yaml path content
//
func splitPath(yamlPath string) []string {
    parts := []string{}
    var cur strings.Builder
    escaped := false

    // Iterate through each rune in path
    for _, r := range yamlPath {
        if escaped {
            cur.WriteRune(r)
            escaped = false
            continue
        }

        if r == '\\' {
            escaped = true
            continue
        }

        if r == '.' {
            parts = append(parts, cur.String())
            cur.Reset()
            continue
        }

        cur.WriteRune(r)
    }

    return append(parts, cur.String())
}


// Updates all the key-value mapping in passed in map to passed in yaml data.
//
// @Parameters
// - yamlBytes:  Slice of raw yaml bytes to be modified
// - updates:  Map containing yaml data to be updated
//
// @Returns
// - The resulting modified yaml data
// - Error if it occurs, otherwise nil on success
//
func UpdateYAMLBytes(yamlBytes []byte,
                     updates map[string]string) (
                     _ []byte, err error) {
    var doc yaml.Node

    // Decode the data in yaml node for modification
    err = yaml.Unmarshal(yamlBytes, &doc)
    if err != nil {
        return nil, fmt.Errorf("unmarshal yaml:  %w", err)
    }

    // Ensure a DocumentNode with content is present
    if len(doc.Content) == 0 {
        return nil, errors.New("empty yaml document")
    }

    // Grab the root node
    root := doc.Content[0]

    // Iterate through the key-value mappings in map
    for rawPath, newVal := range updates {
        // Split any . in path with exception of \ escaped, return as slice
        path := splitPath(rawPath)

        // Set the node value with one parsed from map
        err = setNodeValue(root, path, newVal)
        if err != nil {
            return nil, fmt.Errorf("set %s:  %w", rawPath, err)
        }
    }

    var buffer bytes.Buffer
    // Make new encoder
    enc := yaml.NewEncoder(&buffer)

    defer func() {
        // Close the encoder
        cerr := enc.Close()
        if cerr != nil {
            err = errors.Join(err, fmt.Errorf("closing encoder:  %w", cerr))
        }
    }()

    // Set the encoding indentation to 2 spaces for yaml
    enc.SetIndent(2)
    // Encode the resulting data back to yaml
    err = enc.Encode(&doc)
    if err != nil {
        return nil, fmt.Errorf("encode yaml:  %w", err)
    }

    return buffer.Bytes(), nil
}
