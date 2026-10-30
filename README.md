# nostosx-cli

CLI v1.5.0 generates projects using published servex v1.4.0 and independent
contrib HTTP, configuration, logging and optional infrastructure modules.
Dependencies resolve without sibling source replacements.

CLI v1.5.0 adds --context, --kind and automatic typed CQRS wiring. Services and
business contexts are independent. Existing projects require an explicit source
migration; updating the generator does not rewrite application code.

The `nostosx` command creates a clean Go project and adds contexts and services
as the application grows. Choose a monolith or a service monorepo, with optional
agentx integration. Both layouts use one Go module.

[简体中文](README.zh-CN.md)

This repository owns the executable, embedded templates and generated-project
acceptance. Generated applications own their source. Ordinary builds and runtime
do not need the CLI; contract development uses it without copying its tool source.
Runnable library examples are maintained in
[agentx-examples](https://github.com/nostosx/agentx-examples) and
[servex-examples](https://github.com/nostosx/servex-examples).

## Install

With Go **1.27.3** or later:

```sh
go install github.com/nostosx/nostosx-cli/cmd/nostosx@v1.5.0
nostosx --version
export NOSTOSX="$(command -v nostosx)"
```

Ensure Go's binary installation directory is on PATH. Alternatively, download the
archive for your OS and architecture from [v1.5.0](https://github.com/nostosx/nostosx-cli/releases/tag/v1.5.0),
verify it against `checksums.txt`, and place the executable on PATH.
Project builds and contract plugins still require Go; contracts also require Buf.

CLI v1.5.0 includes transport selection, native HTTP scaffolds and the Task-based
application layout described below.

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
go run ./cmd/server
```

In another terminal, `curl http://127.0.0.1:8080/readyz` checks readiness. Stop
with Ctrl+C. The default server exposes only native HTTP health, without gRPC, Proto or Buf. There
are no business routes, database, migrations, sample contexts or architecture
test package. Project documentation and licensing are application-owned; the CLI does
not generate AGENTS.md, CONTRIBUTING.md, README.md, README.zh-CN.md, CHANGELOG.md,
SECURITY.md or LICENSE.

Generated applications use contrib Viper for configuration and contrib Zap for
readable console text and optional JSON file logging. `config.Load` composes sources and calls Viper;
`app.go` constructs the logger and injects the core Logger interface.
Configuration precedence is defaults < config file < environment < explicit flags.
Monoliths load `config/config.toml`; standalone services load
`services/<name>/config/config.toml`. Paths are relative to the working directory;
run from the repository root. A missing default file is allowed,
while `--config path` requires that file. `APP_ADDRESS`, `APP_GRPC_ADDRESS`,
`APP_LOG_LEVEL` and `APP_TOKEN` override enabled settings; `APP_TOKEN` takes
precedence over the existing `API_TOKEN`. `--log-level` defaults to `info`.

Console records render modules and fields as [Module] message [key:value],
with [操作:…], [耗时:…], [状态:…] and [错误:…] summaries. Newlines and tabs
in messages/fields are escaped so one event occupies one physical line.
An explicit component selects the module; otherwise it uses the caller package.
Console records use local timestamps in YYYY-MM-DD HH:mm:ss format, uppercase levels,
short caller locations and readable durations. File records use one JSON object
per line, the same second-precision local timestamps and numeric durations in seconds. Business fields
remain separate in both outputs. Set log_file in the process TOML file, APP_LOG_FILE or
--log-file to enable JSON file output; an explicit empty --log-file disables it.
The default is console only. Enabled files rotate at 100 MiB, retain at most five
backups for up to seven days, and compress backups with lumberjack v2.2.1.
Give each service process its own file path; close the logger after its users stop.

HTTP listens on `127.0.0.1:8080` by default; use `-addr` to change it.
Enabled gRPC listens on `127.0.0.1:9090`; use `-grpc-addr` to change it.
Listeners accept literal loopback IPs. Projects with gRPC or agentx require
`API_TOKEN` with at least 32 printable, non-whitespace ASCII characters.
Every gRPC call, including health probes, uses `authorization: Bearer <API_TOKEN>`.
The optional agent model constructor reads `AGENT_PROVIDER`, `AGENT_MODEL` and
`AGENT_API`; the application explicitly constructs and mounts its agent features.

| Option | Behavior |
| --- | --- |
| `--module` | Required lowercase Go module path owned by the application |
| `--layout monolith` | Default; adds `cmd/server` and root `app.go` assembly |
| `--layout microservices` | Repository skeleton; add services before running a process |
| `--transport http` | Default; native HTTP without Proto, gRPC or contract CI |
| `--transport grpc` | Only gRPC, with contract tools and authentication |
| `--transport http,grpc` | Both transports; select gateway per operation |
| `--with agentx` | Adds model configuration, a bounded runner and tests under `infrastructure/agent` |
| `--with mysql,postgres,kafka,nats` | Selects infrastructure constructors, configuration and pinned backend dependencies; accepts `postgresql` as an alias |
| `--output` | New destination; its parent must exist |

`new`, `add` and `api scaffold` work offline without executing Go, Buf, shell commands, Git or
model requests. Contract build and check commands explicitly execute tools.
Existing paths and symlinks are rejected. No project metadata file is
created. Released templates resolve pinned public dependencies with `GOWORK=off`, without
sibling checkouts or local replacements. Without `--with agentx`, no agentx source
or dependencies are generated. The agent option does not expose a chat endpoint
or select business tools; application assembly supplies those explicitly.

## Add a context and service

```sh
"$NOSTOSX" add context order --dir .
"$NOSTOSX" add service order --dir .
go run ./services/order/cmd/order -addr 127.0.0.1:8081
```

Contexts declare domain/application/{command,query,port} packages, without invented business or persistence.
Services can be created before any context; service names and business context names are independent.
Services inherit enabled transports from the server constructors, adding app.go, private adapters and a standalone entry.
Both process entries parse configuration, create a signal context and call `Run(ctx, cfg)`.
The monolith imports the root application package; standalone entries import their service.
Root `app.go` owns monolith assembly; `services/<name>/app.go` owns standalone assembly.
To host a service in the monolith, explicitly call its `Register` from root `app.go`
before `app.New`. Additions never rewrite an existing composition root.
Default HTTP services contain no Proto and need no generation. A gRPC-enabled project adds versioned contracts;
install Buf 1.72.0, run task api ACTION=generate and go mod tidy before building it.
Context/service additions read the current go.mod and server constructors without duplicate metadata, dependency edits or assembly rewrites.

A microservices skeleton has no process configuration until services are added.
Each `add service` creates `services/<name>/config/config.toml` and points that
service's entry at its own file. Files are independent: root and other services'
settings are not merged. Both layouts reuse `infrastructure/config` for loading
and validation.

For example, order can use `address = "127.0.0.1:8081"` and payment
`address = "127.0.0.1:8082"` in their respective TOML files. New services retain
the 8080/9090 defaults; assign distinct ports before running them together locally.
Give each process its own log_file when file logging is enabled. APP_ variables
are configured separately in each process environment. Override the path at deployment
with `go run ./services/order/cmd/order --config "/etc/order/config.toml"`.

## Optional infrastructure

```sh
"$NOSTOSX" new workspace --module example.com/acme/workspace --with mysql,nats
cd workspace
go mod tidy
APP_MYSQL__DSN='user:password@tcp(127.0.0.1:3306)/app?parseTime=true' go run ./cmd/server
```

The selected services must already be running. Multiple `--with` values may include
agentx and either or both databases. Both layouts use the same constructors:

| Feature | Generated package | Backend | Connection settings |
| --- | --- | --- | --- |
| mysql | infrastructure/database/mysql | contrib database/sql + go-sql-driver/mysql | mysql.dsn / APP_MYSQL__DSN |
| postgres | infrastructure/database/postgres | contrib database/sql + pgx/v5 stdlib | postgres.dsn / APP_POSTGRES__DSN |
| kafka | infrastructure/messaging/kafka | contrib broker/kafka + franz-go | kafka.brokers / APP_KAFKA__BROKERS |
| nats | infrastructure/messaging/nats | contrib broker/nats + nats.go JetStream | nats.url / APP_NATS__URL |

Monolith and newly added service configurations contain the selected infrastructure
sections. Viper loads nested `APP_` variables
with `__` between levels; Kafka brokers accept a comma-separated list. SQL DSNs
start empty; supply credentials through the deployment environment. The default
SQL pool has 20 open connections, 10 idle connections, a 30-minute lifetime and
a 5-minute idle timeout. All resource checks default to five seconds.

`Config.Values` retains the merged configuration snapshot. Each constructor accepts
the core `config.Reader`; `app.go` owns construction, injection and cleanup. SQL
resources expose native `*sql.DB`; messaging resources expose the contrib Broker.
Inject them explicitly into business constructors through consumer-owned ports.
App starts resources before listeners, includes them in readiness and closes them
in reverse order. NATS connects during construction and requires JetStream; SQL
and Kafka verify connectivity during startup. Tables, migrations, topics, streams,
consumers and containers remain application/deployment responsibilities.

For an existing project:

```sh
"$NOSTOSX" add infrastructure postgres --dir .
go mod tidy
```

This adds the constructor and `config.example.toml` beside it, and updates only the
required dependencies in go.mod. It preserves existing application code,
configuration and go.sum. Merge the example section into your configuration and
construct/register the resource before transport hooks in app.go. Run tidy explicitly
after either infrastructure command; generation never downloads modules. PostgreSQL also pins
the patched golang.org/x/text v0.42.0 dependency floor for its authentication path. Existing
higher dependency versions and go.mod comments are retained. Repeated additions,
file collisions and symlinks are rejected; normal failures roll back new outputs.

Subsequently added services inherit available infrastructure constructors. Older
generated configurations need a `Values config.Reader` field with `json:"-"` and
`result.Values = loaded.Snapshot()` in Load first; the CLI reports this prerequisite
before writing service files. Applications adopting the new configuration template
must also pass defaultPath between name and args in config.Load calls.
Service additions preserve existing assembly; add api updates the target service CQRS wiring.

## Add a native HTTP operation

```sh
"$NOSTOSX" add api get-order --service order --context order --method GET --path '/v1/orders/{order_id}'
```

The default `--transport http` generates the application use case, aggregate wiring and protocol adapter together.
`--context` is required independently of `--service`. For example:

```sh
"$NOSTOSX" add service studio --dir .
"$NOSTOSX" add context creation --dir .
"$NOSTOSX" add api create-profile --service studio --context creation --method POST --path /profiles
"$NOSTOSX" add api get-profile --service studio --context creation --method GET --path '/profiles/{id}'
```

```text
application/creation/
  command/create_profile.go       CreateProfileCommand, Result, Handler and constructor
  query/get_profile.go            GetProfileQuery, Result, Handler and constructor
  port/                           consumer-owned outbound interfaces
services/studio/
  app.go                          constructs dependencies and injects handler aggregates
  internal/application/
    command.go                    typed Command, dependencies and ChainCommand
    query.go                      typed Query, dependencies and ChainQuery
  internal/port/http/
    server.go                     StudioServer, constructor and route mounting
    create_profile.go             protocol conversion and commands.CreateProfile.Handle
    get_profile.go                protocol conversion and queries.GetProfile.Handle
    mapper.go                     application/result conversion boundary
  internal/adapter/               service-private implementations
```

Command and Query fields are typed handlers, without dynamic registration or reflection. Business handlers belong to the
selected context, and operation metadata is `creation.create_profile`, independent of service naming.
Each handler and chain is constructed once per service assembly and shared by its HTTP/gRPC adapters.
`app.go` supplies handlers through CommandDeps/QueryDeps; middleware belongs in the service application handler aggregates.
New contexts and services contain no fake business or repositories. Add outbound ports and constructor
parameters when needed; adapters implement those capabilities and stay private to the owning service.

Choose `--kind command|query`; HTTP/gateway defaults GET=query and other methods=command, with explicit override.
Pure gRPC requires `--kind`. The application handler initially returns Unimplemented and honors cancellation;
implement business behavior there and configure authentication/authorization in service assembly.
Mount the service in root app.go explicitly when using a monolith. Gateway HTTP mounting stays explicit.

Adding a second transport for the same service/context/name/kind preserves business implementation and shares
the existing aggregate field and chain. Automatic mapping supports generated string inputs and an empty result;
custom types/results or constructors requiring new dependencies need explicit application wiring.
Native HTTP needs no Proto, OpenAPI, stub tests or contract tools. Generation remains offline.
Conflicts in contracts, application types, handler aggregates, imports and assembly are rejected before publication.
Normal errors restore original bytes/permissions and remove owned outputs; publish the service Proto last.
Custom aggregate/constructor/registration shapes and dynamic routes require manual wiring.

## Migrate an existing generated service

This layout change does not rewrite previously generated applications automatically.
Keep contracts under the deployable service name and move application ownership explicitly:

1. Pass --context on each add api invocation; it no longer follows --service.
2. Add service-private Command/Query, supply existing handlers through CommandDeps/QueryDeps and move CQRS chains into those handler aggregates.
3. Replace direct Handler dependency fields with a named service Server receiving both handler aggregates; keep protocol conversion in mapper.go.
4. Give application requests Command/Query names and expose explicit constructors returning typed Servex CQRS interfaces.
5. Keep existing business constructor dependencies, middleware, authorization and lifecycle ownership intact; do not regenerate over application files.

## Build optional Proto contracts

Select --transport grpc or --transport http,grpc when creating the project.
Changing transports in an existing application requires reviewing assembly and dependencies; the CLI does not rewrite them automatically.



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

Generated Taskfiles call CLI v1.5.0 through `go run`, without adding it to the
application module. To use a source-built binary, run
`task api ACTION=generate NOSTOSX="$NOSTOSX"`, then `task api NOSTOSX="$NOSTOSX"`.
`NOSTOSX_VERSION` selects another reviewed release. For a local binary, set
`NOSTOSX` to its absolute path. Build and test targets do not invoke the CLI.
Go, Buf, project configuration and plugin sources must be trusted; contract
commands are not a sandbox. Go plugins may access the network and module cache.

## Add a Proto API

After creating a context and service, add an operation explicitly:

```sh
"$NOSTOSX" add api create-order --transport gateway --service order --context order --method POST --path /v1/orders --dir .
"$NOSTOSX" add api get-order --transport gateway --service order --context order --method GET --path '/v1/orders/{order_id}' --dir .
task api ACTION=generate NOSTOSX="$NOSTOSX"
go get github.com/grpc-ecosystem/grpc-gateway/v2@v2.29.0
go mod tidy
task api NOSTOSX="$NOSTOSX"
```

`create-order` becomes the `CreateOrder` RPC in `OrderService`. The command edits
only `api/proto/order/v1/order.proto` to add an import and RPC declaration. It
creates three files:

```text
api/proto/order/v1/create_order.proto                 request and response messages
api/http/routes/order_create_order.yaml                HTTP binding
services/order/internal/port/rpc/create_order.go        Server method calling the application command
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

The RPC adapter converts protocol types, calls its injected CQRS handler and maps errors through infrastructure/rpc.
The application use case initially returns Unimplemented; add behavior tests when implementing it, without generated stub tests.
The command creates the application handler, aggregate dependency, named service Server and constructor/registration wiring, publishing the service Proto last.
Configure authentication and authorization in assembly. Mount gateway HTTP handlers explicitly; an OpenAPI entry does not create a running route.

Duplicate Proto symbols, Server methods/fields, existing output files and
overlapping HTTP routes are rejected across the project. The check is conservative:
a parameter and a literal in the same segment can overlap. Existing inline
`google.api.http`, wildcard/custom mappings or unsupported path patterns require
manual editing; the command refuses to guess their interaction with YAML bindings.

For a gRPC-only operation, select --transport grpc and omit both HTTP flags:

```sh
"$NOSTOSX" add api ping --transport grpc --kind query --service order --context order --dir .
task api ACTION=generate NOSTOSX="$NOSTOSX"
go mod tidy
```

This creates messages and the adapter/test, without an HTTP binding or a gateway
runtime requirement. `--method` and `--path` must be supplied together.

## Implement an existing contract

When you edit Proto first, add your RPCs and messages to the existing v1 service,
then fill missing transport methods:

```sh
"$NOSTOSX" api scaffold --service order --dir .
task api ACTION=generate NOSTOSX="$NOSTOSX"
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
Doctor is a setup check; use `task check` to verify code. gRPC-enabled projects
additionally use `task api` for contract semantics and generated output.

Both layouts retain the same boundaries:

```text
app.go                              monolith Run and explicit service assembly
config/config.toml                  monolith process configuration (TOML)
services/<name>/config/config.toml   standalone service configuration (TOML)
api/proto/<context>/v1/               contracts added with a service
api/gen/go/                          generated Go/gRPC/gateway code
api/http/                            HTTP bindings and OpenAPI configuration
application/<context>/command/       write use cases added by the application
application/<context>/query/         read use cases added by the application
domain/<context>/                    domain added by the application
services/<name>/app.go               service Run and explicit handler mounting
services/<name>/internal/port/rpc/    inbound transport adapters
infrastructure/config/              Viper sources, typed settings and validation
infrastructure/logging/             Zap console/file encoders, rotation and cleanup
infrastructure/server/              HTTP/gRPC construction without startup callbacks
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
task check INTEGRATION=true
task lint
task vulncheck
```

The eight public tasks are `check`, `test`, `build`, `lint`, `vulncheck`, `fmt`,
`fix` and `tidy`. Bare `task` runs `check`. `task test RACE=true` enables race
detection; `COVER=true` enables race detection, writes `coverage.out` and prints coverage. `check`
reuses `test` with race detection and preserves the caller's `INTEGRATION` choice.
The default test/check run needs no Buf. Set `INTEGRATION=true` to include real
Go/Buf contract checks and all twelve layout/agent/transport combinations, both
pristine and after adding contexts, services and APIs. CI selects this mode.

`lint` and `vulncheck` cover both the CLI and all rendered-project variants,
before and after extension. `SCOPE=cli` or `SCOPE=templates` selects one side
without creating additional task names. No Docker, model credentials or sibling
source is needed. All tests and template checks are Go tests. Contract integration tests execute the Go CLI with Buf 1.72.0.
No Python or generated tool source is required.

The CLI command tree is in `internal/cli`; confined writes and embedded templates
are in `internal/scaffold`; contract builds are in `internal/contract`.
Go tooling cannot directly check `.tmpl` files, so
rendered-project checks are required when changing templates. Updating the CLI
does not automatically change applications already generated.

See [Contributing](CONTRIBUTING.md), [Security](SECURITY.md),
[Releasing](RELEASING.md), [Changelog](CHANGELOG.md) and [MIT license](LICENSE).
