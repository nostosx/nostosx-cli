# Changelog

## v1.0.0 - 2026-10-26

Initial release of the `nostosx` CLI and its embedded Go application templates.

- Generate clean single-module monolith and service-monorepo skeletons, with
  optional agentx model configuration and bounded execution. Business contexts,
  database integration and application tools are added explicitly.
- Add domain/application packages with `add context`, and versioned Proto
  services, adapters and standalone process entry points with `add service`.
- Add unary RPCs with `add api`, optionally with HTTP bindings. Preserve existing
  Proto source and reject conflicting symbols, outputs and supported HTTP routes.
- Fill missing unary adapters from local Proto contracts with `api scaffold`,
  preserving compatible implementations and rejecting unsupported signatures.
- Build Go/gRPC/gateway and OpenAPI 2 output through `api generate`; validate
  contracts, generated output and compatibility with `api lint/check/breaking`.
  Contract tools use pinned versions and accept an empty initial contract tree.
- Diagnose Go, Buf and project configuration with read-only `doctor` text/JSON
  reports. Source scaffolding works offline without executing external tools.
- Confine writes to the project root, reject symlinks and output overwrites,
  coordinate additions and contract builds, and roll back owned outputs on normal
  failures. Process crashes are not covered by the rollback guarantee.
- Provide loopback HTTP/gRPC health, gRPC token authentication, safe transport
  errors, explicit service assembly and graceful shutdown in generated projects.
- Include bilingual documentation, Go tests, Make quality gates and CI. Validate
  initial and extended forms of all four layout/agent combinations, including
  race tests, real process shutdown and local model fixtures.
- Support shell completion and installed-version discovery from Go module build
  metadata; release binaries carry an explicit version.
