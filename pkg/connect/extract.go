package connect

func Extract(entries []KeyValue) []Candidate {
	var candidates []Candidate

	for _, entry := range entries {
		if MatchProperty(entry.Key) || MatchEnv(entry.Key) {
			endpoints := ParseValue(entry.Value)
			if len(endpoints) > 0 {
				candidates = append(candidates, Candidate{
					Key:       entry.Key,
					Value:     entry.Value, // In a real implementation this should be redacted
					Endpoints: endpoints,
				})
			}
		}
	}

	return candidates
}
