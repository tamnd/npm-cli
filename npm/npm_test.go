package npm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamnd/npm-cli/npm"
)

func newTestClient(t *testing.T, mux *http.ServeMux) *npm.Client {
	t.Helper()
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := npm.DefaultConfig()
	cfg.RegistryURL = ts.URL
	cfg.DownloadsURL = ts.URL
	cfg.Rate = 0
	return npm.NewClient(cfg)
}

func TestUserAgent(t *testing.T) {
	var gotUA string
	mux := http.NewServeMux()
	mux.HandleFunc("/express", func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "express", "description": "Fast web framework",
			"dist-tags": map[string]any{"latest": "4.18.2"},
			"time":      map[string]any{"created": "2010-12-29", "modified": "2023-01-01"},
			"versions":  map[string]any{},
		})
	})
	c := newTestClient(t, mux)
	_, err := c.Info(context.Background(), "express")
	if err != nil {
		t.Fatal(err)
	}
	if gotUA == "" {
		t.Error("request carried no User-Agent")
	}
	if gotUA != npm.DefaultUserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, npm.DefaultUserAgent)
	}
}

func TestInfo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/express", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "express",
			"description": "Fast, unopinionated, minimalist web framework for node.",
			"dist-tags":   map[string]any{"latest": "4.18.2"},
			"keywords":    []string{"express", "framework", "web", "rest"},
			"homepage":    "https://expressjs.com/",
			"license":     "MIT",
			"author":      map[string]any{"name": "TJ Holowaychuk", "email": "tj@vision-media.ca"},
			"repository":  map[string]any{"url": "git+https://github.com/expressjs/express.git"},
			"time":        map[string]any{"created": "2010-12-29T19:38:25.450Z", "modified": "2023-08-28T13:00:00Z"},
			"versions":    map[string]any{},
		})
	})
	c := newTestClient(t, mux)
	pkg, err := c.Info(context.Background(), "express")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Name != "express" {
		t.Errorf("name = %q, want express", pkg.Name)
	}
	if pkg.Version != "4.18.2" {
		t.Errorf("version = %q, want 4.18.2", pkg.Version)
	}
	if pkg.Author != "TJ Holowaychuk" {
		t.Errorf("author = %q, want TJ Holowaychuk", pkg.Author)
	}
	if pkg.License != "MIT" {
		t.Errorf("license = %q, want MIT", pkg.License)
	}
	if len(pkg.Keywords) == 0 {
		t.Error("keywords should not be empty")
	}
}

func TestInfoStringAuthor(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/lodash", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "lodash",
			"description": "Lodash modular utilities.",
			"dist-tags":   map[string]any{"latest": "4.17.21"},
			"author":      "John-David Dalton <john.david.dalton@gmail.com>",
			"time":        map[string]any{"created": "2012-04-23", "modified": "2021-03-03"},
			"versions":    map[string]any{},
		})
	})
	c := newTestClient(t, mux)
	pkg, err := c.Info(context.Background(), "lodash")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Author != "John-David Dalton <john.david.dalton@gmail.com>" {
		t.Errorf("author = %q, want John-David Dalton...", pkg.Author)
	}
}

func TestVersion(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/react/18.2.0", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":    "react",
			"version": "18.2.0",
			"description": "React is a JavaScript library for building user interfaces.",
			"license": "MIT",
			"dependencies": map[string]any{
				"loose-envify": "^1.1.0",
			},
			"devDependencies": map[string]any{},
		})
	})
	c := newTestClient(t, mux)
	v, err := c.Version(context.Background(), "react", "18.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "react" {
		t.Errorf("name = %q, want react", v.Name)
	}
	if v.Version != "18.2.0" {
		t.Errorf("version = %q, want 18.2.0", v.Version)
	}
	if v.License != "MIT" {
		t.Errorf("license = %q, want MIT", v.License)
	}
	if len(v.Dependencies) != 1 {
		t.Errorf("dependencies count = %d, want 1", len(v.Dependencies))
	}
}

