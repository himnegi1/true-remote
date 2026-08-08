package main

import (
	"fmt"
	"strings"
)

// userConfig derives a per-user Config from the shared base config, overriding
// only what the subscriber personalized.
func userConfig(base Config, u User) Config {
	c := base // copy
	if len(u.Skills) > 0 {
		c.Skills = u.Skills
	}
	if strings.TrimSpace(u.Profile) != "" {
		c.Profile = u.Profile
	}
	if len(u.Regions) > 0 {
		c.ReachableRegions = u.Regions
	}
	if u.MaxJobs > 0 {
		c.MaxJobs = u.MaxJobs
	}
	c.AllowCountryLockedRemote = u.AllowCountryLocked
	return c
}

// unionSkills gathers every skill across all users (plus the base), so the
// skill-queried sources (Remotive, Jobicy) are fetched once for everyone.
func unionSkills(base []string, users []User) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		k := strings.ToLower(strings.TrimSpace(s))
		if k == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, s)
	}
	for _, s := range base {
		add(s)
	}
	for _, u := range users {
		for _, s := range u.Skills {
			add(s)
		}
	}
	return out
}

// runMultiUser serves every subscriber from the shared job pool.
func runMultiUser(base Config, raw []Job, botToken string, dry bool) error {
	users, err := fetchUsers()
	if err != nil {
		return fmt.Errorf("fetch users: %w", err)
	}
	logf("multi-user mode: %d active subscribers", len(users))

	for _, u := range users {
		cfg := userConfig(base, u)

		seen := map[string]bool{}
		if !dry {
			if s, err := sentSet(u.ID); err != nil {
				logf("  user %s: sentSet failed (%v) — skipping to avoid repeats", u.ID, err)
				continue
			} else {
				seen = s
			}
		}

		pool := selectJobs(cfg, raw, seen)
		picked := finalPicks(cfg, pool)
		logf("  user %s (%s): %d picks", shortID(u.ID), u.Channel, len(picked))
		if len(picked) == 0 {
			continue
		}

		if dry {
			fmt.Printf("----- DRY RUN: user %s (%s) -----\n", shortID(u.ID), u.Channel)
			fmt.Println(buildMessage(picked, cfg.Skills))
			fmt.Println("-----------------------------------")
			continue
		}

		if u.Channel == "telegram" && u.TelegramChatID != "" {
			if err := sendTelegram(botToken, u.TelegramChatID, picked, cfg.Skills); err != nil {
				logf("  user %s: telegram send failed: %v", shortID(u.ID), err)
				continue // don't mark sent if delivery failed
			}
		} else {
			logf("  user %s: channel %q not yet supported — skipping", shortID(u.ID), u.Channel)
			continue
		}

		if err := markSent(u.ID, picked); err != nil {
			logf("  user %s: markSent failed: %v", shortID(u.ID), err)
		}
	}
	return nil
}

func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}
