// Package npm is the library behind the npmcli command line:
// the HTTP client, request shaping, and the typed data models for the npm
// package registry.
//
// Two public APIs: the registry at registry.npmjs.org for package metadata and
// the downloads API at api.npmjs.org for download statistics. Both are open and
// require no key.
package npm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultUserAgent identifies the client to npm APIs.
const DefaultUserAgent = "npmcli/0.1.0 (github.com/tamnd/npm-cli)"

// Host is the primary registry hostname.
const Host = "registry.npmjs.org"

// ErrNotFound is returned when the registry returns a 404 for a package.
var ErrNotFound = errors.New("not found")

// Config holds constructor parameters for both endpoints.
type Config struct {
	RegistryURL  string
	DownloadsURL string
	UserAgent    string
	Rate         time.Duration
	Timeout      time.Duration
	Retries      int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		RegistryURL:  "https://registry.npmjs.org",
		DownloadsURL: "https://api.npmjs.org",
		UserAgent:    DefaultUserAgent,
		Rate:         200 * time.Millisecond,
		Timeout:      30 * time.Second,
		Retries:      3,
	}
}

// Client talks to the npm registry and downloads APIs.
type Client struct {
	httpClient   *http.Client
	registryURL  string
	downloadsURL string
	userAgent    string
	rate         time.Duration
	retries      int
	mu           sync.Mutex
	last         time.Time
}

// NewClient returns a Client with the given config.
func NewClient(cfg Config) *Client {
	regURL := cfg.RegistryURL
	if regURL == "" {
		regURL = "https://registry.npmjs.org"
	}
	dlURL := cfg.DownloadsURL
	if dlURL == "" {
		dlURL = "https://api.npmjs.org"
	}
	return &Client{
		httpClient:   &http.Client{Timeout: cfg.Timeout},
		registryURL:  regURL,
		downloadsURL: dlURL,
		userAgent:    cfg.UserAgent,
		rate:         cfg.Rate,
		retries:      cfg.Retries,
	}
}

// get fetches a URL with pacing and retries.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
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
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, fmt.Errorf("http 404")
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rate <= 0 {
		return
	}
	if wait := c.rate - time.Since(c.last); wait > 0 {
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

// getJSON fetches and JSON-decodes into v. Returns ErrNotFound on 404.
func (c *Client) getJSON(ctx context.Context, rawURL string, v any) error {
	body, err := c.get(ctx, rawURL)
	if err != nil {
		if strings.Contains(err.Error(), "http 404") {
			return ErrNotFound
		}
		return err
	}
	if strings.TrimSpace(string(body)) == "null" {
		return ErrNotFound
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode %s: %w", rawURL, err)
	}
	return nil
}

// ─── wire types ──────────────────────────────────────────────────────────────

// flexAuthor handles the npm author field which can be a string or an object.
type flexAuthor struct{ Name string }

func (fa *flexAuthor) UnmarshalJSON(b []byte) error {
	// Try string first.
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		fa.Name = s
		return nil
	}
	// Try object.
	var obj struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	fa.Name = obj.Name
	return nil
}

// flexRepo handles the repository field which can be a string or an object.
type flexRepo struct{ URL string }

func (fr *flexRepo) UnmarshalJSON(b []byte) error {
	// Try string first.
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		fr.URL = cleanRepoURL(s)
		return nil
	}
	// Try object.
	var obj struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	fr.URL = cleanRepoURL(obj.URL)
	return nil
}

func cleanRepoURL(s string) string {
	s = strings.TrimPrefix(s, "git+")
	s = strings.TrimPrefix(s, "git://")
	if strings.HasPrefix(s, "github.com/") {
		s = "https://" + s
	}
	return s
}

type wirePackage struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	DistTags    map[string]string `json:"dist-tags"`
	Keywords    []string          `json:"keywords"`
	Homepage    string            `json:"homepage"`
	License     interface{}       `json:"license"` // can be string or object
	Author      *flexAuthor       `json:"author"`
	Repository  *flexRepo         `json:"repository"`
	Readme      string            `json:"readme"`
	Time        struct {
		Created  string `json:"created"`
		Modified string `json:"modified"`
	} `json:"time"`
	Versions map[string]wireVersion `json:"versions"`
}

