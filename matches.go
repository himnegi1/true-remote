package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// matchOut is the dashboard-facing shape of a job (stable JSON field names).
type matchOut struct {
	Title    string   `json:"title"`
	Company  string   `json:"company"`
	Location string   `json:"location"`
	JobType  string   `json:"jobType"`
	Salary   string   `json:"salary"`
	Source   string   `json:"source"`
	URL      string   `json:"url"`
	Reason   string   `json:"reason"`
	Tags     []string `json:"tags"`
	Posted   string   `json:"posted"`
}

type matchesFile struct {
	Generated string     `json:"generated"`
	Skills    []string   `json:"skills"`
	Count     int        `json:"count"`
	Jobs      []matchOut `json:"jobs"`
}

// writeMatches persists the selected jobs to a JSON file the dashboard reads.
func writeMatches(path string, jobs []Job, skills []string) error {
	out := matchesFile{
		Generated: time.Now().UTC().Format(time.RFC3339),
		Skills:    skills,
		Count:     len(jobs),
		Jobs:      make([]matchOut, 0, len(jobs)),
	}
	for _, j := range jobs {
		posted := ""
		if !j.Posted.IsZero() {
			posted = j.Posted.UTC().Format("2006-01-02")
		}
		out.Jobs = append(out.Jobs, matchOut{
			Title:    j.Title,
			Company:  j.Company,
			Location: j.Location,
			JobType:  j.JobType,
			Salary:   j.Salary,
			Source:   j.Source,
			URL:      j.URL,
			Reason:   j.Reason,
			Tags:     j.Tags,
			Posted:   posted,
		})
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return os.WriteFile(path, b, 0o644)
}
