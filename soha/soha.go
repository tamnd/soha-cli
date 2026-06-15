// Package soha is the library behind the soha command line:
// the HTTP client, RSS feed parsing, and typed data models for Soha
// (soha.vn), Vietnam's tenth-most-visited news website (VCCorp portal).
//
// Soha publishes per-category RSS 2.0 feeds. Article URLs embed a
// numeric ID: https://soha.vn/{slug}-{id}.htm.
package soha

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Host is the canonical site hostname.
const Host = "soha.vn"

// baseURL is the site root.
const baseURL = "https://soha.vn"

// DefaultUserAgent identifies this client to Soha.
const DefaultUserAgent = "soha-cli/0.1.0 (+https://github.com/tamnd/soha-cli)"

// Categories lists the Soha RSS feed category slugs.
var Categories = []string{
	"home",
	"viet-nam",
	"quoc-te",
	"xa-hoi",
	"kinh-te",
	"thi-truong",
	"giao-duc",
	"suc-khoe",
	"giai-tri",
	"the-thao",
	"cong-nghe",
	"quan-su",
}

var categoryNames = map[string]string{
	"home":       "Trang chủ",
	"viet-nam":   "Việt Nam",
	"quoc-te":    "Quốc tế",
	"xa-hoi":     "Xã hội",
	"kinh-te":    "Kinh tế",
	"thi-truong": "Thị trường",
	"giao-duc":   "Giáo dục",
	"suc-khoe":   "Sức khỏe",
	"giai-tri":   "Giải trí",
	"the-thao":   "Thể thao",
	"cong-nghe":  "Công nghệ",
	"quan-su":    "Quân sự",
}

// Config holds the tunable knobs for the HTTP client.
type Config struct {
	BaseURL   string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   baseURL,
		Rate:      500 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
		UserAgent: DefaultUserAgent,
	}
}

// Client talks to Soha RSS feeds over HTTP.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client from DefaultConfig.
func NewClient() *Client { return NewClientWithConfig(DefaultConfig()) }

// NewClientWithConfig returns a Client built from cfg.
func NewClientWithConfig(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

// Get fetches rawURL and returns the body, pacing and retrying.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml, */*")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	return b, err != nil, err
}

func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- wire types ---

type wireRSS struct {
	XMLName xml.Name    `xml:"rss"`
	Channel wireChannel `xml:"channel"`
}

type wireChannel struct {
	Title string     `xml:"title"`
	Items []wireItem `xml:"item"`
}

type wireItem struct {
	Title       string        `xml:"title"`
	Link        string        `xml:"link"`
	Description string        `xml:"description"`
	PubDate     string        `xml:"pubDate"`
	Author      string        `xml:"author"`
	Creator     string        `xml:"creator"`
	Thumb       string        `xml:"thumb"`
	Enclosure   wireEnclosure `xml:"enclosure"`
}

type wireEnclosure struct {
	URL  string `xml:"url,attr"`
	Type string `xml:"type,attr"`
}

// --- public types ---

// Article is one Soha news article extracted from an RSS feed.
type Article struct {
	ID          string `json:"id"                   kit:"id" table:"id"`
	Title       string `json:"title"                          table:"title"`
	URL         string `json:"url,omitempty"                  table:"url,url"`
	Category    string `json:"category,omitempty"             table:"category"`
	Description string `json:"description,omitempty"          table:"-"`
	Author      string `json:"author,omitempty"               table:"author"`
	Thumbnail   string `json:"thumbnail,omitempty"            table:"-"`
	PublishedAt string `json:"published_at,omitempty"         table:"published_at"`
}

// Category represents one Soha RSS feed category.
type Category struct {
	Slug string `json:"slug" kit:"id" table:"slug"`
	Name string `json:"name"          table:"name"`
	URL  string `json:"url"           table:"url,url"`
	RSS  string `json:"rss"           table:"-"`
}

// --- client methods ---

