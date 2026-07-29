# TrueRemote 🧭

A tiny, dependency-free **Go** service that finds up to **5 fresh, *genuinely* remote Go / C++ jobs every day** — roles open to anywhere, not fake "US-only remote" — and pushes them to your **Telegram** (company · pay · stack · job type · apply link). Runs on **GitHub Actions for free**, no server to maintain.

Built to surface roles that pay in **global USD/EUR (not location-normalized)** and to ignore staffing/gig marketplaces. It drops roles geo-locked to a country you can't work from and keeps only what's actually reachable.

---

## How it works

```
 ┌─ Remote-job aggregators ─┐     ┌─ Company career portals (ATS) ──────────┐
 │  Remotive                │     │  Greenhouse: datadog, cloudflare,        │
 │  Remote OK               │     │    mongodb, coinbase, gitlab, stripe…    │
 │  Arbeitnow (EU)          │     │  Ashby: openai, ramp, notion, linear…    │
 │  Jobicy (worldwide)      │     │  Lever: voltus…                          │
              │                    └───────────────┬──────────────────────────┘
              └──────────────┬──────────────────────┘
                             ▼
        Filter:  Go/C++ (title or real description, not tag-soup)
                 · fully remote only
                 · US / EU / Singapore / Dubai / worldwide
                 · engineering titles only
                 · drop staffing & gig marketplaces (A.Team, Toptal…)
                 · dedupe against everything already sent (data/seen.json)
                             ▼
        Score (salary shown, target region, recency) → top 5, max 2 per company
                             ▼
                   Telegram message 📲
```

Why career-portal APIs? They're the **same data a company shows on its own careers page** — official, free, and far cleaner than an aggregator's throttled free tier. That's what makes the results genuinely targeted.

---

## One-time setup (~10 minutes)

### 1. Create your Telegram bot
1. In Telegram, open **@BotFather** → send `/newbot` → follow prompts.
2. Copy the **bot token** it gives you (looks like `123456:ABC-DEF...`).
3. Send any message to your new bot (e.g. "hi") so it can message you back.
4. Get your **chat id**: open this URL in a browser (paste your token):
   `https://api.telegram.org/bot<YOUR_TOKEN>/getUpdates`
   Look for `"chat":{"id":123456789 ...}` — that number is your chat id.

> 🔒 Never paste the bot token into code or commit it. It goes only into GitHub Secrets (below).

### 2. Push this project to a GitHub repo
```bash
git init
git add .
git commit -m "Job Scout"
git branch -M main
git remote add origin https://github.com/<you>/job-scout.git
git push -u origin main
```

### 3. Add your secrets on GitHub
Repo → **Settings → Secrets and variables → Actions → New repository secret**. Add two:
- `TELEGRAM_BOT_TOKEN` = your bot token
- `TELEGRAM_CHAT_ID` = your chat id

### 4. Turn it on
- Repo → **Actions** tab → enable workflows if prompted.
- Open **Daily Job Scout** → **Run workflow** to test it now. You should get a Telegram message within a minute.
- After that it runs **automatically every day at 09:00 IST** (03:30 UTC). Change the time in `.github/workflows/daily-jobs.yml` (`cron:` line).

That's it. Free forever on GitHub's cron.

---

## Test locally (optional)

Preview the digest without sending anything:
```bash
go run . --dry-run
```
Send a real Telegram message from your machine:
```bash
TELEGRAM_BOT_TOKEN=xxx TELEGRAM_CHAT_ID=yyy go run .
```

---

## Tuning it

Everything is plain JSON — no rebuild needed, just edit and push.

**`config.json`**
- `maxJobs` — how many roles per day (default 5).
- `maxPerCompany` — diversity cap (default 2).
- `allowCountryLockedRemote` — **the volume knob.** `false` (default) = only roles reachable from India (worldwide/India/APAC/Singapore/Dubai). `true` = also include country-locked remote (US-remote, EU-remote) for more volume.
- `skills` — currently `golang`, `c++`.
- `reachableRegions` — location strings that count as reachable from India (worldwide/India/APAC/Singapore/Dubai…). **A role is dropped only if it names a specific non-reachable country** (e.g. "Remote, USA"). A plain "Remote" with no country named is kept — nothing says you can't apply.
- `excludeTitle` — kills non-core-dev titles (sales, GTM, recruiter, firmware/embedded/kernel/HFT, gaming…).
- `excludeCompany` — staffing/gig platforms + HFT firms to ignore.

**Relevance:** Go/C++ in the **title** (or a curated tag) = strongly relevant and ranks top. Go/C++ only in the **description** is kept only if the title is a real software-dev role (Software/Backend/Platform/Distributed Engineer), so ops/perf/DBA roles that just mention the language are dropped.

**`companies.json`** — the career portals pulled directly. To add a company, find its board slug in its careers URL and drop the slug in the right list:
- Greenhouse → `boards.greenhouse.io/<slug>`
- Ashby → `jobs.ashbyhq.com/<slug>`
- Lever → `jobs.lever.co/<slug>`

All slugs shipped here are verified live. Add your dream companies freely — a dead slug is skipped safely.

---

## Notes & honest limits
- **Salary** isn't published on many roles; it's shown when the source provides it and marked *not listed* otherwise. Aggregators (Remote OK) surface USD ranges most often.
- **Company-size filtering** is heuristic (curated portal list + staffing denylist), not perfect.
- **Greenhouse** matches on the job *title* (its list API has no description); Ashby/Lever/aggregators also match on the real description.
- **Attribution:** per Remotive's API terms, Remotive is credited as a source in every message and its data is queried a few times/day at most.
- No LinkedIn scraping — it's against their ToS, brittle, and can get you flagged. The career-portal approach is the sustainable substitute.
- **No country-specific job boards** (Seek/StepStone/Bayt/MyCareersFuture/JustJoin.it). They were evaluated and rejected: most have no free API, and they list *locally-work-authorized* roles (Australia/Germany/Singapore work rights) that a fully-remote-from-India candidate can't take — they'd be filtered out anyway. Worldwide-remote boards (Jobicy) and global company portals are used instead.
