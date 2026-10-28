package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Sentry tools: read the mobile app's error reports through this server, so
// admins and their AI tools see them without a Sentry login of their own (the
// org is on Sentry's one-seat free plan).
//
// The token is a read-only Sentry internal integration (Project, Issue & Event,
// Organization: Read), held in SENTRY_READ_TOKEN. Nothing here writes to
// Sentry. What Sentry holds was already scrubbed by the app before it was sent
// (mobile/src/services/observability.ts), so these tools return it as stored.

const (
	sentryAPIBase        = "https://sentry.io/api/0"
	sentryDefaultOrg     = "sfluv"
	sentryDefaultProject = "sfluv-mobile"
	sentryTimeout        = 15 * time.Second
	sentryMaxBreadcrumbs = 40
)

var (
	sentrySlug    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,49}$`)
	sentryIssueID = regexp.MustCompile(`^[0-9]{1,20}$`)
	sentryShortID = regexp.MustCompile(`^[A-Za-z0-9-]{3,64}$`)
	sentryVersion = regexp.MustCompile(`^[0-9A-Za-z.+_-]{1,40}$`)
)

type sentryClient struct {
	token  string
	org    string
	client *http.Client
}

func newSentryClient() *sentryClient {
	org := strings.TrimSpace(os.Getenv("SENTRY_ORG"))
	if org == "" {
		org = sentryDefaultOrg
	}
	return &sentryClient{
		token:  strings.TrimSpace(os.Getenv("SENTRY_READ_TOKEN")),
		org:    org,
		client: &http.Client{Timeout: sentryTimeout},
	}
}

func (c *sentryClient) get(ctx context.Context, path string, query url.Values, out any) error {
	if c.token == "" {
		return errors.New("Sentry is not configured on this server (SENTRY_READ_TOKEN is unset)")
	}
	endpoint := sentryAPIBase + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach Sentry: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("reading Sentry's answer: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return errors.New("Sentry has no such issue or project")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Sentry answered %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	return json.Unmarshal(body, out)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (a *reportCore) registerSentryTools(s *server.MCPServer) {
	sentry := newSentryClient()

	a.addTool(s, mcp.NewTool("sentry_issues",
		mcp.WithDescription("List error reports (issues) from the SFLUV mobile app in Sentry: crashes, backend requests the app treated as failures, screens stuck loading (\"Still waiting: …\"), and handled errors. Start here when debugging a reported bug, then open one with sentry_issue. Each issue groups many occurrences; count and user_count say how widespread it is."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("environment", mcp.Description("preview (test builds), production (store builds) or development. Omit for all.")),
		mcp.WithString("version", mcp.Description("App version, e.g. 2.2.0. Omit for all versions.")),
		mcp.WithString("status", mcp.Description("unresolved (default), resolved, ignored, or all.")),
		mcp.WithString("search", mcp.Description("Optional extra Sentry search, e.g. 'request.route:/conversations' or 'stuck:*' or free text.")),
		mcp.WithString("sort", mcp.Description("date (last seen, default), new (first seen), or freq (most occurrences).")),
		mcp.WithString("period", mcp.Description("How far back: 24h, 7d, 14d (default) or 30d.")),
		mcp.WithNumber("count", mcp.Description("Issues to return, default 25, max 100.")),
		mcp.WithString("project", mcp.Description("Sentry project slug, default sfluv-mobile.")),
	), func(ctx context.Context, request mcp.CallToolRequest) (any, error) {
		return a.sentryIssues(ctx, sentry, request)
	})

	a.addTool(s, mcp.NewTool("sentry_issue",
		mcp.WithDescription("One Sentry issue in full: its latest occurrence with the stack trace (app frames with file, function and line), the trail of steps before it (tab changes, notification taps, each backend request with route, status, duration and request_id, watched waits), tags (release, build, environment, device, OS, OTA update) and contexts. A request_id can be searched for in the backend's Railway logs to see the server side of a failed request."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue id (numeric) or short id (e.g. SFLUV-MOBILE-1A), from sentry_issues.")),
	), func(ctx context.Context, request mcp.CallToolRequest) (any, error) {
		return a.sentryIssue(ctx, sentry, request)
	})

	a.addTool(s, mcp.NewTool("sentry_events",
		mcp.WithDescription("Recent occurrences of one Sentry issue: when, which user (Privy DID), release, environment, and request_id. Use it to see whether a bug hits one person or many, or whether it stopped after a fix."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("issue", mcp.Required(), mcp.Description("Issue id (numeric) or short id, from sentry_issues.")),
		mcp.WithNumber("count", mcp.Description("Occurrences to return, default 20, max 100.")),
	), func(ctx context.Context, request mcp.CallToolRequest) (any, error) {
		return a.sentryEvents(ctx, sentry, request)
	})
}

type sentryIssueRow struct {
	ID        string `json:"id"`
	ShortID   string `json:"shortId"`
	Title     string `json:"title"`
	Culprit   string `json:"culprit"`
	Level     string `json:"level"`
	Status    string `json:"status"`
	Count     string `json:"count"`
	UserCount int    `json:"userCount"`
	FirstSeen string `json:"firstSeen"`
	LastSeen  string `json:"lastSeen"`
	Permalink string `json:"permalink"`
}

func (a *reportCore) sentryIssues(ctx context.Context, c *sentryClient, request mcp.CallToolRequest) (any, error) {
	project := strings.TrimSpace(request.GetString("project", sentryDefaultProject))
	if !sentrySlug.MatchString(project) {
		return nil, errors.New("project must be a Sentry project slug")
	}

	var terms []string
	switch status := strings.TrimSpace(request.GetString("status", "unresolved")); status {
	case "", "unresolved":
		terms = append(terms, "is:unresolved")
	case "resolved", "ignored":
		terms = append(terms, "is:"+status)
	case "all":
	default:
		return nil, errors.New("status must be unresolved, resolved, ignored or all")
	}
	if version := strings.TrimSpace(request.GetString("version", "")); version != "" {
		if !sentryVersion.MatchString(version) {
			return nil, errors.New("version looks wrong; expected something like 2.2.0")
		}
		// Releases are org.sfluv.wallet@<version>+<build>.
		terms = append(terms, fmt.Sprintf("release:*@%s+*", version))
	}
	if search := strings.TrimSpace(request.GetString("search", "")); search != "" {
		terms = append(terms, search)
	}

	query := url.Values{}
	query.Set("query", strings.Join(terms, " "))
	query.Set("limit", strconv.Itoa(clamp(request.GetInt("count", 25), 1, 100)))
	switch sort := strings.TrimSpace(request.GetString("sort", "date")); sort {
	case "date", "new", "freq":
		query.Set("sort", sort)
	default:
		return nil, errors.New("sort must be date, new or freq")
	}
	switch period := strings.TrimSpace(request.GetString("period", "14d")); period {
	case "24h", "7d", "14d", "30d":
		query.Set("statsPeriod", period)
	default:
		return nil, errors.New("period must be 24h, 7d, 14d or 30d")
	}
	if environment := strings.TrimSpace(request.GetString("environment", "")); environment != "" {
		if !sentrySlug.MatchString(environment) {
			return nil, errors.New("environment must be preview, production or development")
		}
		query.Set("environment", environment)
	}

	var rows []sentryIssueRow
	if err := c.get(ctx, fmt.Sprintf("/projects/%s/%s/issues/", c.org, project), query, &rows); err != nil {
		return nil, err
	}
	return map[string]any{
		"generated_at": time.Now().UTC(),
		"project":      project,
		"query":        query.Get("query"),
		"issues":       rows,
	}, nil
}

// resolveIssueID accepts either form sentry_issues returns.
func (c *sentryClient) resolveIssueID(ctx context.Context, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if sentryIssueID.MatchString(raw) {
		return raw, nil
	}
	if !sentryShortID.MatchString(raw) {
		return "", errors.New("issue must be a numeric id or a short id like SFLUV-MOBILE-1A")
	}
	var resolved struct {
		GroupID string `json:"groupId"`
	}
	if err := c.get(ctx, fmt.Sprintf("/organizations/%s/shortids/%s/", c.org, url.PathEscape(raw)), nil, &resolved); err != nil {
		return "", err
	}
	if resolved.GroupID == "" {
		return "", errors.New("Sentry has no issue with that short id")
	}
	return resolved.GroupID, nil
}

type sentryEvent struct {
	EventID     string `json:"eventID"`
	DateCreated string `json:"dateCreated"`
	Title       string `json:"title"`
	Message     string `json:"message"`
	Platform    string `json:"platform"`
	Tags        []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"tags"`
	User *struct {
		ID string `json:"id"`
	} `json:"user"`
	Contexts map[string]json.RawMessage `json:"contexts"`
	Entries  []struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	} `json:"entries"`
}

