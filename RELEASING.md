# Releasing nostosx-cli

A release identifies the CLI, its embedded templates
and their pinned dependencies together. Generation uses the embedded templates.
Generated applications own their source; they are not upgraded by replacing the
generator binary.

Before publication:

1. Verify published servex v1.3.0, grpcx v1.2.0, agentx v1.2.1 and provider
   v1.2.0 dependencies and checksums through the public Go proxy, without sibling
   replacements or development proxies. Confirm repository ownership, the `github.com/nostosx/nostosx-cli` module
   path, the `nostosx` executable name and a private security-reporting channel.
2. Run `task check INTEGRATION=true`, `task lint` and `task vulncheck` from an
   independent source copy with `GOWORK=off`. Lint and vulnerability scans include
   the rendered templates. Verify all twelve layout/agent/transport variants.
3. Follow the bilingual README from a clean directory. Check generation, module
   resolution, added contexts/services/APIs and the optional agent runner's local
   provider tests. Do not infer live-provider acceptance from fixture results.
4. Review licenses, dependency checksums, template output, version metadata and
   changelog. Build with an explicit version, for example
   `go build -trimpath -ldflags "-s -w -X main.version=v1.2.0" -o bin/nostosx ./cmd/nostosx`.
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
  go install github.com/nostosx/nostosx-cli/cmd/nostosx@v1.2.0
nostosx --version
```

The CLI uses Go for installation and contract plugins; Buf is required when contracts
exist. No Python or application-local generator source is needed. Generated
Taskfiles default to CLI v1.2.0; check that this pin matches the approved release.
For source development, use a source-built binary through the NOSTOSX override.

Generated projects use published servex v1.3.0; only gRPC-enabled projects need grpcx v1.2.0. Optional
agentx and provider use v1.2.1 and v1.2.0. Update pinned versions and checksums before independently checking
consumers. Do not move any existing published tags.

Contract changes also require Buf 1.72.0: run `task api ACTION=lint`,
`task api ACTION=generate`, `task api` and `task api ACTION=breaking API_BASE=<baseline>` in
the application source. For the CLI, apply contract checks only to gRPC-enabled variants. Verify all twelve
layout/agent/transport combinations with their applicable quality gates.
Review versioned protobuf, per-context HTTP mappings, OpenAPI operation options
and pinned generators together with generated Go/gRPC/gateway and OpenAPI 2 output.
Confirm that the initial empty project needs no Buf or placeholder contracts.
Native HTTP services/APIs require no generation, Buf or gateway runtime.
Only Proto additions require explicit generation and go mod tidy before building;
the first gateway API also needs the pinned grpc-gateway runtime in the README. Verify real RPC authentication/unimplemented responses, route and symbol
conflicts, preservation of existing Proto source and partial-output rollback.
Exercise HTTP health and authenticated native gRPC in both process topologies and check
that application/domain packages remain free of transport dependencies. Do not
infer cross-host TLS deployment acceptance from plaintext loopback integration tests.

The current backend migration is unreleased. Before releasing these templates,
publish matching core, encoding/json, selector (with random), transport/client,
transport/httpx and transport/grpcx versions. Refresh HTTP-only, gRPC-only and combined go.mod/go.sum
templates from the public proxy, then rerun all twelve generated variants without
local replacements.