// LatestArticles fetches the most recent articles from the home feed.
func (c *Client) LatestArticles(ctx context.Context, limit int) ([]*Article, error) {
	return c.CategoryArticles(ctx, "home", limit)
}

// CategoryArticles fetches articles for the given category slug.
func (c *Client) CategoryArticles(ctx context.Context, slug string, limit int) ([]*Article, error) {
	if limit <= 0 {
		limit = 20
	}
	body, err := c.Get(ctx, c.rssURL(slug))
	if err != nil {
		return nil, fmt.Errorf("feed %s: %w", slug, err)
	}
	items, err := parseRSS(body)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", slug, err)
	}
	out := make([]*Article, 0, len(items))
	for _, item := range items {
		a := articleFromWire(item, slug)
		if a == nil {
			continue
		}
		out = append(out, a)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// SearchArticles keyword-searches several category feeds.
func (c *Client) SearchArticles(ctx context.Context, query string, limit int) ([]*Article, error) {
	if limit <= 0 {
		limit = 20
	}
	q := strings.ToLower(query)
	seen := map[string]bool{}
	var out []*Article

	for _, slug := range []string{"home", "viet-nam", "quoc-te", "kinh-te", "xa-hoi"} {
		if len(out) >= limit {
			break
		}
		body, err := c.Get(ctx, c.rssURL(slug))
		if err != nil {
			continue
		}
		items, err := parseRSS(body)
		if err != nil {
			continue
		}
		for _, item := range items {
			if len(out) >= limit {
				break
			}
			a := articleFromWire(item, slug)
			if a == nil || seen[a.ID] {
				continue
			}
			if strings.Contains(strings.ToLower(a.Title), q) ||
				strings.Contains(strings.ToLower(a.Description), q) {
				seen[a.ID] = true
				out = append(out, a)
			}
		}
	}
	return out, nil
}

// ListCategories returns all known Soha RSS feed categories.
func (c *Client) ListCategories() []*Category {
	base := c.cfg.BaseURL
	if base == "" {
		base = baseURL
	}
	out := make([]*Category, 0, len(Categories))
	for _, slug := range Categories {
		name := categoryNames[slug]
		if name == "" {
			name = slug
		}
		out = append(out, &Category{
			Slug: slug,
			Name: name,
			URL:  base + "/" + slug + ".htm",
			RSS:  c.rssURL(slug),
		})
	}
	return out
}

func (c *Client) rssURL(slug string) string {
	base := c.cfg.BaseURL
	if base == "" {
		base = baseURL
	}
	return base + "/rss/" + slug + ".rss"
}

// --- parsing ---

func parseRSS(body []byte) ([]wireItem, error) {
	var feed wireRSS
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("xml decode: %w", err)
	}
	return feed.Channel.Items, nil
}

// articleIDRE extracts the numeric ID from Soha article URLs.
// Pattern: https://soha.vn/{slug}-{id}.htm
var articleIDRE = regexp.MustCompile(`-(\d{5,})\.htm$`)

func articleFromWire(item wireItem, category string) *Article {
	link := strings.TrimSpace(item.Link)
	if link == "" {
		return nil
	}
	id := extractArticleID(link)
	if id == "" {
		id = link
	}

	author := strings.TrimSpace(item.Author)
	if author == "" {
		author = strings.TrimSpace(item.Creator)
	}

	thumb := strings.TrimSpace(item.Thumb)
	if thumb == "" && strings.HasPrefix(item.Enclosure.Type, "image/") {
		thumb = item.Enclosure.URL
	}

	return &Article{
		ID:          id,
		Title:       strings.TrimSpace(item.Title),
		URL:         link,
		Category:    category,
		Description: strings.TrimSpace(item.Description),
		Author:      author,
		Thumbnail:   thumb,
		PublishedAt: parseRFC1123(item.PubDate),
	}
}

func extractArticleID(rawURL string) string {
	m := articleIDRE.FindStringSubmatch(rawURL)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func parseRFC1123(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, "Mon, 02 Jan 2006 15:04:05 -0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return s
}
