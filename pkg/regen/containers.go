package regen

import (
	"bytes"
	"sort"

	"go.yaml.in/yaml/v3"
)

// Containers and init containers were values lists once; they are maps by
// name now, with an order list (containers/containerOrder and
// initContainers/initContainerOrder). A values file written by a generator of
// the old shape is converted before the 3-way merge, so the user's edits meet
// the new generation on the same keys instead of a list against a map.

// containerKeys are the keys that hold containers, with the key of their
// order list.
var containerKeys = map[string]string{
	"containers":     "containerOrder",
	"initContainers": "initContainerOrder",
}

// migrateContainerLists converts the container lists of base and ours to the
// map form, at the places where theirs (the new generation) has the map form.
// Input that is not a single plain YAML document, or has nothing to convert,
// is returned as it is, byte for byte.
func migrateContainerLists(base, ours, theirs []byte) ([]byte, []byte) {
	root := parseValuesRoot(theirs)
	if root == nil {
		return base, ours
	}
	var parents [][]string
	collectContainerMaps(root, nil, &parents)
	if len(parents) == 0 {
		return base, ours
	}
	return convertContainerLists(base, parents), convertContainerLists(ours, parents)
}

// parseValuesRoot returns the root mapping of a single-document YAML without
// anchors, or nil.
func parseValuesRoot(data []byte) *yaml.Node {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode || hasAnchor(root) {
		return nil
	}
	return root
}

func hasAnchor(n *yaml.Node) bool {
	if n.Anchor != "" || n.Alias != nil {
		return true
	}
	for _, c := range n.Content {
		if hasAnchor(c) {
			return true
		}
	}
	return false
}

// collectContainerMaps records the path of every mapping in n that has a
// containers or initContainers key holding a mapping.
func collectContainerMaps(n *yaml.Node, path []string, out *[][]string) {
	if n.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i].Value, n.Content[i+1]
		if _, isContainers := containerKeys[key]; isContainers && val.Kind == yaml.MappingNode {
			*out = append(*out, append([]string(nil), path...))
		}
		collectContainerMaps(val, append(append([]string(nil), path...), key), out)
	}
}

