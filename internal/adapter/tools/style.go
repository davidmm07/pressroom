package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/davidmm07/pressroom/internal/domain"
)

// avoidPhrases come from the support style guide: they tell the customer
// what we will not do, where a good reply says what we will do.
var avoidPhrases = func() []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, p := range []string{"sorry", "unfortunately", "can't", "cannot", "won't", "not able to", "unable to"} {
		out = append(out, regexp.MustCompile(`\b`+regexp.QuoteMeta(p)+`\b`))
	}
	return out
}()

// maxReplyWords keeps replies short enough to read on a phone.
const maxReplyWords = 120

type StyleInput struct {
	Text         string `json:"text"`
	CustomerName string `json:"customerName"`
}

// StyleReport lists what to fix before a reply is drafted.
type StyleReport struct {
	OK        bool     `json:"styleOk"`
	WordCount int      `json:"wordCount"`
	Issues    []string `json:"styleIssues"`
}

// CheckStyle applies the style guide's rules. Exported for tests and reuse.
func CheckStyle(in StyleInput) StyleReport {
	lower := strings.ToLower(strings.ReplaceAll(in.Text, "’", "'"))
	r := StyleReport{WordCount: len(strings.Fields(in.Text)), Issues: []string{}}
	for _, re := range avoidPhrases {
		if phrase := re.FindString(lower); phrase != "" {
			r.Issues = append(r.Issues, fmt.Sprintf("avoid %q: say what we will do instead", phrase))
		}
	}
	if r.WordCount > maxReplyWords {
		r.Issues = append(r.Issues, fmt.Sprintf("%d words: keep it under %d", r.WordCount, maxReplyWords))
	}
	if first, _, _ := strings.Cut(strings.TrimSpace(in.CustomerName), " "); first != "" && !strings.Contains(lower, strings.ToLower(first)) {
		r.Issues = append(r.Issues, fmt.Sprintf("use the customer's name (%s)", first))
	}
	r.OK = len(r.Issues) == 0
	return r
}

// CheckReplyStyle is a guardrail agents run before drafting a reply, so the
// house tone does not depend on the prompt alone.
func CheckReplyStyle() Tool {
	return Func(domain.ToolSpec{
		Name: "check_reply_style",
		Description: "Check a customer reply against the support style guide before drafting it: no apologies or " +
			"negative phrasing, under 120 words, and it uses the customer's name. Returns the issues to fix.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {
		    "text":         {"type": "string", "minLength": 1, "maxLength": 8000},
		    "customerName": {"type": "string", "maxLength": 120}
		  },
		  "required": ["text"],
		  "additionalProperties": false
		}`),
	}, func(_ context.Context, in StyleInput) (any, error) { return CheckStyle(in), nil })
}
