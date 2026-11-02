package pattern

import (
	"sort"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// containersOf returns the containers stored under field ("containers" or
// "initContainers") of processed values. The processors store them as a map
// by container name plus an order list (ADR-054); a plain list of container
// maps is accepted too. For the map form the name is filled in from the key
// and the containers follow the order list (sorted names as a fallback), the
// same way the generator reads them.
//
// ok is false when the field is absent, nil or of an unknown shape.
func containersOf(values map[string]interface{}, field string) (list []map[string]interface{}, ok bool) {
	switch v := values[field].(type) {
	case []map[string]interface{}:
		return v, true
	case []interface{}:
		list = make([]map[string]interface{}, 0, len(v))
		for _, item := range v {
			if c, isMap := item.(map[string]interface{}); isMap {
				list = append(list, c)
			}
		}
		return list, true
	case map[string]interface{}:
		list = make([]map[string]interface{}, 0, len(v))
		for _, name := range containerNames(v, values[orderKeyOf(field)]) {
			c, isMap := v[name].(map[string]interface{})
			if !isMap {
				continue
			}
			named := make(map[string]interface{}, len(c)+1)
			for k, val := range c {
				named[k] = val
			}
			if _, has := named["name"]; !has {
				named["name"] = name
			}
			list = append(list, named)
		}
		return list, true
	}
	return nil, false
}

func orderKeyOf(field string) string {
	if field == "initContainers" {
		return "initContainerOrder"
	}
	return "containerOrder"
}

// containerNames lists the keys of byName: the order list first, then (when
// the list is missing) the sorted keys.
func containerNames(byName map[string]interface{}, order interface{}) []string {
	var names []string
	switch o := order.(type) {
	case []string:
		names = append(names, o...)
	case []interface{}:
		for _, n := range o {
			if s, isStr := n.(string); isStr {
				names = append(names, s)
			}
		}
	default:
		for name := range byName {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	return names
}

// imageOf returns the image of a container as one string: a plain string, or
// repository[:tag] of the {repository, tag} map the processors produce.
func imageOf(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return v
	case map[string]interface{}:
		repo, _ := v["repository"].(string)
		if tag, ok := v["tag"].(string); ok && tag != "" {
			return repo + ":" + tag
		}
		return repo
	}
	return ""
}

// appendOnce appends key unless the list already has it: a resource with
// several offending containers is reported once.
func appendOnce(list []types.ResourceKey, key types.ResourceKey) []types.ResourceKey {
	for _, k := range list {
		if k == key {
			return list
		}
	}
	return append(list, key)
}
