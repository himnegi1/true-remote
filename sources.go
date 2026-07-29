package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// fetchAll pulls from the remote-job aggregators AND the direct company
// career-portal (ATS) APIs. A single source failing is non-fatal: we log it
// and continue, so a broken/changed endpoint never kills the run.
func fetchAll(cfg Config, companies Companies) []Job {
	var all []Job

	for _, fn := range []struct {
		name string
		run  func(Config) ([]Job, error)
	}{
		{"Remotive", fetchRemotive},
		{"RemoteOK", fetchRemoteOK},
		{"Arbeitnow", fetchArbeitnow},
		{"Jobicy", fetchJobicy},
	} {
		jobs, err := fn.run(cfg)
		if err != nil {
			logf("source %s failed: %v", fn.name, err)
			continue
		}
		logf("source %s returned %d raw jobs", fn.name, len(jobs))
		all = append(all, jobs...)
	}

	// Direct-from-company career portals.
	all = append(all, fetchATS(companies)...)
	return all
}

var reTag = regexp.MustCompile(`<[^>]*>`)

// stripHTML removes tags and collapses whitespace so description matching and
// any display text stays clean.
func stripHTML(s string) string {
	s = reTag.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return strings.Join(strings.Fields(s), " ")
}

// ---------- Remotive ----------
// Docs: https://remotive.com/api-documentation
// ToS: attribute Remotive as source, link back, query at most a few times/day.

type remotiveResp struct {
	Jobs []struct {
		ID                   int      `json:"id"`
		URL                  string   `json:"url"`
		Title                string   `json:"title"`
		CompanyName          string   `json:"company_name"`
		Salary               string   `json:"salary"`
		CandidateRequiredLoc string   `json:"candidate_required_location"`
		Tags                 []string `json:"tags"`
		JobType              string   `json:"job_type"`
		Description          string   `json:"description"`
		PublicationDate      string   `json:"publication_date"`
	} `json:"jobs"`
}

func fetchRemotive(cfg Config) ([]Job, error) {
	var out []Job
	seen := map[int]bool{}
	for _, skill := range cfg.Skills {
		u := "https://remotive.com/api/remote-jobs?limit=100&search=" + url.QueryEscape(skill)
		b, err := httpGet(u)
		if err != nil {
			return out, err
		}
		var r remotiveResp
		if err := json.Unmarshal(b, &r); err != nil {
			return out, fmt.Errorf("decode: %w", err)
		}
		for _, j := range r.Jobs {
			if seen[j.ID] {
				continue
			}
			seen[j.ID] = true
			job := Job{
				Source:     "Remotive",
				ID:         fmt.Sprintf("%d", j.ID),
				Title:      j.Title,
				Company:    j.CompanyName,
				Location:   j.CandidateRequiredLoc,
				URL:        j.URL,
				Salary:     strings.TrimSpace(j.Salary),
				JobType:    normalizeJobType(j.JobType),
				Tags:       j.Tags,
				RemoteHint: true, // Remotive is remote-only
				Posted:     parseTime(j.PublicationDate),
			}
			out = append(out, job.withDescription(stripHTML(j.Description)))
		}
	}
	return out, nil
}

// ---------- Jobicy ----------
// Remote-worldwide board with a clean tag filter, plus geo + job-type fields.
// ToS: credit Jobicy and link to the original job URL (we use job.url as-is).

type jobicyResp struct {
	Jobs []struct {
		ID             json.Number `json:"id"`
		URL            string      `json:"url"`
		JobTitle       string      `json:"jobTitle"`
		CompanyName    string      `json:"companyName"`
		JobType        []string    `json:"jobType"`
		JobGeo         string      `json:"jobGeo"`
		JobExcerpt     string      `json:"jobExcerpt"`
		JobDescription string      `json:"jobDescription"`
		PubDate        string      `json:"pubDate"`
	} `json:"jobs"`
}

func fetchJobicy(cfg Config) ([]Job, error) {
	b, err := httpGet("https://jobicy.com/api/v2/remote-jobs?count=50&tag=golang")
	if err != nil {
		return nil, err
	}
	var r jobicyResp
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	var out []Job
	for _, j := range r.Jobs {
		job := Job{
			Source:     "Jobicy",
			ID:         "jobicy-" + j.ID.String(),
			Title:      j.JobTitle,
			Company:    j.CompanyName,
			Location:   j.JobGeo, // "Anywhere" / "USA" / "Europe, Germany, …"
			URL:        j.URL,
			JobType:    normalizeJobType(strings.Join(j.JobType, " ")),
			RemoteHint: true, // Jobicy is remote-only
			Posted:     parseTime(j.PubDate),
		}
		out = append(out, job.withDescription(stripHTML(j.JobDescription)))
	}
	return out, nil
}

