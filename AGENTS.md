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
  add service inherits configured transports, adds only their adapters and does not rewrite files.
  Native HTTP add api creates one adapter with explicit mounting; no Proto, generator or stub tests.
- add api edits only the target service Proto's imports/RPCs and creates messages,
  HTTP bindings and unimplemented adapters, without stub tests. Parse inputs, reject collisions,
  publish the Proto last and roll back owned files on normal errors. Do not mount
  HTTP handlers or invent CQRS use cases; authorization and assembly stay explicit.
- Domain/application packages remain free of transport dependencies. Use narrow
  consumer-owned ports; assemble typed servex/cqrs middleware only when needed.
- Preserve literal-loopback transport defaults, authenticated gRPC, safe errors,
  cancellation and bounded agent execution. Model endpoints are trusted startup
  configuration; business tools and their authorization are explicitly supplied.
- Contract commands accept no contracts without requiring Buf. For contracts,
  use pinned generators and shared .nostosx.lock; verify drift and failure isolation.
  Keep the Go build implementation in the CLI; do not emit tool source or Python.
- api scaffold fills missing unary Handler methods from local Proto descriptors;
  preserve matching implementations and reject incompatible signatures or streams.
  doctor reads project files and trusted tool versions, without installing tools,
  fetching modules or writing files. Its JSON stdout must stay machine-readable.
- Run task check INTEGRATION=true, lint and vulncheck.
  Verify pristine and extended forms of all twelve layout/agent/transport variants without sibling replaces.
  Cover each optional backend and all four together in both layouts after explicit tidy,
  with isolated loopback messaging fixtures in CLI acceptance only.
- Do not initialize Git, commit, push, tag or publish unless explicitly requested.

Contracts: api/proto/<context>/v1/; output: api/gen/go/; HTTP bindings:
api/http/routes/<context>.yaml; operation options: api/http/operations/<context>.yaml.
OpenAPI output uses Swagger 2. Inbound adapters: services/<name>/internal/port/{http,rpc}/.
Use action-and-object use-case filenames, matching tests and stable CQRS metadata
when implementing actual business behavior. Do not invent use cases for new contexts.
