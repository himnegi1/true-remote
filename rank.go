package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// finalPicks turns the heuristic candidate pool into the final list to send.
// With an LLM key it semantically re-ranks against the user's profile; without
// one it falls back to the heuristic order. Either way it returns at most
// cfg.MaxJobs and never fails the run.
func finalPicks(cfg Config, pool []Job) []Job {
	n := cfg.MaxJobs
	if n <= 0 {
		n = 5
	}
	if !llmEnabled() || len(pool) == 0 {
		return trim(pool, n)
	}
	ranked, err := rankWithLLM(cfg, pool, n)
	if err != nil {
		logf("LLM ranking failed (%v) — falling back to heuristic order", err)
		return trim(pool, n)
	}
	logf("LLM ranking applied (%s)", llmModel())
	return ranked
}

func trim(jobs []Job, n int) []Job {
	if len(jobs) > n {
		return jobs[:n]
	}
	return jobs
}

// rankWithLLM asks the model to pick the best-fitting jobs for the profile and
// return their indices with a one-line reason each.
func rankWithLLM(cfg Config, pool []Job, n int) ([]Job, error) {
	profile := strings.TrimSpace(cfg.Profile)
	if profile == "" {
		profile = "A software engineer. Skills: " + strings.Join(cfg.Skills, ", ") + "."
	}

	var sb strings.Builder
	for i, j := range pool {
		snippet := jobSnippet(j)
		fmt.Fprintf(&sb, "[%d] %s @ %s | %s | %s\n%s\n\n",
			i, j.Title, j.Company, orDash(j.Location), orDash(j.JobType), snippet)
	}

	system := "You are a job-matching assistant. Given a candidate profile and a numbered list of remote jobs, " +
		"select the ones that genuinely fit the candidate's skills, seniority, and preferences, best first. " +
		"Prefer roles where the candidate's core skills are central. Ignore roles that are a poor fit even if listed. " +
		"Respond ONLY with a JSON object of the form " +
		`{"picks":[{"index":<int>,"reason":"<max 12 words why it fits>"}]}` +
		fmt.Sprintf(" with at most %d picks, ordered best first.", n)

	user := "CANDIDATE PROFILE:\n" + profile + "\n\nJOBS:\n" + sb.String()

	raw, err := llmChat(system, user, true)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Picks []struct {
			Index  int    `json:"index"`
			Reason string `json:"reason"`
		} `json:"picks"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &parsed); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	var out []Job
	usedIdx := map[int]bool{}
	for _, p := range parsed.Picks {
		if p.Index < 0 || p.Index >= len(pool) || usedIdx[p.Index] {
			continue
		}
		usedIdx[p.Index] = true
		j := pool[p.Index]
		j.Reason = strings.TrimSpace(p.Reason)
		out = append(out, j)
		if len(out) == n {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("LLM returned no valid picks")
	}
	return out, nil
}

// jobSnippet returns a short, clean description excerpt for LLM context.
func jobSnippet(j Job) string {
	for _, t := range j.Tags {
		if strings.HasPrefix(t, "\x00desc:") {
			d := strings.TrimPrefix(t, "\x00desc:")
			if len(d) > 400 {
				d = d[:400]
			}
			return d
		}
	}
	return ""
}

// extractJSON pulls the first {...} object out of a possibly-noisy reply.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
