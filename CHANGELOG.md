# Changelog

## Unreleased

- Support gRPC-only `add api` operations with paired optional HTTP flags.
- Add offline `api scaffold` to fill missing unary RPC adapters from existing
  Proto contracts while preserving compatible implementations.
- Add read-only `doctor` diagnostics with text/JSON output for project layout,
  Go and Buf versions; skip Buf for an empty skeleton.

- Generate clean single-module monolith and service-monorepo skeletons; keep
  product, inventory, persistence and assistant business examples in the separate
  nostosx-starter reference application.
- Add domain/application boundaries on demand with `add context`, and empty
  versioned Proto services, adapters and entry points with `add service`.
  These two commands leave existing assembly and contracts unchanged.
- Add `add api` for explicit v1 RPCs, request/response messages, HTTP bindings and
  unimplemented adapters with tests. Preserve existing source while inserting
  imports/RPCs into the target service Proto; reject symbol and route conflicts.
- Keep optional agentx model configuration and bounded execution independent of
  business tools and network endpoints.
- Add `api generate/lint/check/breaking` to own contract builds in the Go CLI.
  Generated Makefiles call a pinned CLI version, with a local binary override.
- Separate project, addition and contract command constructors; reject blank
  breaking-check baselines as usage errors before accessing the project.
- Provide explicit Buf, Go/gRPC/gateway and OpenAPI 2 generation; accept an empty
  initial contract tree without external generation tools or placeholder APIs.
- Remove default database dependencies, migrations and the archtest package.
- Preserve offline, deterministic generation, module-path validation, root-confined
  writes, cancellation, exclusive additions and rollback of owned outputs.
- Verify four generated combinations before and after extension, including race
  tests, authenticated transport health, local model fixtures and signal shutdown.
- Use Go tests for contract failure isolation, process smoke tests and generated
  template quality checks. Remove Python and application-local generator source.
- Document clean-project setup, explicit service/agent assembly and security
  boundaries in English and Simplified Chinese.
- Resolve the installed CLI version from Go module build metadata when no
  release version was injected by the binary build.

No release has been published.