// convertContainerLists converts the container lists of data found under the
// given parent paths; data comes back unchanged when nothing was converted.
//
// Only the converted entries are rewritten: the rest of the file (comments,
// blank lines, the user's indentation) stays byte for byte, so the first
// --update does not produce a diff of the whole values file. A list the
// splice cannot locate (flow style, CRLF) falls back to re-encoding the
// document.
func convertContainerLists(data []byte, parents [][]string) []byte {
	if len(data) == 0 {
		return data
	}
	root := parseValuesRoot(data)
	if root == nil {
		return data
	}
	var edits []containerEdit
	for _, path := range parents {
		parent := root
		for _, key := range path {
			parent = mappingValue(parent, key)
			if parent == nil {
				break
			}
		}
		if parent == nil || parent.Kind != yaml.MappingNode {
			continue
		}
		for _, key := range []string{"initContainers", "containers"} {
			if edit, ok := listToMap(parent, key, containerKeys[key]); ok {
				edits = append(edits, edit)
			}
		}
	}
	if len(edits) == 0 {
		return data
	}
	if out, ok := spliceContainerEdits(data, edits); ok {
		return out
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	enc.CompactSeqIndent()
	if err := enc.Encode(root); err != nil {
		return data
	}
	_ = enc.Close()
	return buf.Bytes()
}

// containerEdit is one converted list: where it was, and what replaces it.
type containerEdit struct {
	key         *yaml.Node // the key node, as parsed
	keyLine     int        // 1-based line of the key
	keyCol      int        // 1-based column of the key
	valLine     int        // 1-based line where the list started
	valCol      int        // 1-based column of its first item
	flow        bool       // the list was written in flow style
	byName      *yaml.Node // the mapping that replaces the list
	orderKey    *yaml.Node // the order list entry that was added, or nil
	orderValue  *yaml.Node
	compactList bool // the list's items sat at the column of the key
}

// spliceContainerEdits replaces the lines of each converted list with the
// encoded mapping (and the order list that goes with it), leaving every other
// byte of data alone. It reports false when it cannot place an edit.
func spliceContainerEdits(data []byte, edits []containerEdit) ([]byte, bool) {
	if bytes.Contains(data, []byte("\r")) {
		return nil, false
	}
	lines := bytes.SplitAfter(data, []byte("\n"))
	type span struct {
		start, end int // line indexes, end exclusive
		text       []byte
	}
	spans := make([]span, 0, len(edits))
	for _, e := range edits {
		if e.flow || e.valLine <= e.keyLine || e.keyLine < 1 || e.keyLine > len(lines) {
			return nil, false
		}
		start := e.keyLine - 1
		end := start + 1
		for i := start + 1; i < len(lines); i++ {
			line := bytes.TrimRight(lines[i], "\n")
			if len(bytes.TrimSpace(line)) == 0 {
				continue // blank: part of the block only when more of it follows
			}
			indent := len(line) - len(bytes.TrimLeft(line, " "))
			dash := bytes.HasPrefix(bytes.TrimLeft(line, " "), []byte("-"))
			if indent+1 > e.keyCol || (indent+1 == e.keyCol && dash) {
				end = i + 1
				continue
			}
			break
		}
		text, ok := encodeContainerEdit(e, lines[end-1])
		if !ok {
			return nil, false
		}
		spans = append(spans, span{start, end, text})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	var out bytes.Buffer
	next := 0
	for _, sp := range spans {
		if sp.start < next {
			return nil, false // overlapping edits
		}
		for _, l := range lines[next:sp.start] {
			out.Write(l)
		}
		out.Write(sp.text)
		next = sp.end
	}
	for _, l := range lines[next:] {
		out.Write(l)
	}
	return out.Bytes(), true
}

// encodeContainerEdit renders the converted entry at the column of its key,
// in the sequence style the list had. lastLine is the last line the list
// occupied: the encoded text ends with a newline exactly as that line does.
func encodeContainerEdit(e containerEdit, lastLine []byte) ([]byte, bool) {
	key := *e.key
	key.HeadComment, key.FootComment = "", "" // stay where they are, above the entry
	entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{&key, e.byName}}
	if e.orderKey != nil {
		entry.Content = append(entry.Content, e.orderKey, e.orderValue)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if e.compactList {
		enc.CompactSeqIndent()
	}
	if err := enc.Encode(entry); err != nil {
		return nil, false
	}
	if err := enc.Close(); err != nil {
		return nil, false
	}
	pad := bytes.Repeat([]byte(" "), e.keyCol-1)
	var out bytes.Buffer
	for _, l := range bytes.SplitAfter(buf.Bytes(), []byte("\n")) {
		if len(bytes.TrimSpace(l)) > 0 {
			out.Write(pad)
		}
		out.Write(l)
	}
	text := out.Bytes()
	if !bytes.HasSuffix(lastLine, []byte("\n")) {
		text = bytes.TrimSuffix(text, []byte("\n"))
	}
	return text, true
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// listToMap turns parent[key], a list of mappings with unique scalar names,
// into a mapping by name and adds the order list parent[orderKey] (unless
// there is one). It returns what changed, and false when it converted nothing.
func listToMap(parent *yaml.Node, key, orderKey string) (containerEdit, bool) {
	var edit containerEdit
	var keyNode *yaml.Node
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			keyNode = parent.Content[i]
			break
		}
	}
	list := mappingValue(parent, key)
	if keyNode == nil || list == nil || list.Kind != yaml.SequenceNode || len(list.Content) == 0 {
		return edit, false
	}
	seen := map[string]bool{}
	names := make([]*yaml.Node, 0, len(list.Content))
	for _, item := range list.Content {
		name := nameNode(item)
		if name == nil || seen[name.Value] {
			return edit, false
		}
		seen[name.Value] = true
		names = append(names, name)
	}
	edit = containerEdit{
		key: keyNode, keyLine: keyNode.Line, keyCol: keyNode.Column,
		valLine: list.Line, valCol: list.Column,
		flow:        list.Style&yaml.FlowStyle != 0,
		compactList: list.Column == keyNode.Column,
	}

	byName := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	order := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for i, item := range list.Content {
		// The name moves from the item to its key.
		rest := make([]*yaml.Node, 0, len(item.Content))
		for j := 0; j+1 < len(item.Content); j += 2 {
			if item.Content[j+1] != names[i] {
				rest = append(rest, item.Content[j], item.Content[j+1])
			}
		}
		item.Content = rest
		byName.Content = append(byName.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: names[i].Value}, item)
		order.Content = append(order.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: names[i].Value})
	}
	edit.byName = byName

	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			parent.Content[i+1] = byName
			break
		}
	}
	if mappingValue(parent, orderKey) == nil {
		edit.orderKey = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: orderKey}
		edit.orderValue = order
		parent.Content = append(parent.Content, edit.orderKey, order)
	}
	return edit, true
}

// nameNode returns the value node of the scalar "name" of a mapping item.
func nameNode(item *yaml.Node) *yaml.Node {
	if item.Kind != yaml.MappingNode {
		return nil
	}
	if v := mappingValue(item, "name"); v != nil && v.Kind == yaml.ScalarNode && v.Value != "" {
		return v
	}
	return nil
}
