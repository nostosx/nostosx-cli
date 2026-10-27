# nostosx-cli

The `nostosx` command creates a clean Go project and adds contexts and services
as the application grows. Choose a monolith or a service monorepo, with optional
agentx integration. Both layouts use one Go module.

[简体中文](README.zh-CN.md)

## CLI and Starter

| Project | Purpose |
| --- | --- |
| nostosx-cli | Generate an engineering skeleton, then add your own contexts and services. |
| nostosx-starter | Run and adapt a reference application with product, inventory and agent examples. |

This repository owns the executable and embedded templates. Generation does not
fetch the Starter. Generated applications own their source. Ordinary builds and
runtime need neither repository; contract development uses the CLI without copying
its tool source.

## Install

With Go **1.27.3** or later:

```sh
go install github.com/nostosx/nostosx-cli/cmd/nostosx@v1.1.0
nostosx --version
export NOSTOSX="$(command -v nostosx)"
```

Ensure Go's binary installation directory is on PATH. Alternatively, download the
archive for your OS and architecture from [v1.1.0](https://github.com/nostosx/nostosx-cli/releases/tag/v1.1.0),
verify it against `checksums.txt`, and place the executable on PATH.
Project builds and contract plugins still require Go; contracts also require Buf.

## Build from source

Use Go **1.27.3**, then run from this checkout in a POSIX shell:

```sh
export GOWORK=off
go mod download
mkdir -p bin
go build -o bin/nostosx ./cmd/nostosx
export NOSTOSX="$(pwd)/bin/nostosx"
"$NOSTOSX" --help
```

On Windows, use Git Bash and `bin/nostosx.exe`. The CLI supports `--version` and
shell completion; see `nostosx completion --help`. Release binaries embed the
version; other builds use Go's module version metadata when available and fall
back to `dev`. See [RELEASING.md](RELEASING.md) for the publication checks.

## Create a project

```sh
"$NOSTOSX" new workspace --module example.com/acme/workspace --output ../workspace
cd ../workspace
go mod download
export API_TOKEN="$(openssl rand -hex 32)"
go run ./cmd/server
```

In another terminal, `curl http://127.0.0.1:8080/readyz` checks readiness. Stop
with Ctrl+C. The server exposes HTTP health and authenticated gRPC health; there
are no business routes, database, migrations, sample contexts or architecture
test package. Follow the generated bilingual README for configuration and extension.

| Option | Behavior |
| --- | --- |
| `--module` | Required lowercase Go module path owned by the application |
| `--layout monolith` | Default; adds a runnable `cmd/server` host |
| `--layout microservices` | Repository skeleton; add services before running a process |
| `--with agentx` | Adds model configuration, a bounded runner and tests under `infrastructure/agent` |
| `--output` | New destination; its parent must exist |

`new`, `add` and `api scaffold` work offline without executing Go, Buf, shell commands, Git or
model requests. Contract build and check commands explicitly execute tools.
Existing paths and symlinks are rejected. No project metadata file is
created. Builds resolve pinned public dependencies with `GOWORK=off`, without
sibling checkouts or local replacements. Without `--with agentx`, no agentx source
or dependencies are generated. The agent option does not expose a chat endpoint
or select business tools; application assembly supplies those explicitly.

## Add a context and service

```sh
"$NOSTOSX" add context order --dir .
"$NOSTOSX" add service order --dir .
make generate NOSTOSX="$NOSTOSX"
go mod tidy
go run ./services/order/cmd/order -addr 127.0.0.1:8081 -grpc-addr 127.0.0.1:9091
```

`add context` declares domain and application packages, including command/query
packages, through `doc.go`. It does not invent use cases or persistence.
`add service` requires the same-named context and creates an empty versioned
Proto service, RPC adapter, registration function and standalone entry point.
HTTP mapping and operation files initially contain guidance comments only.
Define real RPCs and their HTTP bindings when implementing the service.

After adding a service, explicitly regenerate contracts and tidy dependencies.
Install Buf 1.72.0 first; pinned Go plugins are downloaded during
contract generation. An initial project has no contracts and builds without Buf.
See the generated README for explicit monolith registration and agent assembly.
Additions read the module from `go.mod`, not separate metadata. Context/service
additions create new files; neither changes existing assembly or dependencies.

## Build contracts

Source scaffolding and contract builds are separate operations:

```sh
"$NOSTOSX" api generate --dir .
"$NOSTOSX" api lint --dir .
"$NOSTOSX" api check --dir .
"$NOSTOSX" api breaking --dir . --against '.git#ref=main'
```

`api generate` builds Go, gRPC, HTTP gateway and OpenAPI 2 output. `api check`
compares paths and content without changing published output. `api lint` runs
Buf lint; `api breaking` requires an explicit Buf baseline. All four commands
share the additions lock. Plugins must succeed before output is replaced.

Generated Makefiles call CLI v1.1.0 through `go run`, without adding it to the
application module. To use a source-built binary, run
`make generate api-check NOSTOSX="$NOSTOSX"`.
`NOSTOSX_VERSION` selects another reviewed release. For a local binary, set
`NOSTOSX` to its absolute path. Build and test targets do not invoke the CLI.
Go, Buf, project configuration and plugin sources must be trusted; contract
commands are not a sandbox. Go plugins may access the network and module cache.

## Add an API

After creating a context and service, add an operation explicitly:

```sh
"$NOSTOSX" add api create-order --service order --method POST --path /v1/orders --dir .
"$NOSTOSX" add api get-order --service order --method GET --path '/v1/orders/{order_id}' --dir .
make generate NOSTOSX="$NOSTOSX"
go get github.com/grpc-ecosystem/grpc-gateway/v2@v2.29.0
go mod tidy
make api-check NOSTOSX="$NOSTOSX"
```

`create-order` becomes the `CreateOrder` RPC in `OrderService`. The command edits
only `api/proto/order/v1/order.proto` to add an import and RPC declaration. It
creates four files:

```text
api/proto/order/v1/create_order.proto                 request and response messages
api/http/routes/order_create_order.yaml                HTTP binding
services/order/internal/port/rpc/create_order.go        unimplemented Handler method
services/order/internal/port/rpc/create_order_test.go   stub regression test
```

The explicit `go get` pins the gateway runtime to the generator version. The
initial skeleton does not depend on it. For an existing application that already
selects a newer runtime, review compatibility before changing its version.

Requests/responses start empty, except explicit path parameters such as
`{order_id}`, which become `string` request fields. Choose their final types and
add payload fields in Proto, then regenerate. Methods are `GET`, `POST`, `PUT`,
`PATCH` or `DELETE`; POST/PUT/PATCH bind the body with `"*"`. Paths support literal
segments and simple `{field_name}` parameters, not wildcards, nested fields or
query strings. API names use lowercase hyphenated slugs; the target must be an
existing generated v1 service.

The adapter returns gRPC `Unimplemented` until implemented; replace the stub test
with application behavior tests as you implement it. The command does not create
CQRS use cases, mount HTTP handlers or change service registration. Define
authorization and explicitly mount the generated gateway handlers in `module.go`
before exposing HTTP. An OpenAPI entry documents a contract, not a running route.

Duplicate Proto symbols, Handler methods/fields, existing output files and
overlapping HTTP routes are rejected across the project. The check is conservative:
a parameter and a literal in the same segment can overlap. Existing inline
`google.api.http`, wildcard/custom mappings or unsupported path patterns require
manual editing; the command refuses to guess their interaction with YAML bindings.

For a gRPC-only operation, omit both HTTP flags:

```sh
"$NOSTOSX" add api ping --service order --dir .
make generate NOSTOSX="$NOSTOSX"
go mod tidy
```

This creates messages and the adapter/test, without an HTTP binding or a gateway
runtime requirement. `--method` and `--path` must be supplied together.

## Implement an existing contract

When you edit Proto first, add your RPCs and messages to the existing v1 service,
then fill missing transport methods:

```sh
"$NOSTOSX" api scaffold --service order --dir .
make generate NOSTOSX="$NOSTOSX"
go mod tidy
```

`api scaffold` resolves local imports under `api/proto` and built-in Protobuf
well-known types, without Go, Buf or network access. Vendor other imports into
that tree first. It adds one Go file per missing unary RPC in
`services/order/internal/port/rpc`, returning `Unimplemented`. Existing matching
methods are preserved, so repeating the command adds nothing. It creates no stub
tests, HTTP mappings, business use cases or service registration. Implement
authorization and the application call, then write behavior tests.

Streaming RPCs, incompatible existing signatures, build constraints, platform
files, conflicting fields and existing output paths are rejected before writing.
Signature checks recognize concrete generated message types and import aliases;
type aliases and dot imports require manual implementation. The operation uses
the shared project lock and rolls back owned files on normal write errors.

## Diagnose a project

```sh
nostosx doctor --dir .
nostosx doctor --dir . --json
```

Doctor checks installed Go (1.27.3 or newer), `go.mod`, the Buf v2 module layout
and the bounded local Proto tree. It requires Buf 1.72.0 once contracts exist;
an empty skeleton skips that tool. It only runs trusted `go version` and
`buf --version` commands, with a timeout and automatic Go toolchain downloads
disabled. It never installs tools, fetches dependencies or changes project files.
JSON goes to stdout; failed checks return exit code 1 with a summary on stderr.
Doctor is a setup check; use `make check` and `make api-check` to verify code,
contract semantics and generated output.

Both layouts retain the same boundaries:

```text
api/proto/<context>/v1/               contracts added with a service
api/gen/go/                          generated Go/gRPC/gateway code
api/http/                            HTTP bindings and OpenAPI configuration
application/<context>/command/       write use cases added by the application
application/<context>/query/         read use cases added by the application
domain/<context>/                    domain added by the application
services/<name>/module.go            explicit service assembly
services/<name>/internal/port/rpc/    inbound transport adapters
infrastructure/bootstrap/            transport configuration and lifecycle
infrastructure/rpc/                  authentication and safe RPC errors
infrastructure/agent/                optional model setup and bounded execution
```

Context/service names are lowercase Go package names. Project names are lowercase
slugs. Keywords and reserved filesystem names are rejected. Generation and `add`
refuse output overwrites and roll back their own outputs on normal failure.
`add api` replaces the target service Proto only after validating and writing its
new outputs; other source is preserved. Rollback does not cover process crashes.
Contract generation
and additions share a temporary `.nostosx.lock`; remove a stale lock only after
confirming its owner has stopped. `new` does not upgrade existing projects.

## Verify changes

```sh
make check
make lint
make vulncheck
make smoke
make templates-lint
make templates-vulncheck
```

`make check` covers the CLI and four layout/agent combinations, both pristine and
after adding contexts, services and APIs. `make smoke` starts generated hosts and added
services on temporary loopback ports, tests explicit monolith assembly and signal
shutdown, checks API authentication and unimplemented responses, and uses local
model fixtures for agentx. Template lint and vulnerability
gates also run before and after extension. No Docker, model credentials or sibling
source is needed. All tests are Go tests, including process smoke tests and
template checks. Contract integration tests execute the Go CLI with Buf 1.72.0.
No Python or generated tool source is required.

The CLI command tree is in `internal/cli`; confined writes and embedded templates
are in `internal/scaffold`; contract builds are in `internal/contract`.
Go tooling cannot directly check `.tmpl` files, so
rendered-project checks are required when changing templates. Starter changes do
not automatically change templates or applications already generated.

See [Contributing](CONTRIBUTING.md), [Security](SECURITY.md),
[Releasing](RELEASING.md), [Changelog](CHANGELOG.md) and [MIT license](LICENSE).
