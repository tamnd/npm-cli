package npm

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring, which need no network.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "npm" {
		t.Errorf("Scheme = %q, want npm", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want first=%s", info.Hosts, Host)
	}
	if info.Identity.Binary != "npmcli" {
		t.Errorf("Identity.Binary = %q, want npmcli", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"express", "package", "express"},
		{"@scope/pkg", "package", "@scope/pkg"},
		{"https://www.npmjs.com/package/react", "package", "react"},
		{"https://npmjs.com/package/@babel/core", "package", "@babel/core"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("package", "react")
	want := "https://www.npmjs.com/package/react"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateScoped(t *testing.T) {
	got, err := Domain{}.Locate("package", "@babel/core")
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Error("Locate returned empty URL for scoped package")
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "react")
	if err == nil {
		t.Error("expected error for unknown URI type, got nil")
	}
}

// TestHostWiring mounts the driver in a kit Host and checks the round trip.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	p := &Package{
		Name:    "express",
		Version: "4.18.2",
		URL:     "https://www.npmjs.com/package/express",
	}
	u, err := h.Mint(p)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "npm://package/express"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("npm", "react")
	if err != nil || got.String() != "npm://package/react" {
		t.Errorf("ResolveOn = (%q, %v), want npm://package/react", got.String(), err)
	}
}
