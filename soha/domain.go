package soha

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

func init() { kit.Register(Domain{}) }

// Domain is the Soha driver.
type Domain struct{}

func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "soha",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "soha",
			Short:  "Read public Soha (soha.vn) news articles.",
			Long: `Read public Soha (soha.vn) news articles.

soha reads from Soha RSS feeds — no API key, no browser required.
Returns clean JSON records ready for jq, sqlite-utils, and shell pipelines.`,
			Site: Host,
			Repo: "https://github.com/tamnd/soha-cli",
		},
	}
}

func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "article", Group: "read", Single: true,
		URIType: "article", Resolver: true,
		Summary: "Resolve a Soha article ID to its URL",
		Args:    []kit.Arg{{Name: "id", Help: "numeric article ID"}}}, getArticle)

	kit.Handle(app, kit.OpMeta{Name: "latest", Group: "read", List: true,
		URIType: "article",
		Summary: "Fetch the latest Soha articles"}, getLatest)

	kit.Handle(app, kit.OpMeta{Name: "category", Group: "read", List: true,
		URIType: "article",
		Summary: "Fetch articles for a category (e.g. viet-nam, kinh-te)",
		Args:    []kit.Arg{{Name: "slug", Help: "category slug"}}}, getCategoryArticles)

	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		URIType: "article",
		Summary: "Search Soha articles by keyword",
		Args:    []kit.Arg{{Name: "query", Help: "search keyword"}}}, searchArticles)

	kit.Handle(app, kit.OpMeta{Name: "categories", Group: "read", List: true,
		Summary: "List all Soha RSS feed categories"}, listCategories)
}

func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	return NewClientWithConfig(c), nil
}

type articleInput struct {
	ID     string  `kit:"arg"   help:"numeric article ID"`
	Client *Client `kit:"inject"`
}

type latestInput struct {
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type categoryInput struct {
	Slug   string  `kit:"arg"          help:"category slug"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type searchInput struct {
	Query  string  `kit:"arg"          help:"search keyword"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type categoriesInput struct {
	Client *Client `kit:"inject"`
}

func getArticle(_ context.Context, in articleInput, emit func(*Article) error) error {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		return errs.Usage("article id is required")
	}
	return emit(&Article{
		ID:  id,
		URL: baseURL + "/article-" + id + ".htm",
	})
}

func getLatest(ctx context.Context, in latestInput, emit func(*Article) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	articles, err := in.Client.LatestArticles(ctx, limit)
	if err != nil {
		return err
	}
	for _, a := range articles {
		if err := emit(a); err != nil {
			return err
		}
	}
	return nil
}

func getCategoryArticles(ctx context.Context, in categoryInput, emit func(*Article) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	articles, err := in.Client.CategoryArticles(ctx, in.Slug, limit)
	if err != nil {
		return err
	}
	for _, a := range articles {
		if err := emit(a); err != nil {
			return err
		}
	}
	return nil
}

func searchArticles(ctx context.Context, in searchInput, emit func(*Article) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	articles, err := in.Client.SearchArticles(ctx, in.Query, limit)
	if err != nil {
		return err
	}
	for _, a := range articles {
		if err := emit(a); err != nil {
			return err
		}
	}
	return nil
}

func listCategories(_ context.Context, in categoriesInput, emit func(*Category) error) error {
	for _, cat := range in.Client.ListCategories() {
		if err := emit(cat); err != nil {
			return err
		}
	}
	return nil
}

// Classify turns a Soha URL or numeric ID into (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty Soha reference")
	}
	if strings.Contains(input, "soha.vn/") {
		id = extractArticleID(input)
		if id != "" {
			return "article", id, nil
		}
	}
	if isDigits(input) {
		return "article", input, nil
	}
	for _, slug := range Categories {
		if strings.EqualFold(input, slug) {
			return "category", slug, nil
		}
	}
	return "", "", errs.Usage("unrecognized Soha reference: %q", input)
}

// Locate returns the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "article":
		return baseURL + "/article-" + id + ".htm", nil
	case "category":
		return baseURL + "/" + id + ".htm", nil
	default:
		return "", errs.Usage("soha has no resource type %q", uriType)
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