func TestSearch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/-/v1/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("text") == "" {
			t.Error("search query missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total": 12345,
			"objects": []any{
				map[string]any{
					"package": map[string]any{
						"name": "react", "version": "18.2.0",
						"description": "A JavaScript library for building user interfaces.",
						"keywords":    []string{"react"},
						"links":       map[string]any{"npm": "https://www.npmjs.com/package/react"},
					},
					"score": map[string]any{"final": 0.9432, "detail": map[string]any{
						"quality": 0.9, "popularity": 0.98, "maintenance": 0.95,
					}},
				},
				map[string]any{
					"package": map[string]any{
						"name": "react-dom", "version": "18.2.0",
						"description": "React package for working with the DOM.",
						"keywords":    []string{"react"},
						"links":       map[string]any{"npm": "https://www.npmjs.com/package/react-dom"},
					},
					"score": map[string]any{"final": 0.9012, "detail": map[string]any{
						"quality": 0.88, "popularity": 0.97, "maintenance": 0.95,
					}},
				},
				map[string]any{
					"package": map[string]any{
						"name": "react-router", "version": "6.15.0",
						"description": "Declarative routing for React",
						"keywords":    []string{"react", "router"},
						"links":       map[string]any{"npm": "https://www.npmjs.com/package/react-router"},
					},
					"score": map[string]any{"final": 0.8823, "detail": map[string]any{
						"quality": 0.85, "popularity": 0.95, "maintenance": 0.92,
					}},
				},
			},
		})
	})
	c := newTestClient(t, mux)
	results, err := c.Search(context.Background(), "react", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	if results[0].Name != "react" {
		t.Errorf("first result = %q, want react", results[0].Name)
	}
	if results[0].Rank != 1 {
		t.Errorf("first rank = %d, want 1", results[0].Rank)
	}
	if results[0].ScoreFinal != 0.9432 {
		t.Errorf("score = %f, want 0.9432", results[0].ScoreFinal)
	}
}

func TestDownloads(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/downloads/point/last-week/react", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"downloads": 142126952,
			"package":   "react",
			"start":     "2023-11-06",
			"end":       "2023-11-12",
		})
	})
	c := newTestClient(t, mux)
	dl, err := c.Downloads(context.Background(), "react", "last-week")
	if err != nil {
		t.Fatal(err)
	}
	if dl.Package != "react" {
		t.Errorf("package = %q, want react", dl.Package)
	}
	if dl.Downloads != 142126952 {
		t.Errorf("downloads = %d, want 142126952", dl.Downloads)
	}
}

func TestDeps(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/express", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":      "express",
			"dist-tags": map[string]any{"latest": "4.18.2"},
			"time":      map[string]any{"created": "", "modified": ""},
			"versions": map[string]any{
				"4.18.2": map[string]any{
					"name": "express", "version": "4.18.2",
					"dependencies": map[string]any{
						"accepts":       "~1.3.8",
						"array-flatten": "1.1.1",
						"body-parser":   "1.20.1",
					},
					"devDependencies": map[string]any{
						"mocha": "^9.1.3",
					},
				},
			},
		})
	})
	c := newTestClient(t, mux)
	deps, err := c.Deps(context.Background(), "express")
	if err != nil {
		t.Fatal(err)
	}
	// Should have 3 deps + 1 devDep = 4 total
	if len(deps) != 4 {
		t.Fatalf("got %d deps, want 4", len(deps))
	}
	// Deps are sorted alphabetically; first should be "accepts"
	if deps[0].Name != "accepts" {
		t.Errorf("first dep = %q, want accepts", deps[0].Name)
	}
	if deps[0].Kind != "dep" {
		t.Errorf("first dep kind = %q, want dep", deps[0].Kind)
	}
	// devDep should be last
	last := deps[len(deps)-1]
	if last.Name != "mocha" {
		t.Errorf("last dep = %q, want mocha", last.Name)
	}
	if last.Kind != "devDep" {
		t.Errorf("last dep kind = %q, want devDep", last.Kind)
	}
}

func TestInfo404(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/nonexistent-package-xyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	c := newTestClient(t, mux)
	_, err := c.Info(context.Background(), "nonexistent-package-xyz")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestRetryOn503(t *testing.T) {
	var hits int
	mux := http.NewServeMux()
	mux.HandleFunc("/mypackage", func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":      "mypackage",
			"dist-tags": map[string]any{"latest": "1.0.0"},
			"time":      map[string]any{"created": "", "modified": ""},
			"versions":  map[string]any{},
		})
	})
	c := newTestClient(t, mux)
	pkg, err := c.Info(context.Background(), "mypackage")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Name != "mypackage" {
		t.Errorf("name = %q, want mypackage", pkg.Name)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
}
