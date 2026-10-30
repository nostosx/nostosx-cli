# Working on nostosx-cli

Read README.md, CONTRIBUTING.md and native CI before changes. Use Go 1.27.3 and
GOWORK=off; load relevant Go CLI, layout and testing skills when available.

- This repository owns the CLI, embedded templates and generated-project acceptance.
  Runnable library examples belong to agentx-examples and servex-examples.
- Default output is a clean skeleton: no business contexts, database, migrations,
  placeholder APIs, architecture-test package or project metadata file.
- --with mysql,postgres,kafka,nats opts into constructors/configuration and pinned
  backend dependencies. Keep drivers in infrastructure; core and business ports stay
  independent. Config.Values carries the accepted Reader snapshot. Assemble resources
  before transport hooks with bounded startup/readiness and caller-owned cleanup.
  add infrastructure adds source/config.example.toml and only necessary go.mod requirements;
  preserve app/config/go.sum, comments and higher versions. Users run go mod tidy explicitly.
  Provision no tables, migrations, topics, streams, consumers or containers.
- Generated projects contain no AGENTS, CONTRIBUTING, README, CHANGELOG, SECURITY
  or LICENSE files; documentation and licensing belong to the application owner.
- Both layouts use one module. Monolith adds root app.go and cmd/server; microservices starts
  without services. Add contexts/services only when requested by the user.
  Commands parse configuration and call Run; assembly belongs in root or service app.go.
- Process configuration is TOML: config/config.toml for monoliths, services/<name>/config/config.toml
  for standalone services, created by add service. No root process config for empty microservices.
  Entry points pass their default path to config.Load; do not merge other processes' files.
- Generated applications load defaults/file/environment/explicit flags through
  contrib config/viper. Console text and optional JSON file logging use contrib logger/zap, injected
  through core Logger; construct and flush it in app.go without global loggers.
- new/add/api scaffold are offline. Never invoke Go, shell commands, Git, Buf or
  model APIs from source scaffolding. Other api commands execute trusted contract tools.
  Preserve no-overwrite behavior, root confinement, cancellation,
  exclusive additions and rollback of owned outputs.
- Default transport is native HTTP; --transport grpc or http,grpc explicitly enables Proto/gRPC.
  add service inherits configured transports and adds only their adapters.
- Services and business contexts are independent. add service requires no same-name
  context and generates private adapter, application handler aggregates and named HTTP/RPC Servers.
  add context declares application/{command,query,port} plus domain packages only.
- add api requires --context and --service; use cases and stable operation names belong
  to the context, contracts and transport servers belong to the service. Generate one
  Command/Query request and Handler/constructor per use case. Constructors return typed CQRS interfaces.
- services/<name>/internal/application owns Command/Query and ChainCommand/ChainQuery;
  these are explicit typed handler aggregates, never dynamic dispatchers or registries.
  Service app.go constructs dependencies and supplies CommandDeps/QueryDeps once; named Servers receive
  both handler aggregates and perform protocol conversion. Keep mapping code at the protocol boundary.
- HTTP/gateway defaults GET=query and other methods=command; --kind overrides intent and
  is required for pure gRPC. Share a aggregate field and chain across transports of the same
  service/context/name/kind, preserving business code and existing constructor dependencies.
- Keep root service mounting, authentication/authorization and gateway HTTP mounting explicit.
  Do not invent repositories, migrations, business port methods or concrete adapters.
  Preserve locking, collision checks, bounded input, cancellation and ordinary-error rollback
  of new outputs and replaced bytes/permissions. Publish the service Proto last.
- Preserve literal-loopback transport defaults, authenticated gRPC, safe errors,
  cancellation and bounded agent execution. Model endpoints are trusted startup
  configuration; business tools and their authorization are explicitly supplied.
- Contract commands accept no contracts without requiring Buf. For contracts,
  use pinned generators and shared .nostosx.lock; verify drift and failure isolation.
  Keep the Go build implementation in the CLI; do not emit tool source or Python.
- api scaffold fills missing unary named Server methods from local Proto descriptors;
  preserve matching implementations and reject incompatible signatures or streams.
  doctor reads project files and trusted tool versions, without installing tools,
  fetching modules or writing files. Its JSON stdout must stay machine-readable.
- Run task check INTEGRATION=true, lint and vulncheck.
  Verify pristine and extended forms of all twelve layout/agent/transport variants without sibling replaces.
  Cover each optional backend and all four together in both layouts after explicit tidy,
  with isolated loopback messaging fixtures in CLI acceptance only.
- Do not initialize Git, commit, push, tag or publish unless explicitly requested.

Contracts: api/proto/<service>/v1/; output: api/gen/go/; HTTP bindings:
api/http/routes/<service>.yaml; operation options: api/http/operations/<service>.yaml.
OpenAPI output uses Swagger 2. Inbound adapters: services/<name>/internal/port/{http,rpc}/.
Use action-and-object use-case filenames, matching tests and stable CQRS metadata
when implementing actual business behavior. Do not invent use cases for new contexts.
