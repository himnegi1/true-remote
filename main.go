package main

import (
	"fmt"
	"log"
	"os"
)

func logf(format string, args ...any) { log.Printf(format, args...) }

func main() {
	log.SetFlags(log.Ltime)

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

	// --dry-run prints the digest to stdout instead of hitting Telegram.
	dry := len(os.Args) > 1 && os.Args[1] == "--dry-run"
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

	picked := selectJobs(cfg, raw, seen)
	logf("selected %d fresh matching jobs", len(picked))

	if dry {
		fmt.Println("----- DRY RUN: Telegram message preview -----")
		fmt.Println(buildMessage(picked))
		fmt.Println("---------------------------------------------")
		return
	}

	if err := sendTelegram(token, chatID, picked); err != nil {
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
