# Contributing

This repository owns the nostosx CLI, embedded clean-project templates and
generated-project acceptance. It needs no sibling checkout to build or test.
Runnable library examples belong to agentx-examples and servex-examples.

Use Go 1.27.3, Task v3.53.1 and `GOWORK=off`. Do not add local replacements. Install tools
outside the module, with their binaries on PATH:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
go install github.com/bufbuild/buf/cmd/buf@v1.72.0
```

Tests and contract builds are written in Go. Run:

```sh
go mod download
task check INTEGRATION=true
task lint
task vulncheck
```

`GO`, `GOFMT`, `GOLANGCI_LINT` and `GOVULNCHECK` select executables. Generated
Taskfiles pin CLI v1.1.0; tests override `NOSTOSX` with the binary built from
this checkout. Use that override when developing the CLI or its templates.
Generated projects resolve pinned public modules. Tests use temporary files,
loopback listeners and local model fixtures, without Docker or paid model APIs.

Install [Task](https://taskfile.dev/docs/installation/) v3.53.1 outside the module,
matching CI, and put its binary on `PATH`:

```sh
go install github.com/go-task/task/v3/cmd/task@v3.53.1
```

Run `task --list` to discover tasks. Tool overrides accept environment variables or
Task arguments, for example `task check GO="/path with spaces/go"`.
Checks force `GOWORK=off`; `check`, `lint` and `vulncheck` do not apply source fixes.

The public entry points are `check`, `test`, `build`, `lint`, `vulncheck`, `fmt`,
`fix` and `tidy`; bare `task` selects `check`. Tests accept `RACE=true`,
`COVER=true` and `INTEGRATION=true`; coverage also enables race detection. Check enables race detection and forwards
the integration choice. Integration mode needs Buf and retains all twelve
pristine/extended generated-project variants. Lint and vulnerability scans include
the CLI and rendered templates by default; `SCOPE=cli|templates` selects one side.

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

Native HTTP is the default. Project --transport grpc or http,grpc explicitly enables
Proto/gRPC; add service reads the server construction shape instead of storing project metadata.
Native HTTP APIs create only an adapter and explicit mount helper, without Buf or
contract generation. Keep their symbol/route collision and no-overwrite checks.

`add api --transport grpc|gateway` is the deliberate exception for existing contracts: parse Proto syntax
and edit only the target v1 service's imports/RPCs. Use embedded templates for new
messages, bindings and adapters; do not generate stub tests. Check all external HTTP bindings, package symbols
and Handler methods/fields before writing. Reject unsupported mapping forms;
do not infer CRUD, CQRS use cases or application authorization. Preserve user
comments, shared-lock ownership and rollback on normal errors. Replace the target
Proto last, after all new outputs are written. Process termination is not a
transactional rollback guarantee.

Domain/application code must remain independent of protobuf, gRPC and telemetry
backends. Put consumer-owned ports beside use cases and inbound adapters in
`services/<name>/internal/port/rpc`. Compose services explicitly in `app.go`.
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

A CLI build does not check embedded `.tmpl` sources. Test all twelve layout/agent/transport
combinations before and after adding contexts, services and APIs. Assert behavior and
filesystem outcomes rather than incidental whitespace. Generated modules must
stay tidy and pass readonly builds, vet, modernization and shuffled race tests.
Keep process cleanup reliable even when assertions fail.

`api generate/lint/check/breaking` execute trusted Go/Buf tools under the shared
project lock. Do not embed tool source in generated applications. Preserve
cancellation, symlink rejection, staged publication and normal-error rollback.
Contract checks use Buf 1.72.0, protoc-gen-go v1.36.12, protoc-gen-go-grpc v1.6.0
and grpc-gateway/openapiv2 v2.29.0. Empty projects must not invoke these tools.
Adding or changing contracts requires `task api ACTION=generate`, `go mod tidy` and
`task api`. After adding the first gateway API, explicitly select
`github.com/grpc-ecosystem/grpc-gateway/v2@v2.29.0` before tidying, as shown in
the README. Test stale output cleanup, plugin-failure isolation, UTF-8/LF
output and shared `.nostosx.lock` ownership. Breaking checks require an explicit
Git baseline; an initial empty project is not such a baseline.

Explain observable changes and validation. Chinese commit messages are welcome,
for example `fix: 拒绝覆盖已有脚手架输出目录`. Do not commit, push, tag or publish
without explicit authorization. See [RELEASING.md](RELEASING.md).

The current HTTP backend and shared client migration is unreleased. Validate matching core and
contrib source revisions through throwaway module overrides, never repository
replace directives. Refresh actual dependency versions/checksums and rerun the
standalone public-proxy gates after those libraries are published.
