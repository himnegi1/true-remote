package main

import (
	"encoding/json"
	"os"
)

const seenPath = "data/seen.json"
const seenCap = 3000 // bound the file growth; oldest keys drop off

type seenFile struct {
	Keys []string `json:"keys"`
}

func loadSeen() (map[string]bool, []string, error) {
	set := map[string]bool{}
	b, err := os.ReadFile(seenPath)
	if os.IsNotExist(err) {
		return set, nil, nil
	}
	if err != nil {
		return set, nil, err
	}
	var sf seenFile
	if err := json.Unmarshal(b, &sf); err != nil {
		return set, nil, err
	}
	for _, k := range sf.Keys {
		set[k] = true
	}
	return set, sf.Keys, nil
}

// saveSeen appends the newly sent keys (most recent last) and caps the total.
func saveSeen(existing []string, sent []Job) error {
	keys := existing
	for _, j := range sent {
		keys = append(keys, j.Key())
	}
	if len(keys) > seenCap {
		keys = keys[len(keys)-seenCap:]
	}
	b, err := json.MarshalIndent(seenFile{Keys: keys}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(seenPath, b, 0o644)
}