type sentryFrame struct {
	Filename    string  `json:"filename"`
	AbsPath     string  `json:"absPath"`
	Function    string  `json:"function"`
	LineNo      *int    `json:"lineNo"`
	ColNo       *int    `json:"colNo"`
	InApp       bool    `json:"inApp"`
	ContextLine string  `json:"context_line,omitempty"`
	Context     [][]any `json:"context"`
}

// Contexts worth returning. Device and OS help with "only on old iPhones";
// the rest are the ones the app sets (request, wait, handled) or the SDK sets
// for over-the-air updates.
var sentryContextKeys = []string{"request", "wait", "handled", "ota_updates", "app", "device", "os"}

func summarizeEvent(event sentryEvent) map[string]any {
	tags := map[string]string{}
	for _, tag := range event.Tags {
		tags[tag.Key] = tag.Value
	}
	contexts := map[string]json.RawMessage{}
	for _, key := range sentryContextKeys {
		if value, ok := event.Contexts[key]; ok {
			contexts[key] = value
		}
	}

	out := map[string]any{
		"event_id":     event.EventID,
		"date_created": event.DateCreated,
		"title":        event.Title,
		"tags":         tags,
		"contexts":     contexts,
	}
	if event.Message != "" {
		out["message"] = event.Message
	}
	if event.User != nil && event.User.ID != "" {
		out["user_id"] = event.User.ID
	}

	for _, entry := range event.Entries {
		switch entry.Type {
		case "exception":
			var data struct {
				Values []struct {
					Type       string `json:"type"`
					Value      string `json:"value"`
					Stacktrace *struct {
						Frames []sentryFrame `json:"frames"`
					} `json:"stacktrace"`
				} `json:"values"`
			}
			if json.Unmarshal(entry.Data, &data) != nil {
				continue
			}
			var exceptions []map[string]any
			for _, value := range data.Values {
				exception := map[string]any{"type": value.Type, "value": value.Value}
				if value.Stacktrace != nil {
					exception["frames"] = appFrames(value.Stacktrace.Frames)
				}
				exceptions = append(exceptions, exception)
			}
			out["exceptions"] = exceptions
		case "breadcrumbs":
			var data struct {
				Values []struct {
					Timestamp string         `json:"timestamp"`
					Category  string         `json:"category"`
					Level     string         `json:"level"`
					Message   string         `json:"message"`
					Data      map[string]any `json:"data"`
				} `json:"values"`
			}
			if json.Unmarshal(entry.Data, &data) != nil {
				continue
			}
			values := data.Values
			if len(values) > sentryMaxBreadcrumbs {
				values = values[len(values)-sentryMaxBreadcrumbs:]
			}
			out["trail"] = values
		}
	}
	return out
}

