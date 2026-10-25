package merge

import (
	"bytes"
	"encoding/json"
	"reflect"

	"go.yaml.in/yaml/v3"
)

type YAMLConflict struct {
	Path               string
	Base, Ours, Theirs string
	Kind               string
}

type YAMLResult struct {
	Content     []byte
	Conflicts   []YAMLConflict
	Reformatted bool
}

type Labels struct {
	Ours, Base, Theirs string
}

type YAMLOptions struct {
	Labels   Labels
	ShowBase bool
}

func parseYAML(b []byte) (*yaml.Node, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(b, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

func eq(x, y *yaml.Node) bool {
	if x == nil && y == nil {
		return true
	}
	if x == nil || y == nil {
		return false
	}
	var vx, vy interface{}
	_ = x.Decode(&vx)
	_ = y.Decode(&vy)
	return reflect.DeepEqual(vx, vy)
}

func hasAnchors(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	if n.Anchor != "" || n.Alias != nil {
		return true
	}
	for _, c := range n.Content {
		if hasAnchors(c) {
			return true
		}
	}
	return false
}

// MergeYAML merges three YAML documents structurally.
func MergeYAML(base, ours, theirs []byte, opts YAMLOptions) (MergeResult, error) {
	// 1. Text diff3
	textRes := Merge3(string(base), string(ours), string(theirs), Merge3Options{ShowBase: opts.ShowBase})

	// Try parse
	bn, bErr := parseYAML(base)
	on, oErr := parseYAML(ours)
	tn, tErr := parseYAML(theirs)

	structuralFail := false
	if bErr != nil || oErr != nil || tErr != nil {
		structuralFail = true
	} else if (bn != nil && len(bn.Content) > 1) || (on != nil && len(on.Content) > 1) || (tn != nil && len(tn.Content) > 1) {
		structuralFail = true // multiple docs
	} else if hasAnchors(bn) || hasAnchors(on) || hasAnchors(tn) {
		structuralFail = true
	} else if bn != nil && len(bn.Content) > 0 && bn.Content[0].Kind != yaml.MappingNode {
		structuralFail = true
	}

	if structuralFail {
		return MergeResult{
			Content:       textRes.Content,
			Conflict:      textRes.Conflict,
			ConflictCount: textRes.ConflictCount,
			Conflicts:     textRes.Conflicts,
		}, nil
	}

	var bRoot, oRoot, tRoot *yaml.Node
	if bn != nil && len(bn.Content) > 0 {
		bRoot = bn.Content[0]
	}
	if on != nil && len(on.Content) > 0 {
		oRoot = on.Content[0]
	}
	if tn != nil && len(tn.Content) > 0 {
		tRoot = tn.Content[0]
	}

	var yamlConflicts []YAMLConflict
	resNode := mergeNodes("", bRoot, oRoot, tRoot, &yamlConflicts)

	// Build result document
	resDoc := &yaml.Node{Kind: yaml.DocumentNode}
	if resNode != nil {
		resDoc.Content = append(resDoc.Content, resNode)
	}

	// Encode structural
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(resDoc)
	enc.Close()
	sBytes := buf.Bytes()

	// Check if text diff3 gives identical semantic result and is clean
	if textRes.ConflictCount == 0 && len(yamlConflicts) == 0 {
		txtNode, tErr := parseYAML([]byte(textRes.Content))
		if tErr == nil && txtNode != nil && len(txtNode.Content) > 0 {
			if eq(txtNode.Content[0], resNode) {
				// Perfect semantic match, use user text
				return textRes, nil
			}
		}
	}

	// Structural output
	// But note: EPIC-06 design says structural conflicts should add dhg-conflict comments.
	// We did that inside mergeNodes.
	// But it also says YAMLMerge returns a YAMLResult... Wait, the spec says MergeYAML(..., YAMLOptions) (MergeResult, error).
	// In the diff task I used TextResult. For YAML, I'll return MergeResult, ignoring the specific YAML conflicts slice since MergeResult has no place for it... Wait, MergeResult has `Conflicts []ConflictHunk`.

	var ch []ConflictHunk
	for _, yc := range yamlConflicts {
		ch = append(ch, ConflictHunk{
			Base:   []string{yc.Base},
			Ours:   []string{yc.Ours},
			Theirs: []string{yc.Theirs},
		})
	}
	return MergeResult{
		Content:       string(sBytes),
		Conflict:      len(yamlConflicts) > 0,
		ConflictCount: len(yamlConflicts),
		Conflicts:     ch,
	}, nil
}

func marshalJSON1(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	var v interface{}
	_ = n.Decode(&v)
	b, _ := json.Marshal(v)
	s := string(b)
	if len(s) > 120 {
		s = s[:119] + "…"
	}
	return s
}

func addConflictComment(keyNode, oNode *yaml.Node, b, t *yaml.Node) {
	bs := marshalJSON1(b)
	ts := marshalJSON1(t)
	os := marshalJSON1(oNode)
	msg := "# dhg-conflict: generator changed this value from " + bs + " to " + ts + "; your value " + os + " is kept (see MERGE.md)"
	if keyNode.HeadComment != "" {
		keyNode.HeadComment += "\n" + msg
	} else {
		keyNode.HeadComment = msg
	}
}

func mergeNodes(path string, b, o, t *yaml.Node, conflicts *[]YAMLConflict) *yaml.Node {
	if eq(o, t) {
		return o
	}
	if eq(o, b) {
		if t == nil {
			return nil
		}
		return t
	}
	if eq(t, b) {
		if o == nil {
			return nil
		}
		return o
	}

	if isMap(o) && isMap(t) && (b == nil || isMap(b)) {
		return mergeMaps(path, b, o, t, conflicts)
	}

	if isKeyedList(b, o, t) {
		return mergeKeyedLists(path, b, o, t, conflicts)
	}

	// Scalar conflict
	kind := "value"
	if b == nil {
		kind = "add-add"
	} else if o == nil {
		kind = "delete-modify"
	} else if t == nil {
		kind = "modify-delete"
	}

	*conflicts = append(*conflicts, YAMLConflict{
		Path:   path,
		Base:   marshalJSON1(b),
		Ours:   marshalJSON1(o),
		Theirs: marshalJSON1(t),
		Kind:   kind,
	})

	if o != nil {
		return o // handled by caller if it's a map key
	}
	return o
}

func isMap(n *yaml.Node) bool {
	return n != nil && n.Kind == yaml.MappingNode
}

func mergeMaps(path string, b, o, t *yaml.Node, conflicts *[]YAMLConflict) *yaml.Node {
	res := *o
	res.Content = nil

	oKeys := make(map[string]*yaml.Node)
	oVals := make(map[string]*yaml.Node)
	var oOrder []string
	for i := 0; i < len(o.Content); i += 2 {
		k := o.Content[i]
		v := o.Content[i+1]
		oKeys[k.Value] = k
		oVals[k.Value] = v
		oOrder = append(oOrder, k.Value)
	}

	bVals := make(map[string]*yaml.Node)
	if b != nil {
		for i := 0; i < len(b.Content); i += 2 {
			k := b.Content[i]
			v := b.Content[i+1]
			bVals[k.Value] = v
		}
	}

	tKeys := make(map[string]*yaml.Node)
	tVals := make(map[string]*yaml.Node)
	var tOrder []string
	for i := 0; i < len(t.Content); i += 2 {
		k := t.Content[i]
		v := t.Content[i+1]
		tKeys[k.Value] = k
		tVals[k.Value] = v
		tOrder = append(tOrder, k.Value)
	}

	seen := make(map[string]bool)

	// o order
	for _, kStr := range oOrder {
		seen[kStr] = true
		kNode := oKeys[kStr]
		oNode := oVals[kStr]
		bNode := bVals[kStr]
		tNode := tVals[kStr]

		subPath := kStr
		if path != "" {
			subPath = path + "." + kStr
		}

		merged := mergeNodes(subPath, bNode, oNode, tNode, conflicts)
		if merged != nil {
			if merged == oNode && oNode != nil && tNode != nil && bNode != nil && !eq(oNode, tNode) && !eq(oNode, bNode) && !eq(tNode, bNode) {
				if !isMap(oNode) && !isKeyedList(bNode, oNode, tNode) {
					addConflictComment(kNode, oNode, bNode, tNode)
				}
			}
			res.Content = append(res.Content, kNode, merged)
		}
	}

	// t new keys
	for _, kStr := range tOrder {
		if !seen[kStr] {
			kNode := tKeys[kStr]
			tNode := tVals[kStr]
			subPath := kStr
			if path != "" {
				subPath = path + "." + kStr
			}

			merged := mergeNodes(subPath, nil, nil, tNode, conflicts)
			if merged != nil {
				res.Content = append(res.Content, kNode, merged)
			}
		}
	}

	return &res
}

func isKeyedList(b, o, t *yaml.Node) bool {
	check := func(n *yaml.Node) bool {
		if n == nil {
			return true
		}
		if n.Kind != yaml.SequenceNode {
			return false
		}
		names := make(map[string]bool)
		for _, c := range n.Content {
			if c.Kind != yaml.MappingNode {
				return false
			}
			hasName := false
			for i := 0; i < len(c.Content); i += 2 {
				if c.Content[i].Value == "name" && c.Content[i+1].Kind == yaml.ScalarNode {
					name := c.Content[i+1].Value
					if names[name] {
						return false
					} // duplicate
					names[name] = true
					hasName = true
					break
				}
			}
			if !hasName {
				return false
			}
		}
		return true
	}
	return check(b) && check(o) && check(t) && (b != nil || o != nil || t != nil) &&
		((b != nil && b.Kind == yaml.SequenceNode) || (o != nil && o.Kind == yaml.SequenceNode) || (t != nil && t.Kind == yaml.SequenceNode))
}

func mergeKeyedLists(path string, b, o, t *yaml.Node, conflicts *[]YAMLConflict) *yaml.Node {
	res := *o
	if o == nil {
		res = *t
	}
	res.Content = nil

	getMap := func(n *yaml.Node) (map[string]*yaml.Node, []string) {
		m := make(map[string]*yaml.Node)
		var order []string
		if n == nil {
			return m, order
		}
		for _, c := range n.Content {
			for i := 0; i < len(c.Content); i += 2 {
				if c.Content[i].Value == "name" {
					m[c.Content[i+1].Value] = c
					order = append(order, c.Content[i+1].Value)
					break
				}
			}
		}
		return m, order
	}

	bMap, _ := getMap(b)
	oMap, oOrder := getMap(o)
	tMap, tOrder := getMap(t)

	seen := make(map[string]bool)
	for _, name := range oOrder {
		seen[name] = true
		subPath := path + "[name=" + name + "]"
		merged := mergeNodes(subPath, bMap[name], oMap[name], tMap[name], conflicts)
		if merged != nil {
			// Find name node to add comment
			var nameKeyNode *yaml.Node
			for i := 0; i < len(merged.Content); i += 2 {
				if merged.Content[i].Value == "name" {
					nameKeyNode = merged.Content[i]
					break
				}
			}
			if nameKeyNode != nil {
				if merged == oMap[name] && oMap[name] != nil && tMap[name] != nil && bMap[name] != nil && !eq(oMap[name], tMap[name]) && !eq(oMap[name], bMap[name]) && !eq(tMap[name], bMap[name]) {
					addConflictComment(nameKeyNode, oMap[name], bMap[name], tMap[name])
				}
			}
			res.Content = append(res.Content, merged)
		}
	}

	for _, name := range tOrder {
		if !seen[name] {
			subPath := path + "[name=" + name + "]"
			merged := mergeNodes(subPath, nil, nil, tMap[name], conflicts)
			if merged != nil {
				res.Content = append(res.Content, merged)
			}
		}
	}

	return &res
}
