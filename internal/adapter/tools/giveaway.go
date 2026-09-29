package tools

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/davidmm07/pressroom/internal/domain"
)

// Entry is one person's entry in a giveaway.
type Entry struct {
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	EnteredAt time.Time `json:"enteredAt"`
}

// Giveaways reads giveaway entries.
type Giveaways interface {
	Entries(ctx context.Context, giveawayID string) ([]Entry, error) // oldest first
}

// disposableDomains hand out throwaway inboxes, a favourite for entering
// the same giveaway many times.
var disposableDomains = map[string]bool{
	"mailinator.com": true, "guerrillamail.com": true, "10minutemail.com": true, "yopmail.com": true, "tempmail.dev": true,
}

// NormalizeEmail folds the tricks that make one inbox look like many:
// letter case, +tags and, for Gmail, dots in the local part.
func NormalizeEmail(email string) string {
	local, host, ok := strings.Cut(strings.ToLower(strings.TrimSpace(email)), "@")
	if !ok {
		return email
	}
	local, _, _ = strings.Cut(local, "+")
	if host == "googlemail.com" {
		host = "gmail.com"
	}
	if host == "gmail.com" {
		local = strings.ReplaceAll(local, ".", "")
	}
	return local + "@" + host
}

var (
	nonAlnum      = regexp.MustCompile(`[^a-z0-9]+`)
	abbreviations = map[string]string{"street": "st", "avenue": "ave", "road": "rd", "apartment": "apt", "suite": "ste"}
)

// NormalizeAddress makes "12 Oak Street, Apt. 4" and "12 oak st apt 4" equal.
func NormalizeAddress(address string) string {
	words := strings.Fields(nonAlnum.ReplaceAllString(strings.ToLower(address), " "))
	for i, w := range words {
		if short, ok := abbreviations[w]; ok {
			words[i] = short
		}
	}
	return strings.Join(words, " ")
}

// GiveawayScreening counts who may win and why the others may not.
type GiveawayScreening struct {
	GiveawayID          string         `json:"giveawayId"`
	TotalEntries        int            `json:"totalEntries"`
	EligibleEntries     int            `json:"eligibleEntries"`
	DisqualifiedEntries int            `json:"disqualifiedEntries"`
	Reasons             map[string]int `json:"reasons"`
	Examples            []string       `json:"examples"` // masked, so they are safe to post in chat
}

// ScreenEntries keeps each person's first entry and drops repeats and
// throwaway inboxes. Exported for tests and reuse.
func ScreenEntries(giveawayID string, entries []Entry) GiveawayScreening {
	s := GiveawayScreening{GiveawayID: giveawayID, TotalEntries: len(entries), Reasons: map[string]int{}, Examples: []string{}}
	emails, addresses := map[string]bool{}, map[string]bool{}
	for _, e := range entries {
		email, address := NormalizeEmail(e.Email), NormalizeAddress(e.Address)
		_, host, _ := strings.Cut(email, "@")
		reason := ""
		switch {
		case disposableDomains[host]:
			reason = "DISPOSABLE_EMAIL"
		case emails[email]:
			reason = "DUPLICATE_EMAIL"
		case address != "" && addresses[address]:
			reason = "DUPLICATE_ADDRESS"
		}
		if reason == "" {
			s.EligibleEntries++
			emails[email], addresses[address] = true, true
			continue
		}
		s.DisqualifiedEntries++
		s.Reasons[reason]++
		if len(s.Examples) < 5 {
			s.Examples = append(s.Examples, maskEmail(e.Email)+": "+reason)
		}
	}
	return s
}

func maskEmail(email string) string {
	local, host, ok := strings.Cut(email, "@")
	if !ok || local == "" {
		return "***"
	}
	return local[:1] + "***@" + host
}

// ScreenGiveawayEntries checks a closed giveaway before winners are drawn.
func ScreenGiveawayEntries(g Giveaways) Tool {
	return Func(domain.ToolSpec{
		Name: "screen_giveaway_entries",
		Description: "Screen a closed giveaway's entries before winners are drawn: one entry per person and per address, " +
			"no throwaway inboxes. Returns eligible and disqualified counts with masked examples.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {"giveawayId": {"type": "string", "pattern": "^G-[0-9]{1,8}$"}},
		  "required": ["giveawayId"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in struct {
		GiveawayID string `json:"giveawayId"`
	}) (any, error) {
		entries, err := g.Entries(ctx, in.GiveawayID)
		if err != nil {
			return nil, err
		}
		return ScreenEntries(in.GiveawayID, entries), nil
	})
}

// ---- demo tables

func (c *PGCommerce) Entries(ctx context.Context, giveawayID string) ([]Entry, error) {
	rows, err := c.pool.Query(ctx, `SELECT email, name, address, entered_at FROM giveaway_entries
		WHERE giveaway_id = $1 ORDER BY entered_at, email`, giveawayID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Entry])
}
