package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
)

// sendTelegram posts an HTML-formatted digest to the configured chat.
func sendTelegram(token, chatID string, jobs []Job) error {
	text := buildMessage(jobs)

	payload := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return fmt.Errorf("telegram status %d: %s", resp.StatusCode, buf.String())
	}
	return nil
}

func buildMessage(jobs []Job) string {
	date := time.Now().Format("Mon, 02 Jan 2006")
	var b strings.Builder

	if len(jobs) == 0 {
		fmt.Fprintf(&b, "🧭 <b>TrueRemote</b> — %s\n\n", date)
		b.WriteString("No new Go / C++ remote roles matched your filters today. ")
		b.WriteString("The scan ran fine — check again tomorrow. 👋")
		return b.String()
	}

	fmt.Fprintf(&b, "🧭 <b>TrueRemote</b> — %s\n", date)
	fmt.Fprintf(&b, "Top %d remote Go / C++ roles — reachable from India (worldwide · India · APAC · any timezone)\n\n", len(jobs))

	for i, j := range jobs {
		fmt.Fprintf(&b, "<b>%d. %s</b>\n", i+1, esc(j.Title))
		fmt.Fprintf(&b, "🏢 %s\n", esc(orDash(j.Company)))
		if j.Salary != "" {
			fmt.Fprintf(&b, "💰 %s\n", esc(j.Salary))
		} else {
			b.WriteString("💰 not listed\n")
		}
		if strings.TrimSpace(j.Location) != "" {
			fmt.Fprintf(&b, "🌍 %s\n", esc(j.Location))
		}
		if j.JobType != "" {
			fmt.Fprintf(&b, "🕒 %s\n", esc(j.JobType))
		}
		if len(j.Tags) > 0 {
			fmt.Fprintf(&b, "🧩 %s\n", esc(strings.Join(j.Tags, ", ")))
		}
		fmt.Fprintf(&b, "🔗 <a href=\"%s\">Apply</a> · via %s\n\n", esc(j.URL), esc(j.Source))
	}

	b.WriteString("<i>Sources: company career portals (Greenhouse/Ashby/Lever) + Remotive, Remote OK, Arbeitnow, Jobicy.</i>")
	return b.String()
}

func esc(s string) string { return html.EscapeString(s) }

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
