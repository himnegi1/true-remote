package main

import (
	"fmt"
	"log"
	"os"
	"strings"
)

func logf(format string, args ...any) { log.Printf(format, args...) }

func main() {
	log.SetFlags(log.Ltime)

	// --onboard "plain english" → LLM-generated skills + profile for config.json.
	if len(os.Args) > 1 && os.Args[1] == "--onboard" {
		if err := runOnboard(strings.Join(os.Args[2:], " ")); err != nil {
			log.Fatalf("onboard: %v", err)
		}
		return
	}

	cfg, err := loadConfig("config.json")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	companies, err := loadCompanies("companies.json")
	if err != nil {
		log.Fatalf("companies: %v", err)
	}

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	dry := len(os.Args) > 1 && os.Args[1] == "--dry-run"

	// ---- Multi-user mode (Supabase configured): serve every subscriber. ----
	if supabaseEnabled() {
		if !dry && token == "" {
			log.Fatal("TELEGRAM_BOT_TOKEN must be set for multi-user delivery (or pass --dry-run)")
		}
		users, err := fetchUsers()
		if err != nil {
			log.Fatalf("fetch users: %v", err)
		}
		// Fetch the skill-queried sources once for the union of all users' skills.
		fetchCfg := cfg
		fetchCfg.Skills = unionSkills(cfg.Skills, users)
		raw := fetchAll(fetchCfg, companies)
		logf("total raw jobs across sources: %d", len(raw))

		if err := runMultiUser(cfg, raw, token, dry); err != nil {
			log.Fatalf("multi-user run: %v", err)
		}
		return
	}

	// ---- Single-user mode (local config.json + seen.json). ----
	if !dry && (token == "" || chatID == "") {
		log.Fatal("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID must be set (or pass --dry-run)")
	}

	seen, existingKeys, err := loadSeen()
	if err != nil {
		log.Fatalf("load seen: %v", err)
	}
	logf("loaded %d previously-seen jobs", len(seen))

	raw := fetchAll(cfg, companies)
	logf("total raw jobs across sources: %d", len(raw))

	pool := selectJobs(cfg, raw, seen)
	logf("candidate pool: %d jobs", len(pool))

	// Final selection: LLM re-ranks against the profile if a key is set,
	// otherwise the heuristic top picks are used.
	picked := finalPicks(cfg, pool)
	logf("final selection: %d jobs (llm=%v)", len(picked), llmEnabled())

	if dry {
		fmt.Println("----- DRY RUN: Telegram message preview -----")
		fmt.Println(buildMessage(picked, cfg.Skills))
		fmt.Println("---------------------------------------------")
		return
	}

	if err := sendTelegram(token, chatID, picked, cfg.Skills); err != nil {
		log.Fatalf("telegram send: %v", err)
	}
	logf("telegram message sent")

	// Only remember jobs we actually delivered.
	if len(picked) > 0 {
		if err := saveSeen(existingKeys, picked); err != nil {
			log.Fatalf("save seen: %v", err)
		}
		logf("persisted %d newly-seen jobs", len(picked))
	}
}
