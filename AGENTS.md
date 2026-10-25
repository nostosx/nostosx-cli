# Working on nostosx-cli

Read README.md, CONTRIBUTING.md and native CI before changes. Use Go 1.27.3 and
GOWORK=off; load relevant Go CLI, layout and testing skills when available.

- This repository owns the CLI and embedded templates. nostosx-starter separately
  owns the runnable product/inventory/assistant examples; generation never fetches it.
- Default output is a clean skeleton: no business contexts, database, migrations,
  placeholder APIs, architecture-test package or project metadata file.
- Both layouts use one module. Monolith adds cmd/server; microservices starts
  without services. Add contexts/services only when requested by the user.
- new/add/api scaffold are offline. Never invoke Go, shell commands, Git, Buf or
  model APIs from source scaffolding. Other api commands execute trusted contract tools.
  Preserve no-overwrite behavior, root confinement, cancellation,
  exclusive additions and rollback of owned outputs.
- add context declares packages. add service adds an empty versioned Proto,
  adapters, module.go and a standalone entry point, without rewriting existing files.
- add api edits only the target service Proto's imports/RPCs and creates messages,
  HTTP bindings and unimplemented adapters/tests. Parse inputs, reject collisions,
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
- Run make check, lint, vulncheck, smoke, templates-lint and templates-vulncheck.
  Verify pristine and extended forms of all four variants without sibling replaces.
- Do not initialize Git, commit, push, tag or publish unless explicitly requested.

Contracts: api/proto/<context>/v1/; output: api/gen/go/; HTTP bindings:
api/http/routes/<context>.yaml; operation options: api/http/operations/<context>.yaml.
OpenAPI output uses Swagger 2. Inbound adapters: services/<name>/internal/port/rpc/.
Use action-and-object use-case filenames, matching tests and stable CQRS metadata
when implementing actual business behavior. Do not invent use cases for new contexts.
