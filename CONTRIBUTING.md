# Contributing

This repository owns the nostosx CLI and embedded clean-project templates.
nostosx-starter separately maintains runnable business examples. Neither needs
its sibling checkout to build or test.

Use Go 1.27.3, Make and `GOWORK=off`. Do not add local replacements. Install tools
outside the module, with their binaries on PATH:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
go install github.com/bufbuild/buf/cmd/buf@v1.72.0
```

Tests, process checks and contract builds are written in Go. Run:

```sh
go mod download
make check
make lint
make vulncheck
make smoke
make templates-lint
make templates-vulncheck
```

`GO`, `GOFMT`, `GOLANGCI_LINT` and `GOVULNCHECK` select executables. Generated
Makefiles pin CLI v1.0.0; tests override `NOSTOSX` with the binary built from
this checkout. Do the same before that release is public.
Generated projects resolve pinned public modules. Tests use temporary files,
loopback listeners and local model fixtures, without Docker or paid model APIs.

## Implementation boundaries

`cmd/nostosx` owns process startup, signals and version resolution. In
`internal/cli`, keep root registration and error reporting in `root.go`, project
creation in `new.go`, additions in `add.go` and `add_api.go`, and contract build
commands in `contract.go`. Each command constructor owns its flags and I/O.
`internal/scaffold` owns embedded templates and offline source changes;
`internal/contract` owns external contract tools and output publication. Both
coordinate writes through `internal/projectfs`; neither imports the other.
`internal/doctor` owns read-only project and tool-version diagnostics.

Keep command flags and I/O local to each invocation. `new` and `add` must remain
independent of Go, shell commands, network access, Git and operator credentials.
Preserve path validation, root-confined writes, no-overwrite behavior, cancellation,
exclusive additions and rollback. Read additions' module path from bounded
`go.mod` input; do not persist duplicate metadata.

The default output contains no example business, database, migration, placeholder
service or archtest package. `add context` declares packages; `add service`
creates an empty Proto service and explicit assembly. It must not edit existing
contracts, dependencies or composition roots. New features need an observable,
necessary effect; avoid speculative configuration or abstractions.

`add api` is the deliberate exception for existing contracts: parse Proto syntax
and edit only the target v1 service's imports/RPCs. Use embedded templates for new
messages, bindings and adapters. Check all external HTTP bindings, package symbols
and Handler methods/fields before writing. Reject unsupported mapping forms;
do not infer CRUD, CQRS use cases or application authorization. Preserve user
comments, shared-lock ownership and rollback on normal errors. Replace the target
Proto last, after all new outputs are written. Process termination is not a
transactional rollback guarantee.

Domain/application code must remain independent of protobuf, gRPC and telemetry
backends. Put consumer-owned ports beside use cases and inbound adapters in
`services/<name>/internal/port/rpc`. Compose services explicitly in `module.go`.
When real use cases need CQRS, use servex/cqrs with stable operation metadata;
select contrib middleware explicitly at assembly. Do not generate fake use cases
solely to demonstrate those dependencies.

`api scaffold` compiles bounded local Proto inputs offline, adds missing unary
methods and preserves matching implementations. Reject unsupported signatures,
streaming methods and conditional source before writing. Do not create tests
that would become false once the user implements a scaffolded method.
`doctor` must not fetch modules, install tools or create lock files. Test failure
reports and JSON output separately from installed tool versions.

Agent integration supplies model configuration and a bounded runner, not an
assistant application. Keep tools and authorization application-owned. Endpoint
selection must come from trusted startup configuration, never request input.

## Tests and contracts

A CLI build does not check embedded `.tmpl` sources. Test all four layout/agent
combinations before and after adding contexts, services and APIs. Assert behavior and
filesystem outcomes rather than incidental whitespace. Generated modules must
stay tidy and pass readonly builds, vet, modernization and shuffled race tests.
Keep process cleanup reliable even when assertions fail.

`api generate/lint/check/breaking` execute trusted Go/Buf tools under the shared
project lock. Do not embed tool source in generated applications. Preserve
cancellation, symlink rejection, staged publication and normal-error rollback.
Contract checks use Buf 1.72.0, protoc-gen-go v1.36.12, protoc-gen-go-grpc v1.6.0
and grpc-gateway/openapiv2 v2.29.0. Empty projects must not invoke these tools.
Adding or changing contracts requires `make generate`, `go mod tidy` and
`make api-check`. After adding the first HTTP API, explicitly select
`github.com/grpc-ecosystem/grpc-gateway/v2@v2.29.0` before tidying, as shown in
the README. Test stale output cleanup, plugin-failure isolation, UTF-8/LF
output and shared `.nostosx.lock` ownership. Breaking checks require an explicit
Git baseline; an initial empty project is not such a baseline.

Explain observable changes and validation. Chinese commit messages are welcome,
for example `fix: 拒绝覆盖已有脚手架输出目录`. Do not commit, push, tag or publish
without explicit authorization. See [RELEASING.md](RELEASING.md).
