package kafkaclient

// FromContainer is a temporary adapter to extract Property slice from a container definition.
// This is to be replaced by EPIC-03 configuration source.
func FromContainer(container map[string]interface{}, configMaps map[string]map[string]string) []Property {
	var props []Property

	// Handle envFrom (which has lower precedence)
	if envFromList, ok := container["envFrom"].([]interface{}); ok {
		for _, envFrom := range envFromList {
			if envFromMap, ok := envFrom.(map[string]interface{}); ok {
				prefix := ""
				if p, ok := envFromMap["prefix"].(string); ok {
					prefix = p
				}
				if cmRef, ok := envFromMap["configMapRef"].(map[string]interface{}); ok {
					if cmName, ok := cmRef["name"].(string); ok {
						if cmData, ok := configMaps[cmName]; ok {
							for k, v := range cmData {
								props = append(props, Property{
									Key:    prefix + k,
									Value:  v,
									Origin: "configMap " + cmName + " key " + k,
								})
							}
						}
					}
				}
			}
		}
	}

	// Handle env
	if envList, ok := container["env"].([]interface{}); ok {
		for _, env := range envList {
			if envMap, ok := env.(map[string]interface{}); ok {
				if name, ok := envMap["name"].(string); ok {
					if val, ok := envMap["value"].(string); ok {
						props = append(props, Property{
							Key:    name,
							Value:  val,
							Origin: "env " + name,
						})
					} else if valFrom, ok := envMap["valueFrom"].(map[string]interface{}); ok {
						if _, ok := valFrom["secretKeyRef"].(map[string]interface{}); ok {
							props = append(props, Property{
								Key:        name,
								Value:      "",
								FromSecret: true,
								Origin:     "env " + name,
							})
						} else if cmKeyRef, ok := valFrom["configMapKeyRef"].(map[string]interface{}); ok {
							if cmName, ok := cmKeyRef["name"].(string); ok {
								if key, ok := cmKeyRef["key"].(string); ok {
									if cmData, ok := configMaps[cmName]; ok {
										if val, ok := cmData[key]; ok {
											props = append(props, Property{
												Key:    name,
												Value:  val,
												Origin: "env " + name,
											})
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return props
}
