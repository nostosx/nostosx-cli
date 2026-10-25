# Releasing nostosx-cli

A release identifies the CLI, its embedded templates
and their pinned dependencies together. nostosx-starter source snapshots are
versioned separately and are not bundled or downloaded during generation.
Generated applications own their source; they are not upgraded by replacing the
generator binary.

Before publication:

1. Verify published servex v1.1.0 and grpcx v1.0.0
   dependencies and checksums through the public Go proxy, without sibling
   replacements or development proxies. Confirm repository ownership, the `github.com/nostosx/nostosx-cli` module
   path, the `nostosx` executable name and a private security-reporting channel.
2. Run `make check`, `make lint`, `make vulncheck`, `make smoke`,
   `make templates-lint` and `make templates-vulncheck` from an independent source
   copy with `GOWORK=off`. Verify both layouts with and without agentx.
3. Follow the bilingual README from a clean directory. Check generation, module
   resolution, empty-project health, added contexts/services, explicit monolith
   assembly, signal shutdown and the optional agent runner's local provider fixture. Do not infer live-provider
   acceptance from fixture results.
4. Review licenses, dependency checksums, template output, version metadata and
   changelog. The initial version is `v1.0.0`. Build with an explicit
   version: `go build -trimpath -ldflags "-s -w -X main.version=v1.0.0" -o bin/nostosx ./cmd/nostosx`.
   Verify `nostosx --version`. A tagged `go install` uses Go's module build
   metadata. Source builds can report a Go-derived module version, including a
   pseudo-version; builds without version metadata report `dev`.
5. Obtain explicit authorization before initializing Git, committing, pushing,
   tagging or publishing. Require hosted CI to pass for the exact release commit.
6. Add a dated changelog section. Never move a published tag; fixes require a new
   version. Describe changes affecting newly generated applications separately
   from CLI behavior. Existing projects need an explicit migration.

The checked-in workflow validates changes. It does not create tags, publish
binaries or deploy generated services. Cross-building a binary is not proof of
its runtime behavior on the target platform.

After the repository is public and its exact commit passes hosted CI, publish
the approved version with an immutable tag. Include the changelog, source
revision, toolchain and validation limits in the release notes. Build standalone
`nostosx` binaries for Linux, macOS and Windows on amd64 and arm64 with
`CGO_ENABLED=0`, bundle each with LICENSE and the bilingual READMEs, and attach
SHA-256 checksums. Keep built archives outside the source tree. Check the native
binary's version, help, completion and offline generation before uploading.

Only after publication, verify installation in a fresh directory and module
cache through the public proxy and sumdb:

```sh
GOWORK=off GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org \
  go install github.com/nostosx/nostosx-cli/cmd/nostosx@v1.0.0
nostosx --version
```

The CLI uses Go for installation and contract plugins; Buf is required when contracts
exist. No Python or application-local generator source is needed. Generated
Makefiles default to CLI v1.0.0; check that this pin matches the approved release.
For source development, use a source-built binary through the NOSTOSX override.

The generated core uses published servex v1.1.0. grpcx and optional agentx/provider
use v1.0.0. Update pinned versions and checksums before independently checking
consumers. Do not move any existing published tags.

Contract changes also require Buf 1.72.0: run `make api-lint`,
`make generate`, `make api-check` and `make api-breaking API_BASE=<baseline>` in
the application source. For the CLI, apply these checks to each generated variant.
Review versioned protobuf, per-context HTTP mappings, OpenAPI operation options
and pinned generators together with generated Go/gRPC/gateway and OpenAPI 2 output.
Confirm that the initial empty project needs no Buf or placeholder contracts.
After add service or add api, require explicit generation and go mod tidy before
building; the first HTTP API also needs the pinned grpc-gateway runtime described
in the README. Verify real RPC authentication/unimplemented responses, route and symbol
conflicts, preservation of existing Proto source and partial-output rollback.
Exercise HTTP health and authenticated native gRPC in both process topologies and check
that application/domain packages remain free of transport dependencies. Do not
infer cross-host TLS deployment acceptance from plaintext loopback smoke tests.
