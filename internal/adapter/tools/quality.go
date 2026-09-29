package tools

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/davidmm07/pressroom/internal/domain"
)

// Review is a customer's rating of a delivered order.
type Review struct {
	ID        string    `json:"reviewId"`
	OrderID   string    `json:"orderId"`
	Product   string    `json:"product"`
	Rating    int       `json:"rating"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// Reviews reads the review feed.
type Reviews interface {
	Recent(ctx context.Context, since time.Time, maxRating int) ([]Review, error)
}

// reviewTopics sort complaints into the categories each team owns. A model
// reads the reviews too; the keywords give it, and the sandbox, a first cut.
var reviewTopics = []struct {
	category string
	pattern  *regexp.Regexp
}{
	{"ADHESION", regexp.MustCompile(`\b(peel\w*|adhes\w*|fell off|falling off|won't stick|not stick\w*)`)},
	{"COLOR", regexp.MustCompile(`\b(colou?rs?|faded|dull|washed out)\b`)},
	{"CUTTING", regexp.MustCompile(`\b(cut|cuts|cutting|miscut|crooked|off[- ]cent(er|re)d?)\b`)},
	{"DAMAGE", regexp.MustCompile(`\b(crack\w*|bent|scratch\w*|broken|damaged)\b`)},
	{"SHIPPING", regexp.MustCompile(`\b(late|slow|lost|never arrived|shipping)\b`)},
}

// ReviewTopic returns the first category a review's text matches, or OTHER.
func ReviewTopic(body string) string {
	lower := strings.ToLower(strings.ReplaceAll(body, "’", "'"))
	for _, t := range reviewTopics {
		if t.pattern.MatchString(lower) {
			return t.category
		}
	}
	return "OTHER"
}

// Cluster is a group of complaints about one product and topic.
type Cluster struct {
	Product        string   `json:"product"`
	Category       string   `json:"category"`
	ReviewCount    int      `json:"reviewCount"`
	SampleOrderIDs []string `json:"sampleOrderIds"`
}

// Digest summarises the low ratings of a period.
type Digest struct {
	ReviewsRead int       `json:"reviewsRead"`
	Clusters    []Cluster `json:"clusters"`
	TopIssue    *Cluster  `json:"topIssue,omitempty"`
	NotNeeded   []string  `json:"notNeeded"`
}

// minClusterSize is how many similar complaints make a pattern worth a
// defect report rather than bad luck.
const minClusterSize = 3

// DigestReviews groups complaints by product and topic, largest first.
// Exported for tests and reuse.
func DigestReviews(reviews []Review) Digest {
	byKey := map[[2]string]*Cluster{}
	for _, r := range reviews {
		key := [2]string{r.Product, ReviewTopic(r.Body)}
		c := byKey[key]
		if c == nil {
			c = &Cluster{Product: key[0], Category: key[1], SampleOrderIDs: []string{}}
			byKey[key] = c
		}
		c.ReviewCount++
		if len(c.SampleOrderIDs) < 5 {
			c.SampleOrderIDs = append(c.SampleOrderIDs, r.OrderID)
		}
	}
	d := Digest{ReviewsRead: len(reviews), Clusters: []Cluster{}, NotNeeded: []string{}}
	for _, c := range byKey {
		d.Clusters = append(d.Clusters, *c)
	}
	sort.Slice(d.Clusters, func(i, j int) bool {
		a, b := d.Clusters[i], d.Clusters[j]
		if a.ReviewCount != b.ReviewCount {
			return a.ReviewCount > b.ReviewCount
		}
		return a.Product+a.Category < b.Product+b.Category
	})
	for i, c := range d.Clusters {
		if c.Category != "OTHER" && c.ReviewCount >= minClusterSize {
			d.TopIssue = &d.Clusters[i]
			break
		}
	}
	if d.TopIssue == nil {
		d.NotNeeded = []string{"file_defect_report", "notify_team"}
	}
	return d
}

// FetchReviews reads recent low ratings and groups them.
func FetchReviews(r Reviews) Tool {
	return Func(domain.ToolSpec{
		Name: "fetch_reviews",
		Description: "Read recent customer reviews at or below a rating and group the complaints by product and topic " +
			"(adhesion, color, cutting, damage, shipping). topIssue is the largest group worth a defect report.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {
		    "sinceDays": {"type": "integer", "minimum": 1, "maximum": 90},
		    "maxRating": {"type": "integer", "minimum": 1, "maximum": 5, "default": 3}
		  },
		  "required": ["sinceDays"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in struct {
		SinceDays int `json:"sinceDays"`
		MaxRating int `json:"maxRating"`
	}) (any, error) {
		if in.MaxRating == 0 {
			in.MaxRating = 3
		}
		reviews, err := r.Recent(ctx, time.Now().AddDate(0, 0, -in.SinceDays), in.MaxRating)
		if err != nil {
			return nil, err
		}
		return DigestReviews(reviews), nil
	})
}

// defectOwner routes each category to the team that can fix it.
var defectOwner = map[string]domain.Department{
	"ADHESION": domain.DepartmentManufacturing, "COLOR": domain.DepartmentPrepress, "CUTTING": domain.DepartmentManufacturing,
	"DAMAGE": domain.DepartmentOperations, "SHIPPING": domain.DepartmentOperations, "OTHER": domain.DepartmentCustomerExperience,
}

type defectInput struct {
	Product        string   `json:"product"`
	Category       string   `json:"category"`
	ReviewCount    int      `json:"reviewCount"`
	SampleOrderIDs []string `json:"sampleOrderIds"`
	Summary        string   `json:"summary"`
}

// FileDefectReport opens a quality ticket with the team that owns the cause.
func FileDefectReport(c Commerce) Tool {
	return Func(domain.ToolSpec{
		Name: "file_defect_report",
		Description: "Open a defect report for a pattern of complaints. It is routed to the team that owns the cause: " +
			"manufacturing for adhesion and cutting, prepress for color, operations for damage and shipping.",
		InputSchema: Schema(fmt.Sprintf(`{
		  "type": "object",
		  "properties": {
		    "product":        {"type": "string", "enum": ["STICKERS","LABELS","MAGNETS","BUTTONS","PACKAGING","TSHIRTS"]},
		    "category":       {"type": "string", "enum": ["ADHESION","COLOR","CUTTING","DAMAGE","SHIPPING","OTHER"]},
		    "reviewCount":    {"type": "integer", "minimum": 1},
		    "sampleOrderIds": {"type": "array", "items": %s, "minItems": 1, "maxItems": 10},
		    "summary":        {"type": "string", "minLength": 10, "maxLength": 2000}
		  },
		  "required": ["product","category","reviewCount","sampleOrderIds","summary"],
		  "additionalProperties": false
		}`, orderIDSchema)),
	}, func(ctx context.Context, in defectInput) (any, error) {
		id, err := c.Record(ctx, "DEFECT_REPORT", idempotencyKey(ctx, "defect"), in)
		if err != nil {
			return nil, err
		}
		return map[string]any{"reportId": id, "status": "OPEN", "assignedTo": defectOwner[in.Category]}, nil
	})
}

// ---- demo tables

func (c *PGCommerce) Recent(ctx context.Context, since time.Time, maxRating int) ([]Review, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT id, order_id, product, rating, body, created_at FROM storefront_reviews
		WHERE created_at >= $1 AND rating <= $2 ORDER BY created_at DESC, id LIMIT 500`, since, maxRating)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Review])
}
