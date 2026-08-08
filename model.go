package main

import (
	"encoding/json"
	"os"
	"time"
)

// Job is the normalized representation used across all sources.
type Job struct {
	Source   string    // "Remotive", "RemoteOK", "Arbeitnow"
	ID       string    // source-native id
	Title    string    // role title
	Company  string    // company name
	Location string    // raw location string from the source (may be empty)
	URL      string    // apply/detail link
	Salary   string    // formatted salary if the source provides one, else ""
	JobType  string    // Full-time / Part-time / Contract / Freelance (best effort)
	Reason   string    // LLM one-line "why it fits" (only when LLM ranking is on)
	Tags     []string  // skill/tech tags
	Posted   time.Time // publication time (best effort)

	RemoteHint bool // true if the source asserts the role is remote
	score      int  // computed relevance score (higher = better)
}

// Key uniquely identifies a job for cross-run deduplication.
func (j Job) Key() string { return j.Source + ":" + j.ID }

// Config holds tunable matching rules, loaded from config.json.
type Config struct {
	MaxJobs                  int      `json:"maxJobs"`
	MaxPerCompany            int      `json:"maxPerCompany"`
	AllowCountryLockedRemote bool     `json:"allowCountryLockedRemote"`
	Profile                  string   `json:"profile"` // free-text candidate description for LLM ranking
	Skills                   []string `json:"skills"`
	ReachableRegions         []string `json:"reachableRegions"`
	ExcludeTitle             []string `json:"excludeTitle"`
	ExcludeCompany           []string `json:"excludeCompany"`
}

// Companies lists career-portal (ATS) tokens to pull directly. Each token is
// the company's board slug on that ATS. Add your own targets here.
type Companies struct {
	Greenhouse []string `json:"greenhouse"`
	Ashby      []string `json:"ashby"`
	Lever      []string `json:"lever"`
}

func loadCompanies(path string) (Companies, error) {
	var c Companies
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(b, &c)
}

func loadConfig(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.MaxJobs == 0 {
		c.MaxJobs = 5
	}
	return c, nil
}