type wireVersion struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Description     string            `json:"description"`
	Keywords        []string          `json:"keywords"`
	Homepage        string            `json:"homepage"`
	License         interface{}       `json:"license"`
	Author          *flexAuthor       `json:"author"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Repository      *flexRepo         `json:"repository"`
}

type wireSearch struct {
	Total   int `json:"total"`
	Objects []struct {
		Package struct {
			Name        string   `json:"name"`
			Version     string   `json:"version"`
			Description string   `json:"description"`
			Keywords    []string `json:"keywords"`
			Links       struct {
				NPM        string `json:"npm"`
				Homepage   string `json:"homepage"`
				Repository string `json:"repository"`
			} `json:"links"`
		} `json:"package"`
		Score struct {
			Final  float64 `json:"final"`
			Detail struct {
				Quality     float64 `json:"quality"`
				Popularity  float64 `json:"popularity"`
				Maintenance float64 `json:"maintenance"`
			} `json:"detail"`
		} `json:"score"`
	} `json:"objects"`
}

type wireDownloads struct {
	Downloads int    `json:"downloads"`
	Package   string `json:"package"`
	Start     string `json:"start"`
	End       string `json:"end"`
}

// ─── public types ─────────────────────────────────────────────────────────────

// Package is the record returned by the info command.
type Package struct {
	Name        string   `json:"name" kit:"id"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Keywords    []string `json:"keywords"`
	Homepage    string   `json:"homepage"`
	License     string   `json:"license"`
	Author      string   `json:"author"`
	Repository  string   `json:"repository"`
	Created     string   `json:"created"`
	Modified    string   `json:"modified"`
	Readme      string   `json:"readme,omitempty"`
	URL         string   `json:"url"`
}

// PackageVersion is the record returned by the version command.
type PackageVersion struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Description     string            `json:"description"`
	Keywords        []string          `json:"keywords"`
	Homepage        string            `json:"homepage"`
	License         string            `json:"license"`
	Author          string            `json:"author"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"dev_dependencies"`
	URL             string            `json:"url"`
}

// Dependency is a flat record for one dependency entry.
type Dependency struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint"`
	Kind       string `json:"kind"` // "dep" or "devDep"
}