// ---------- RemoteOK ----------
// The first array element is a legal notice and must be skipped.

type remoteOKItem struct {
	ID          string   `json:"id"`
	Position    string   `json:"position"`
	Company     string   `json:"company"`
	Tags        []string `json:"tags"`
	Location    string   `json:"location"`
	URL         string   `json:"url"`
	Epoch       int64    `json:"epoch"`
	SalaryMin   int      `json:"salary_min"`
	SalaryMax   int      `json:"salary_max"`
	Description string   `json:"description"`
	Legal       string   `json:"legal"` // present only on the notice element
}

func fetchRemoteOK(cfg Config) ([]Job, error) {
	b, err := httpGet("https://remoteok.com/api")
	if err != nil {
		return nil, err
	}
	var items []remoteOKItem
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	var out []Job
	for _, it := range items {
		if it.Legal != "" || it.ID == "" { // skip legal notice / malformed
			continue
		}
		// RemoteOK tags are notoriously noisy (it stapled "golang" onto a
		// "Clinical Pharmacist"), so we do NOT trust them for matching or
		// display — matching runs on title + real description instead.
		job := Job{
			Source:     "RemoteOK",
			ID:         it.ID,
			Title:      it.Position,
			Company:    it.Company,
			Location:   it.Location,
			URL:        it.URL,
			Salary:     formatSalaryRange(it.SalaryMin, it.SalaryMax),
			RemoteHint: true, // RemoteOK is remote-only
			Posted:     time.Unix(it.Epoch, 0),
		}
		out = append(out, job.withDescription(stripHTML(it.Description)))
	}
	return out, nil
}

// ---------- Arbeitnow ----------
// Strong EU coverage. General board, so skill-filtering does the heavy lifting.

type arbeitnowResp struct {
	Data []struct {
		Slug        string   `json:"slug"`
		CompanyName string   `json:"company_name"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Remote      bool     `json:"remote"`
		URL         string   `json:"url"`
		Tags        []string `json:"tags"`
		JobTypes    []string `json:"job_types"`
		Location    string   `json:"location"`
		CreatedAt   int64    `json:"created_at"`
	} `json:"data"`
}

func fetchArbeitnow(cfg Config) ([]Job, error) {
	b, err := httpGet("https://www.arbeitnow.com/api/job-board-api")
	if err != nil {
		return nil, err
	}
	var r arbeitnowResp
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	var out []Job
	for _, j := range r.Data {
		if !j.Remote {
			continue
		}
		// Fold description into tags so the skill matcher can see it.
		tags := append([]string{}, j.Tags...)
		out = append(out, Job{
			Source:     "Arbeitnow",
			ID:         j.Slug,
			Title:      j.Title,
			Company:    j.CompanyName,
			Location:   j.Location,
			URL:        j.URL,
			Salary:     "",
			JobType:    normalizeJobType(strings.Join(j.JobTypes, " ")),
			Tags:       tags,
			RemoteHint: true, // filtered to j.Remote above
			Posted:     time.Unix(j.CreatedAt, 0),
			// description matched separately via matchSkill's deep check
		}.withDescription(stripHTML(j.Description)))
	}
	return out, nil
}

// withDescription stashes description text into a hidden tag prefix so the
// skill matcher can inspect it without polluting the displayed tag list.
func (j Job) withDescription(desc string) Job {
	if strings.TrimSpace(desc) != "" {
		j.Tags = append(j.Tags, "\x00desc:"+strings.ToLower(desc))
	}
	return j
}

func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// normalizeJobType maps the many source-specific spellings to a clean label.
func normalizeJobType(s string) string {
	l := strings.ToLower(s)
	switch {
	case l == "":
		return ""
	case strings.Contains(l, "free"):
		return "Freelance"
	case strings.Contains(l, "contract") || strings.Contains(l, "contractor"):
		return "Contract"
	case strings.Contains(l, "part"):
		return "Part-time"
	case strings.Contains(l, "intern") || strings.Contains(l, "temp"):
		return "Temporary"
	case strings.Contains(l, "full"):
		return "Full-time"
	}
	return ""
}

func formatSalaryRange(min, max int) string {
	k := func(n int) string {
		if n >= 1000 {
			return fmt.Sprintf("$%dk", n/1000)
		}
		return fmt.Sprintf("$%d", n)
	}
	switch {
	case min > 0 && max > 0:
		return k(min) + "–" + k(max)
	case max > 0:
		return "up to " + k(max)
	case min > 0:
		return "from " + k(min)
	}
	return ""
}
