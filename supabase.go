package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// Multi-user mode is enabled when Supabase env vars are present. The engine
// then serves every subscriber in the `users` table instead of the single
// local config.json. Uses the PostgREST REST API with the service_role key
// (bypasses RLS) — no database driver dependency, pure stdlib.
//
//	SUPABASE_URL          e.g. https://<ref>.supabase.co
//	SUPABASE_SERVICE_KEY  service_role key (backend only — never client-side)

func supabaseEnabled() bool {
	return os.Getenv("SUPABASE_URL") != "" && os.Getenv("SUPABASE_SERVICE_KEY") != ""
}

// User is one subscriber row.
type User struct {
	ID                 string   `json:"id"`
	Channel            string   `json:"channel"`
	TelegramChatID     string   `json:"telegram_chat_id"`
	Email              string   `json:"email"`
	Profile            string   `json:"profile"`
	Skills             []string `json:"skills"`
	Regions            []string `json:"regions"`
	MaxJobs            int      `json:"max_jobs"`
	AllowCountryLocked bool     `json:"allow_country_locked"`
	Active             bool     `json:"active"`
}

func sbRequest(method, path string, body []byte) (*http.Request, error) {
	base := os.Getenv("SUPABASE_URL")
	key := os.Getenv("SUPABASE_SERVICE_KEY")
	req, err := http.NewRequest(method, base+"/rest/v1/"+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", key)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// fetchUsers returns all active subscribers.
func fetchUsers() ([]User, error) {
	req, err := sbRequest(http.MethodGet, "users?select=*&active=eq.true", nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch users: status %d", resp.StatusCode)
	}
	var users []User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("decode users: %w", err)
	}
	return users, nil
}

// sentSet returns the job keys already delivered to a user.
func sentSet(userID string) (map[string]bool, error) {
	req, err := sbRequest(http.MethodGet, "sent_jobs?select=job_key&user_id=eq."+userID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch sent_jobs: status %d", resp.StatusCode)
	}
	var rows []struct {
		JobKey string `json:"job_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(rows))
	for _, r := range rows {
		set[r.JobKey] = true
	}
	return set, nil
}

// markSent records the jobs just delivered to a user (idempotent).
func markSent(userID string, jobs []Job) error {
	if len(jobs) == 0 {
		return nil
	}
	type row struct {
		UserID string `json:"user_id"`
		JobKey string `json:"job_key"`
	}
	rows := make([]row, 0, len(jobs))
	for _, j := range jobs {
		rows = append(rows, row{UserID: userID, JobKey: j.Key()})
	}
	body, _ := json.Marshal(rows)
	req, err := sbRequest(http.MethodPost, "sent_jobs", body)
	if err != nil {
		return err
	}
	req.Header.Set("Prefer", "resolution=ignore-duplicates,return=minimal")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("mark sent: status %d", resp.StatusCode)
	}
	return nil
}
