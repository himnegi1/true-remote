package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// This file pulls jobs directly from companies' own career portals via their
// ATS (applicant tracking system) public APIs. This is the same data shown on
// each company's careers page — official, free, and far cleaner than
// aggregator tag-soup. Each company slug is verified live in companies.json.

type atsTask struct {
	ats  string
	slug string
	run  func(string) ([]Job, error)
}

// fetchATS pulls every company board concurrently (bounded pool), since the
// roster is large and each board is an independent network call.
func fetchATS(c Companies) []Job {
	var tasks []atsTask
	for _, s := range c.Greenhouse {
		tasks = append(tasks, atsTask{"Greenhouse", s, fetchGreenhouse})
	}
	for _, s := range c.Ashby {
		tasks = append(tasks, atsTask{"Ashby", s, fetchAshby})
	}
	for _, s := range c.Lever {
		tasks = append(tasks, atsTask{"Lever", s, fetchLever})
	}

	const workers = 12
	jobsCh := make(chan atsTask)
	resCh := make(chan []Job)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobsCh {
				jobs, err := t.run(t.slug)
				reportATS(t.ats, t.slug, jobs, err)
				resCh <- jobs
			}
		}()
	}
	go func() {
		for _, t := range tasks {
			jobsCh <- t
		}
		close(jobsCh)
	}()
	go func() { wg.Wait(); close(resCh) }()

	var all []Job
	for jobs := range resCh {
		all = append(all, jobs...)
	}
	return all
}

func reportATS(ats, slug string, jobs []Job, err error) {
	if err != nil {
		logf("ATS %s/%s failed: %v", ats, slug, err)
		return
	}
	logf("ATS %s/%s returned %d raw jobs", ats, slug, len(jobs))
}

// ---------- Greenhouse ----------
// List endpoint has no description, so we match skill on the title and read
// remote/region from the location string (e.g. "Remote, United States").

type ghResp struct {
	Jobs []struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		AbsoluteURL string `json:"absolute_url"`
		CompanyName string `json:"company_name"`
		UpdatedAt   string `json:"updated_at"`
		Location    struct {
			Name string `json:"name"`
		} `json:"location"`
	} `json:"jobs"`
}

func fetchGreenhouse(slug string) ([]Job, error) {
	b, err := httpGet("https://boards-api.greenhouse.io/v1/boards/" + slug + "/jobs")
	if err != nil {
		return nil, err
	}
	var r ghResp
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	var out []Job
	for _, j := range r.Jobs {
		loc := j.Location.Name
		out = append(out, Job{
			Source:     "Greenhouse",
			ID:         slug + "-" + fmt.Sprintf("%d", j.ID),
			Title:      j.Title,
			Company:    orName(j.CompanyName, slug),
			Location:   loc,
			URL:        j.AbsoluteURL,
			RemoteHint: hasRemote(loc),
			Posted:     parseTime(j.UpdatedAt),
		})
	}
	return out, nil
}

// ---------- Ashby ----------
// Rich list: includes descriptionPlain, isRemote, and applyUrl directly.

type ashResp struct {
	Jobs []struct {
		ID                 string `json:"id"`
		Title              string `json:"title"`
		Department         string `json:"department"`
		EmploymentType     string `json:"employmentType"`
		Location           string `json:"location"`
		SecondaryLocations []struct {
			Location string `json:"location"`
		} `json:"secondaryLocations"`
		IsRemote         bool   `json:"isRemote"`
		WorkplaceType    string `json:"workplaceType"`
		ApplyURL         string `json:"applyUrl"`
		JobURL           string `json:"jobUrl"`
		PublishedAt      string `json:"publishedAt"`
		DescriptionPlain string `json:"descriptionPlain"`
		Address          struct {
			PostalAddress struct {
				AddressCountry string `json:"addressCountry"`
			} `json:"postalAddress"`
		} `json:"address"`
	} `json:"jobs"`
}

func fetchAshby(slug string) ([]Job, error) {
	b, err := httpGet("https://api.ashbyhq.com/posting-api/job-board/" + slug)
	if err != nil {
		return nil, err
	}
	var r ashResp
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	var out []Job
	for _, j := range r.Jobs {
		locParts := []string{j.Location, j.Address.PostalAddress.AddressCountry}
		for _, s := range j.SecondaryLocations {
			locParts = append(locParts, s.Location)
		}
		loc := joinNonEmpty(locParts, " / ")
		url := j.ApplyURL
		if url == "" {
			url = j.JobURL
		}
		job := Job{
			Source:     "Ashby",
			ID:         slug + "-" + j.ID,
			Title:      j.Title,
			Company:    prettyName(slug),
			Location:   loc,
			URL:        url,
			JobType:    normalizeJobType(j.EmploymentType),
			Tags:       nonEmpty([]string{j.Department}),
			RemoteHint: j.IsRemote || hasRemote(j.WorkplaceType) || hasRemote(loc),
			Posted:     parseTime(j.PublishedAt),
		}
		out = append(out, job.withDescription(j.DescriptionPlain))
	}
	return out, nil
}

// ---------- Lever ----------

type leverItem struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	HostedURL  string `json:"hostedUrl"`
	CreatedAt  int64  `json:"createdAt"`
	WorkType   string `json:"workplaceType"`
	Categories struct {
		Team       string `json:"team"`
		Location   string `json:"location"`
		Commitment string `json:"commitment"`
	} `json:"categories"`
	DescriptionPlain string `json:"descriptionPlain"`
}

func fetchLever(slug string) ([]Job, error) {
	b, err := httpGet("https://api.lever.co/v0/postings/" + slug + "?mode=json")
	if err != nil {
		return nil, err
	}
	var items []leverItem
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	var out []Job
	for _, it := range items {
		loc := it.Categories.Location
		job := Job{
			Source:     "Lever",
			ID:         slug + "-" + it.ID,
			Title:      it.Text,
			Company:    prettyName(slug),
			Location:   loc,
			URL:        it.HostedURL,
			JobType:    normalizeJobType(it.Categories.Commitment),
			Tags:       nonEmpty([]string{it.Categories.Team}),
			RemoteHint: hasRemote(it.WorkType) || hasRemote(loc),
			Posted:     time.Unix(it.CreatedAt/1000, 0),
		}
		out = append(out, job.withDescription(it.DescriptionPlain))
	}
	return out, nil
}

// ---------- helpers ----------

func hasRemote(s string) bool { return strings.Contains(strings.ToLower(s), "remote") }

func orName(name, fallback string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return prettyName(fallback)
}

var nameOverrides = map[string]string{
	"openai": "OpenAI", "gitlab": "GitLab", "mongodb": "MongoDB",
	"grafanalabs": "Grafana Labs", "digitalocean": "DigitalOcean",
	"github": "GitHub", "hashicorp": "HashiCorp", "vanta": "Vanta",
}

func prettyName(slug string) string {
	if slug == "" {
		return "—"
	}
	if n, ok := nameOverrides[strings.ToLower(slug)]; ok {
		return n
	}
	return strings.ToUpper(slug[:1]) + slug[1:]
}

func joinNonEmpty(parts []string, sep string) string {
	return strings.Join(nonEmpty(parts), sep)
}

func nonEmpty(parts []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[strings.ToLower(p)] {
			continue
		}
		seen[strings.ToLower(p)] = true
		out = append(out, p)
	}
	return out
}