// appFrames keeps the frames from the app's own code, innermost first, with the
// source line where Sentry has it. Library frames are counted, not listed: they
// are rarely where the fix goes and they bury the ones that are.
func appFrames(frames []sentryFrame) map[string]any {
	var app []map[string]any
	library := 0
	for i := len(frames) - 1; i >= 0; i-- {
		frame := frames[i]
		if !frame.InApp {
			library++
			continue
		}
		row := map[string]any{"file": frame.Filename, "function": frame.Function}
		if frame.LineNo != nil {
			row["line"] = *frame.LineNo
		}
		if frame.ColNo != nil {
			row["column"] = *frame.ColNo
		}
		for _, line := range frame.Context {
			if len(line) == 2 && frame.LineNo != nil {
				if n, ok := line[0].(float64); ok && int(n) == *frame.LineNo {
					if text, ok := line[1].(string); ok {
						row["source"] = strings.TrimSpace(text)
					}
				}
			}
		}
		app = append(app, row)
	}
	return map[string]any{"app_frames": app, "library_frames_omitted": library}
}

func (a *reportCore) sentryIssue(ctx context.Context, c *sentryClient, request mcp.CallToolRequest) (any, error) {
	id, err := c.resolveIssueID(ctx, request.GetString("issue", ""))
	if err != nil {
		return nil, err
	}
	var issue sentryIssueRow
	if err := c.get(ctx, fmt.Sprintf("/organizations/%s/issues/%s/", c.org, id), nil, &issue); err != nil {
		return nil, err
	}
	var latest sentryEvent
	if err := c.get(ctx, fmt.Sprintf("/organizations/%s/issues/%s/events/latest/", c.org, id), nil, &latest); err != nil {
		return nil, err
	}
	return map[string]any{
		"generated_at": time.Now().UTC(),
		"issue":        issue,
		"latest_event": summarizeEvent(latest),
	}, nil
}

func (a *reportCore) sentryEvents(ctx context.Context, c *sentryClient, request mcp.CallToolRequest) (any, error) {
	id, err := c.resolveIssueID(ctx, request.GetString("issue", ""))
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("limit", strconv.Itoa(clamp(request.GetInt("count", 20), 1, 100)))
	var events []sentryEvent
	if err := c.get(ctx, fmt.Sprintf("/organizations/%s/issues/%s/events/", c.org, id), query, &events); err != nil {
		return nil, err
	}
	wanted := []string{"release", "environment", "request_id", "request.status", "device", "os"}
	rows := make([]map[string]any, 0, len(events))
	for _, event := range events {
		row := map[string]any{"event_id": event.EventID, "date_created": event.DateCreated}
		if event.User != nil && event.User.ID != "" {
			row["user_id"] = event.User.ID
		}
		for _, tag := range event.Tags {
			for _, key := range wanted {
				if tag.Key == key {
					row[key] = tag.Value
				}
			}
		}
		rows = append(rows, row)
	}
	return map[string]any{
		"generated_at": time.Now().UTC(),
		"issue_id":     id,
		"events":       rows,
	}, nil
}

func clamp(value, low, high int) int {
	return max(low, min(high, value))
}
