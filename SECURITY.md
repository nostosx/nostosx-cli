# Security

The latest v1.x release receives security fixes. Report vulnerabilities privately
through [GitHub security advisories](https://github.com/nostosx/nostosx-cli/security/advisories/new).
Do not post credentials or exploitable details in public issues. Review the CLI,
generated source and application deployment boundaries separately.

## Generation boundary

`new`, `add` and `api scaffold` write source locally without executing generated code,
Go, Buf, shell commands or Git. Treat generated source and dependency changes as code
to review. Existing destinations and new-output collisions are rejected. `add api`
deliberately edits the target service Proto's imports/RPCs after conflict checks;
it never overwrites existing adapters or assembles HTTP routes. Unsupported inline
or complex HTTP mappings require manual edits. Additions use confined
root-relative writes, reject symlink parents and coordinate with contract generation
through `.nostosx.lock`. They are not a sandbox against an adversary controlling
the same user account. Never remove another active process's lock.
Normal failures roll back owned outputs; interrupted processes may leave a lock,
temporary Proto or partial new files. Inspect those before retrying.

`api scaffold` resolves bounded local Proto inputs and built-in well-known types;
it does not download imports or modify matching implementations. Newly created
methods return `Unimplemented` and require application-owned authorization.
`doctor` only reads project files and executes trusted tool version commands. It
does not install tools, download modules or create project files or locks.

## Contract build boundary

`api generate/lint/check/breaking` explicitly execute trusted Go/Buf tools and
consume application configuration. They are not a sandbox for untrusted projects,
tools or plugins. Plugins may download modules. The CLI rejects symlinked contract
inputs and outputs, shares the additions lock and stages output before publication.
Plugin failures and cancellation before publication preserve previous output.
Normal publication errors attempt rollback; process termination is not a crash
transaction and may leave a lock or recoverable staging directory.

## Generated runtime

The default HTTP and gRPC listeners require literal loopback IPs. They use plaintext
for local development. HTTP exposes only health checks. gRPC requires a printable,
whitespace-free bearer token of at least 32 ASCII characters. Authentication and
safe unary error handling do not replace application-level authorization.
Before external deployment, configure TLS, caller identity, authorization, resource
limits and the policy for every new HTTP/RPC method. Generated HTTP mapping does
not automatically install authentication for application routes.

Agent support is opt-in and does not expose a network API or business tools.
Only trusted operator configuration may select model endpoints; provider keys
are sent there. The model client refuses redirects. Each Ask has a fresh history,
bounded prompt size, deadline, turns, output tokens and concurrent capacity. Any
new tools require explicit authorization, input validation and side-effect policy.
Local fixtures do not establish real-provider or production security acceptance.
