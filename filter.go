package main

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Seniority signal — senior+ roles rank higher (configurable via profile).
var reSenior = regexp.MustCompile(`(?i)\b(senior|sr\.?|staff|principal|lead|architect|distinguished)\b`)

var reAlnum = regexp.MustCompile(`^[a-z0-9]+$`)

// skillMatchers are compiled from the user's configured skills, so the tool is
// not tied to Go/C++ — it works for any skill set.
type skillMatchers struct {
	loose  *regexp.Regexp // all skills; used on titles/curated tags (high signal)
	strict *regexp.Regexp // skills ≥3 chars; used on free-text descriptions to
	// avoid short ambiguous tokens (e.g. "go" matching the English word)
}

// buildSkillMatchers turns []{"golang","c++",…} into word-boundary regexes.
func buildSkillMatchers(skills []string) skillMatchers {
	var loosePats, strictPats []string
	for _, s := range skills {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		var pat string
		if reAlnum.MatchString(s) {
			pat = `\b` + regexp.QuoteMeta(s) + `\b` // whole-word for plain terms
		} else {
			pat = regexp.QuoteMeta(s) // symbols (c++, .net, c#) — plain escaped
		}
		loosePats = append(loosePats, pat)
		if len(s) >= 3 {
			strictPats = append(strictPats, pat)
		}
	}
	if len(strictPats) == 0 {
		strictPats = loosePats
	}
	if len(loosePats) == 0 { // no skills configured → match nothing
		loosePats = []string{`$^`}
		strictPats = []string{`$^`}
	}
	return skillMatchers{
		loose:  regexp.MustCompile(`(?i)` + strings.Join(loosePats, "|")),
		strict: regexp.MustCompile(`(?i)` + strings.Join(strictPats, "|")),
	}
}

// trustedTagSources have curated, low-noise tags we can match against.
// RemoteOK/ATS tags are noisy or generic, so we ignore them for matching.
var trustedTagSources = map[string]bool{"Remotive": true, "Arbeitnow": true}

// coreDevQualifiers mark a title as a real software-development role (where Go/
// C++ would be a primary skill), as opposed to an adjacent ops/perf/data role
// that merely mentions the language in passing.
var coreDevQualifiers = []string{
	"software", "backend", "back-end", "back end", "full stack", "full-stack",
	"fullstack", "distributed", "platform", "application", "api", "microservice",
	"micro-service", "server", "golang", "go engineer", "go developer", "web",
}

func isCoreDevTitle(title string) bool {
	t := strings.ToLower(title)
	if !containsAnyWord(t, "engineer", "developer", "programmer") {
		return false
	}
	return containsAnySub(t, coreDevQualifiers...)
}

// skillRelevance scores how central a configured skill is to a role:
//
//	4 = the skill is in the TITLE (or a curated tag) — strongly relevant
//	2 = the skill is only in the description, but the title is a core dev role
//	0 = no real match
func skillRelevance(j Job, m skillMatchers) int {
	titleBlob := strings.ToLower(j.Title)
	var descBlob string
	for _, t := range j.Tags {
		if strings.HasPrefix(t, "\x00desc:") {
			descBlob += " " + strings.TrimPrefix(t, "\x00desc:")
			continue
		}
		if trustedTagSources[j.Source] {
			titleBlob += " " + strings.ToLower(t)
		}
	}
	if m.loose.MatchString(titleBlob) {
		return 4
	}
	if m.strict.MatchString(descBlob) && isCoreDevTitle(j.Title) {
		return 2
	}
	return 0
}