// SearchResult is one hit from the npm search API.
type SearchResult struct {
	Rank        int      `json:"rank"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
	ScoreFinal  float64  `json:"score_final"`
	URL         string   `json:"url"`
}

// Downloads is the record returned by the downloads command.
type Downloads struct {
	Package   string `json:"package"`
	Downloads int    `json:"downloads"`
	Start     string `json:"start"`
	End       string `json:"end"`
}

// ─── client methods ───────────────────────────────────────────────────────────

// Info fetches full package metadata for the latest version.
func (c *Client) Info(ctx context.Context, name string) (*Package, error) {
	var w wirePackage
	u := fmt.Sprintf("%s/%s", c.registryURL, url.PathEscape(name))
	if err := c.getJSON(ctx, u, &w); err != nil {
		return nil, err
	}
	latest := w.DistTags["latest"]
	author := ""
	if w.Author != nil {
		author = w.Author.Name
	}
	// Check per-version author if top-level is missing.
	if author == "" && latest != "" {
		if v, ok := w.Versions[latest]; ok && v.Author != nil {
			author = v.Author.Name
		}
	}
	repo := ""
	if w.Repository != nil {
		repo = w.Repository.URL
	}
	license := extractLicense(w.License)
	return &Package{
		Name:        w.Name,
		Description: w.Description,
		Version:     latest,
		Keywords:    w.Keywords,
		Homepage:    w.Homepage,
		License:     license,
		Author:      author,
		Repository:  repo,
		Created:     w.Time.Created,
		Modified:    w.Time.Modified,
		Readme:      w.Readme,
		URL:         fmt.Sprintf("https://www.npmjs.com/package/%s", url.PathEscape(name)),
	}, nil
}

// Version fetches metadata for a specific version of a package.
func (c *Client) Version(ctx context.Context, name, version string) (*PackageVersion, error) {
	var w wireVersion
	u := fmt.Sprintf("%s/%s/%s", c.registryURL, url.PathEscape(name), url.PathEscape(version))
	if err := c.getJSON(ctx, u, &w); err != nil {
		return nil, err
	}
	author := ""
	if w.Author != nil {
		author = w.Author.Name
	}
	deps := w.Dependencies
	if deps == nil {
		deps = map[string]string{}
	}
	devDeps := w.DevDependencies
	if devDeps == nil {
		devDeps = map[string]string{}
	}
	return &PackageVersion{
		Name:            w.Name,
		Version:         w.Version,
		Description:     w.Description,
		Keywords:        w.Keywords,
		Homepage:        w.Homepage,
		License:         extractLicense(w.License),
		Author:          author,
		Dependencies:    deps,
		DevDependencies: devDeps,
		URL:             fmt.Sprintf("https://www.npmjs.com/package/%s/v/%s", url.PathEscape(name), url.PathEscape(version)),
	}, nil
}

// Search searches the npm registry.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	u := fmt.Sprintf("%s/-/v1/search?text=%s&size=%d", c.registryURL, url.QueryEscape(query), limit)
	var w wireSearch
	if err := c.getJSON(ctx, u, &w); err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, len(w.Objects))
	for i, obj := range w.Objects {
		out = append(out, SearchResult{
			Rank:        i + 1,
			Name:        obj.Package.Name,
			Version:     obj.Package.Version,
			Description: obj.Package.Description,
			Keywords:    obj.Package.Keywords,
			ScoreFinal:  obj.Score.Final,
			URL:         obj.Package.Links.NPM,
		})
	}
	return out, nil
}

// Downloads fetches download counts for a package over a period.
// period can be "last-week", "last-month", "last-year", or a date range "YYYY-MM-DD:YYYY-MM-DD".
func (c *Client) Downloads(ctx context.Context, name, period string) (*Downloads, error) {
	if period == "" {
		period = "last-week"
	}
	u := fmt.Sprintf("%s/downloads/point/%s/%s", c.downloadsURL, period, url.PathEscape(name))
	var w wireDownloads
	if err := c.getJSON(ctx, u, &w); err != nil {
		return nil, err
	}
	return &Downloads{
		Package:   w.Package,
		Downloads: w.Downloads,
		Start:     w.Start,
		End:       w.End,
	}, nil
}

// Deps fetches the dependencies for the latest version of a package.
func (c *Client) Deps(ctx context.Context, name string) ([]Dependency, error) {
	// Get the latest version from the info endpoint.
	var w wirePackage
	u := fmt.Sprintf("%s/%s", c.registryURL, url.PathEscape(name))
	if err := c.getJSON(ctx, u, &w); err != nil {
		return nil, err
	}
	latest := w.DistTags["latest"]
	if latest == "" {
		return nil, ErrNotFound
	}
	v, ok := w.Versions[latest]
	if !ok {
		return nil, ErrNotFound
	}
	// Collect and sort deps for deterministic output.
	var out []Dependency
	depNames := make([]string, 0, len(v.Dependencies))
	for n := range v.Dependencies {
		depNames = append(depNames, n)
	}
	sort.Strings(depNames)
	for _, n := range depNames {
		out = append(out, Dependency{Name: n, Constraint: v.Dependencies[n], Kind: "dep"})
	}
	devNames := make([]string, 0, len(v.DevDependencies))
	for n := range v.DevDependencies {
		devNames = append(devNames, n)
	}
	sort.Strings(devNames)
	for _, n := range devNames {
		out = append(out, Dependency{Name: n, Constraint: v.DevDependencies[n], Kind: "devDep"})
	}
	return out, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// extractLicense handles the license field which can be a string or an object.
func extractLicense(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case map[string]interface{}:
		if typ, ok := t["type"].(string); ok {
			return typ
		}
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}
