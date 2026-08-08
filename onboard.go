package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// runOnboard is the "ask the user intelligently" step: it takes a plain-English
// description of what someone is looking for and uses the LLM to turn it into
// the structured fields TrueRemote needs (skills + profile). It prints the
// result for the user to drop into config.json — it never overwrites their
// file automatically.
func runOnboard(desc string) error {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return fmt.Errorf("usage: go run . --onboard \"describe your ideal role in plain English\"")
	}
	if !llmEnabled() {
		return fmt.Errorf("onboarding needs an LLM: set LLM_API_KEY (free Groq key works)")
	}

	system := "You configure a remote-job search from a candidate's description. " +
		"Return ONLY a JSON object: " +
		`{"skills":["<searchable tech keywords, lowercase, e.g. golang, c++, rust>"],` +
		`"profile":"<one tight paragraph capturing skills, seniority, location/pay/remote needs, and interests/exclusions>"}. ` +
		"skills must be concrete job-search keywords (languages/frameworks), 2-8 of them."

	raw, err := llmChat(system, desc, true)
	if err != nil {
		return err
	}

	var parsed struct {
		Skills  []string `json:"skills"`
		Profile string   `json:"profile"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &parsed); err != nil {
		return fmt.Errorf("parse LLM response: %w\nraw: %s", err, raw)
	}

	out, _ := json.MarshalIndent(map[string]any{
		"skills":  parsed.Skills,
		"profile": parsed.Profile,
	}, "", "  ")

	fmt.Println("----- Suggested config (paste these keys into config.json) -----")
	fmt.Println(string(out))
	fmt.Println("---------------------------------------------------------------")
	fmt.Println("Tip: adjust reachableRegions / excludeTitle / excludeCompany in config.json to taste.")
	return nil
}
