# npm-cli

`npmcli` — a command line for the npm package registry.

A single pure-Go binary that reads public npm data over plain HTTPS, shapes it
into clean records, and prints output that pipes into the rest of your tools.
No API key, nothing to run alongside it.

The same package is also a [resource-URI driver](#use-it-as-a-resource-uri-driver),
so a host program like [ant](https://github.com/tamnd/ant) can address
npm packages as `npm://` URIs.

## Install

```bash
go install github.com/tamnd/npm-cli/cmd/npm@latest
```

Or grab a prebuilt binary from the [releases](https://github.com/tamnd/npm-cli/releases), or run
the container image:

```bash
docker run --rm ghcr.io/tamnd/npmcli:latest --help
```

## Usage

```bash
npmcli info express                     # latest version, author, keywords, repo
npmcli info express -o json             # as JSON, ready for jq
npmcli version react 18.2.0             # metadata for a specific version
npmcli search "react hooks" --limit 10  # search packages
npmcli downloads react --period last-month  # download counts
npmcli deps express                     # list dependencies
npmcli --help                           # the whole command tree
```

Every command shares one output contract: `-o table|json|jsonl|csv|tsv|url|raw`,
`--fields` to pick columns, `--template` for a custom line, and `-n` to limit.
The default adapts to where output goes (a table on a terminal, JSONL in a
pipe), so the same command reads well by hand and parses cleanly downstream.

## Serve it

The same operations are available over HTTP and as an MCP tool set for agents:

```bash
npmcli serve --addr :7777    # GET /v1/info/<name>  returns NDJSON
npmcli mcp                   # speak MCP over stdio
```

## Use it as a resource-URI driver

`npmcli` registers a `npm` domain the way a program registers a database driver
with `database/sql`. A host enables it with one blank import:

```go
import _ "github.com/tamnd/npm-cli/npm"
```

Then [ant](https://github.com/tamnd/ant) (or any program that links the package)
dereferences `npm://` URIs without knowing anything about npm:

```bash
ant get npm://package/express           # fetch the record
ant url npm://package/react             # the live https URL
```

## Development

```
cmd/npm/   thin main: hands cli.NewApp to kit.Run
cli/                 assembles the kit App from the npm domain
npm/                the library: HTTP client, data models, and domain.go (the driver)
docs/                tago documentation site
```

```bash
make build      # ./bin/npmcli
make test       # go test ./...
make vet        # go vet ./...
```

## Releasing

Push a version tag and GitHub Actions runs GoReleaser:

```bash
git tag v0.1.0
git push --tags
```

## License

Apache-2.0. See [LICENSE](LICENSE).