func containsAnyWord(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func containsAnySub(s string, subs ...string) bool {
	for _, w := range subs {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// reachability decides whether a role is open to someone working from
// anywhere (the user is in India, any timezone OK). We keep only roles that
// are genuinely global/regionally-open to them and DROP roles geo-locked to a
// country they can't work from (US-only, an EU country only, India-only, …).
func reachability(cfg Config, loc string) (bump int, keep bool) {
	l := strings.ToLower(strings.TrimSpace(loc))
	if l == "" {
		return 0, true // unknown — keep at low priority
	}
	for _, r := range cfg.ReachableRegions {
		if strings.Contains(l, r) {
			return 2, true
		}
	}
	// Bare "Remote"/"Fully remote" with NO specific country named → nothing says
	// you can't apply, so keep it (better to see it than miss it). Only an
	// explicitly named non-reachable country (e.g. "Remote, USA") is dropped.
	if isBareRemote(l) {
		return 1, true
	}
	// A specific country/region that isn't reachable (e.g. "United States").
	// Strict mode drops it; the toggle keeps it at low priority for volume.
	if cfg.AllowCountryLockedRemote {
		return 0, true
	}
	return 0, false
}

// isBareRemote reports whether a location is just "remote"/empty with no
// specific country named — i.e. plausibly open to anyone.
func isBareRemote(loc string) bool {
	l := strings.ToLower(loc)
	for _, w := range []string{"remote", "fully", "home based", "home-based", "distributed", "anywhere", "global"} {
		l = strings.ReplaceAll(l, w, "")
	}
	for _, r := range l {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func excludedTitle(cfg Config, title string) bool {
	t := strings.ToLower(title)
	for _, bad := range cfg.ExcludeTitle {
		if strings.Contains(t, bad) {
			return true
		}
	}
	return false
}

// excludedCompany drops talent-marketplace / staffing / gig platforms — the
// "cheap labour" sources the user wants to steer clear of.
func excludedCompany(cfg Config, company string) bool {
	c := strings.ToLower(company)
	for _, bad := range cfg.ExcludeCompany {
		if strings.Contains(c, bad) {
			return true
		}
	}
	return false
}

// selectJobs applies all filters, scores, dedups against seen, sorts, and
// returns at most cfg.MaxJobs fresh jobs (best first).
func selectJobs(cfg Config, raw []Job, seen map[string]bool) []Job {
	dedupe := map[string]bool{}
	matchers := buildSkillMatchers(cfg.Skills)
	var candidates []Job
	var nRemote, nSkill, nExcl, nRegionDropped int

	for _, j := range raw {
		if seen[j.Key()] || dedupe[j.Key()] {
			continue
		}
		if !j.RemoteHint { // fully-remote only
			continue
		}
		nRemote++
		rel := skillRelevance(j, matchers)
		if rel == 0 {
			continue
		}
		nSkill++
		if excludedTitle(cfg, j.Title) || excludedCompany(cfg, j.Company) {
			continue
		}
		nExcl++
		bump, keep := reachability(cfg, j.Location)
		if !keep {
			nRegionDropped++ // relevant, but locked to a country you can't work from
			continue
		}
		j.score = bump + rel // relevance dominates ranking
		if j.Salary != "" {
			j.score += 3 // reward transparent (usually USD/EUR) pay
		}
		if !j.Posted.IsZero() && time.Since(j.Posted) < 7*24*time.Hour {
			j.score++
		}
		if reSenior.MatchString(j.Title) {
			j.score += 2 // match the user's seniority (12+ yrs)
		}
		dedupe[j.Key()] = true
		// strip hidden description tags before display
		j.Tags = visibleTags(j.Tags)
		candidates = append(candidates, j)
	}

	logf("funnel: remote=%d skill-relevant=%d after-excludes=%d region-dropped=%d final=%d",
		nRemote, nSkill, nExcl, nRegionDropped, len(candidates))

	sort.SliceStable(candidates, func(a, b int) bool {
		if candidates[a].score != candidates[b].score {
			return candidates[a].score > candidates[b].score
		}
		return candidates[a].Posted.After(candidates[b].Posted)
	})

	// Diversity: cap how many roles any single company can take, so one big
	// employer (e.g. OpenAI with 700+ openings) can't monopolize the digest.
	perCompany := cfg.MaxPerCompany
	if perCompany <= 0 {
		perCompany = 2
	}
	count := map[string]int{}
	titleSeen := map[string]bool{}
	var picked []Job
	for _, j := range candidates {
		key := strings.ToLower(j.Company)
		if count[key] >= perCompany {
			continue
		}
		// Skip the same role reposted per-geo (e.g. US + Canada listing).
		titleKey := key + "|" + strings.ToLower(strings.TrimSpace(j.Title))
		if titleSeen[titleKey] {
			continue
		}
		titleSeen[titleKey] = true
		count[key]++
		picked = append(picked, j)
		if len(picked) == candidatePoolSize {
			break
		}
	}
	return picked
}

// candidatePoolSize is how many heuristically-ranked candidates we keep before
// the final selection. When LLM ranking is on it re-ranks this pool; otherwise
// the top cfg.MaxJobs are taken directly.
const candidatePoolSize = 30

func visibleTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		if strings.HasPrefix(t, "\x00desc:") || strings.TrimSpace(t) == "" {
			continue
		}
		out = append(out, t)
		if len(out) == 6 { // keep the Telegram card compact
			break
		}
	}
	return out
}
