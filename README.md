# soha

A command line for soha.

`soha` is a single pure-Go binary. It reads public soha data
over plain HTTPS, shapes it into clean records, and prints output that pipes
into the rest of your tools. No API key, nothing to run alongside it.

The same package is also a [resource-URI driver](#use-it-as-a-resource-uri-driver),
so a host program like [ant](https://github.com/tamnd/ant) can address
soha as `soha://` URIs.

## Install

```bash
go install github.com/tamnd/soha-cli/cmd/soha@latest
```

Or grab a prebuilt binary from the [releases](https://github.com/tamnd/soha-cli/releases), or run
the container image:

```bash
docker run --rm ghcr.io/tamnd/soha:latest --help
```

## Usage

```bash
soha page <path>                      # fetch one page as a record
soha page <path> -o json              # as JSON, ready for jq
soha page <path> --template '{{.Body}}'  # just the readable body text
soha links <path>                     # the pages it links to, one per line
soha --help                           # the whole command tree
```

Every command shares one output contract: `-o table|json|jsonl|csv|tsv|url|raw`,
`--fields` to pick columns, `--template` for a custom line, and `-n` to limit.
The default adapts to where output goes (a table on a terminal, JSONL in a
pipe), so the same command reads well by hand and parses cleanly downstream.

This is a fresh scaffold. It ships one example resource type, `page`, wired end
to end. Model the real soha records in `soha/` and declare their
operations in `soha/domain.go`; each one becomes a command, an HTTP
route, and an MCP tool at once.

## Serve it

The same operations are available over HTTP and as an MCP tool set for agents,
with no extra code:

```bash
soha serve --addr :7777    # GET /v1/page/<path>  returns NDJSON
soha mcp                   # speak MCP over stdio
```

## Use it as a resource-URI driver

`soha` registers a `soha` domain the way a program registers a
database driver with `database/sql`. A host enables it with one blank import:

```go
import _ "github.com/tamnd/soha-cli/soha"
```

Then [ant](https://github.com/tamnd/ant) (or any program that links the package)
dereferences `soha://` URIs without knowing anything about soha:

```bash
ant get soha://page/<path>   # fetch the record
ant cat soha://page/<path>   # just the body text
ant ls  soha://page/<path>   # the pages it links to, each addressable
ant url soha://page/<path>   # the live https URL
```

## Development

```
cmd/soha/   thin main: hands cli.NewApp to kit.Run
cli/                 assembles the kit App from the soha domain
soha/                the library: HTTP client, data models, and domain.go (the driver)
docs/                tago documentation site
```

```bash
make build      # ./bin/soha
make test       # go test ./...
make vet        # go vet ./...
```

## Releasing

Push a version tag and GitHub Actions runs GoReleaser, which builds the
archives, Linux packages, the multi-arch GHCR image, checksums, SBOMs, and a
cosign signature:

```bash
git tag v0.1.0
git push --tags
```

The Homebrew and Scoop steps self-disable until their tokens exist, so the first
release works with no extra secrets.

## License

Apache-2.0. See [LICENSE](LICENSE).
