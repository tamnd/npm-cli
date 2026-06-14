package npm

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes the npm registry as a kit Domain: a driver that a
// multi-domain host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/npm-cli/npm"
//
// The same Domain also builds the standalone npmcli binary (see cli.NewApp),
// so the binary and any host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the npm registry driver.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for help and version output.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "npm",
		Hosts:  []string{Host, "www.npmjs.com", "npmjs.com"},
		Identity: kit.Identity{
			Binary: "npmcli",
			Short:  "A command line for the npm package registry.",
			Long: `npmcli reads public npm package data over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools. No API
key is required.

npmcli is an independent tool and is not affiliated with npm, Inc. or Microsoft.`,
			Site: "registry.npmjs.org",
			Repo: "https://github.com/tamnd/npm-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// info: full package metadata for the latest version.
	kit.Handle(app, kit.OpMeta{Name: "info", Group: "read", Single: true,
		Summary: "Show package info (latest version, author, keywords, repo)",
		URIType: "package", Resolver: true,
		Args: []kit.Arg{{Name: "name", Help: "package name or npmjs.com URL"}}}, getInfo)

	// version: specific version metadata.
	kit.Handle(app, kit.OpMeta{Name: "version", Group: "read", Single: true,
		Summary: "Show metadata for a specific package version",
		URIType: "package",
		Args: []kit.Arg{
			{Name: "name", Help: "package name"},
			{Name: "version", Help: "version string (e.g. 1.2.3)"},
		}}, getVersion)

	// deps: dependencies for the latest version.
	kit.Handle(app, kit.OpMeta{Name: "deps", Group: "read", List: true,
		Summary: "List dependencies for the latest version",
		URIType: "package",
		Args:    []kit.Arg{{Name: "name", Help: "package name or npmjs.com URL"}}}, listDeps)

	// search: full-text package search.
	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read",
		Summary: "Search packages on the npm registry",
		Args:    []kit.Arg{{Name: "query", Help: "search terms", Variadic: true}}}, search)

	// downloads: download stats for a period.
	kit.Handle(app, kit.OpMeta{Name: "downloads", Group: "read", Single: true,
		Summary: "Show download counts for a package",
		Args:    []kit.Arg{{Name: "name", Help: "package name"}}}, getDownloads)
}

// newClient builds the npm client from the kit-resolved config.
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
	return NewClient(c), nil
}

// ─── input structs ────────────────────────────────────────────────────────────

type nameRef struct {
	Name   string  `kit:"arg" help:"package name or npmjs.com URL"`
	Client *Client `kit:"inject"`
}

type versionRef struct {
	Name    string  `kit:"arg" help:"package name"`
	Version string  `kit:"arg" help:"version string"`
	Client  *Client `kit:"inject"`
}

type depsRef struct {
	Name   string  `kit:"arg" help:"package name or npmjs.com URL"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type searchRef struct {
	Query  []string `kit:"arg,variadic" help:"search terms"`
	Limit  int      `kit:"flag,inherit" help:"max results"`
	Client *Client  `kit:"inject"`
}

type downloadsRef struct {
	Name   string  `kit:"arg" help:"package name"`
	Period string  `kit:"flag" help:"period: last-week|last-month|last-year" default:"last-week"`
	Client *Client `kit:"inject"`
}

// ─── handlers ─────────────────────────────────────────────────────────────────

func getInfo(ctx context.Context, in nameRef, emit func(*Package) error) error {
	pkg, err := in.Client.Info(ctx, packageName(in.Name))
	if err != nil {
		return mapErr(err)
	}
	return emit(pkg)
}

func getVersion(ctx context.Context, in versionRef, emit func(*PackageVersion) error) error {
	v, err := in.Client.Version(ctx, in.Name, in.Version)
	if err != nil {
		return mapErr(err)
	}
	return emit(v)
}

func listDeps(ctx context.Context, in depsRef, emit func(Dependency) error) error {
	deps, err := in.Client.Deps(ctx, packageName(in.Name))
	if err != nil {
		return mapErr(err)
	}
	for _, d := range deps {
		if err := emit(d); err != nil {
			return err
		}
	}
	return nil
}

func search(ctx context.Context, in searchRef, emit func(SearchResult) error) error {
	results, err := in.Client.Search(ctx, strings.Join(in.Query, " "), in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, r := range results {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

func getDownloads(ctx context.Context, in downloadsRef, emit func(*Downloads) error) error {
	period := in.Period
	if period == "" {
		period = "last-week"
	}
	dl, err := in.Client.Downloads(ctx, in.Name, period)
	if err != nil {
		return mapErr(err)
	}
	return emit(dl)
}

// ─── URI driver ───────────────────────────────────────────────────────────────

// Classify turns any accepted input into the canonical (type, id), so
// `ant resolve` and `ant url` touch no network.
func (Domain) Classify(input string) (uriType, id string, err error) {
	id = packageName(input)
	if id == "" {
		return "", "", errs.Usage("unrecognized npm reference: %q", input)
	}
	return "package", id, nil
}

// Locate returns the live npmjs.com URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	if uriType != "package" {
		return "", errs.Usage("npm has no resource type %q", uriType)
	}
	return "https://www.npmjs.com/package/" + url.PathEscape(id), nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// packageName extracts the bare package name from any accepted input:
// a bare name, a scoped name like @scope/pkg, or a full npmjs.com URL.
func packageName(input string) string {
	input = strings.TrimSpace(input)
	if u, err := url.Parse(input); err == nil &&
		(u.Scheme == "http" || u.Scheme == "https") {
		// https://www.npmjs.com/package/@scope/pkg -> @scope/pkg
		path := strings.TrimPrefix(u.Path, "/package/")
		path = strings.Trim(path, "/")
		if path != "" {
			return path
		}
	}
	return strings.Trim(input, "/")
}

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return errs.NotFound("%s", err.Error())
	default:
		return err
	}
}
