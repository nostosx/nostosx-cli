# nostosx-cli

CLI v1.4.0 使用公开发布的 servex v1.4.0 与独立的 contrib HTTP、配置、日志及
可选基础设施模块生成工程，无需同级源码或本地 replace。

`nostosx` 命令创建干净的 Go 工程，随后按需添加业务上下文和服务。
支持单体与多服务布局，以及可选的 agentx 接入；两种布局均使用一个 Go 模块。

[English](README.md)

本仓库维护命令、内嵌模板和生成工程的验收。生成的应用自行维护源码，
普通构建和运行不依赖 CLI；契约开发调用 CLI，不复制工具源码。
库的可运行示例由 [agentx-examples](https://github.com/nostosx/agentx-examples)
和 [servex-examples](https://github.com/nostosx/servex-examples) 维护。

## 安装

使用 Go **1.27.3** 或更新版本：

```sh
go install github.com/nostosx/nostosx-cli/cmd/nostosx@v1.4.0
nostosx --version
export NOSTOSX="$(command -v nostosx)"
```

请将 Go 的二进制安装目录加入 PATH。也可以从
[v1.4.0](https://github.com/nostosx/nostosx-cli/releases/tag/v1.4.0)
下载对应系统和架构的压缩包，使用 `checksums.txt` 校验后，将可执行文件放入 PATH。
工程构建和契约插件仍需要 Go；存在契约时还需要 Buf。

CLI v1.4.0 包含本文的传输选择、原生 HTTP 骨架与 Task 工程结构。

## 从源码构建

使用 Go **1.27.3**，在本仓库根目录通过 POSIX shell 执行：

```sh
export GOWORK=off
go mod download
mkdir -p bin
go build -o bin/nostosx ./cmd/nostosx
export NOSTOSX="$(pwd)/bin/nostosx"
"$NOSTOSX" --help
```

Windows 使用 Git Bash，并将可执行文件名改为 `bin/nostosx.exe`。
命令支持 `--version`；补全用法见 `nostosx completion --help`。发布二进制注入版本号，
其他构建优先读取 Go 模块版本信息；没有可用版本信息时显示 `dev`。
发布前检查见 [RELEASING.md](RELEASING.md)。

## 创建工程

```sh
"$NOSTOSX" new workspace --module example.com/acme/workspace --output ../workspace
cd ../workspace
go mod download
go run ./cmd/server
```

在另一终端用 `curl http://127.0.0.1:8080/readyz` 检查就绪状态，Ctrl+C 停止。
默认宿主仅提供原生 HTTP 健康检查，不包含 gRPC、Proto、Buf、业务路由、数据库、
迁移、示例上下文或架构测试包。项目文档和许可证由应用自行维护，CLI 不生成
AGENTS.md、CONTRIBUTING.md、README.md、README.zh-CN.md、CHANGELOG.md、SECURITY.md 或 LICENSE。

生成项目默认使用 contrib Viper 读取配置、contrib Zap 输出控制台文本和可选 JSON 文件日志。
`config.Load` 只负责装配配置源并调用 Viper，`app.go` 构造 Logger 并通过核心接口注入。
配置优先级为：默认值 < 配置文件 < 环境变量 < 显式命令行参数。
单体默认读取 `config/config.toml`，独立服务默认读取
`services/<name>/config/config.toml`；路径相对于启动时的工作目录，建议从仓库根目录启动。
默认文件缺失时可用默认值和环境变量启动，
显式使用 `--config path` 时要求文件存在。`APP_ADDRESS`、`APP_GRPC_ADDRESS`、
`APP_LOG_LEVEL`、`APP_TOKEN` 覆盖对应配置；`APP_TOKEN` 优先于原有的 `API_TOKEN`。
`--log-level` 默认是 `info`。

控制台以 [模块] 消息 [字段:值] 展示事件，常用字段显示为 [操作:…]、
[耗时:…]、[状态:…]、[错误:…]。消息及字段中的换行、制表符转义为文本，
每个事件只占一行；模块优先取 component，缺省使用调用者包名。
控制台使用本地时间（格式为 2026-10-29 17:13:50）、大写级别、简短调用位置和易读耗时；
文件每行写一个 JSON 对象，使用相同的秒级本地时间及以秒为单位的数值耗时。
两种输出都保留独立业务字段。通过进程 TOML 配置的 log_file、APP_LOG_FILE
或 --log-file 开启文件输出，显式传入空的 --log-file 可关闭文件输出。
默认只输出控制台；启用文件后，通过 lumberjack v2.2.1 按 100 MiB 轮转，
最多保留五个备份、七天并压缩。每个服务进程使用独立文件路径，所有使用者停止后关闭 Logger。

HTTP 默认监听 `127.0.0.1:8080`，通过 `-addr` 修改；启用 gRPC 时默认监听
`127.0.0.1:9090`，通过 `-grpc-addr` 修改。监听地址使用字面量 loopback IP。
启用 gRPC 或 agentx 的工程需要 `API_TOKEN`，至少包含 32 个不含空白的可打印 ASCII 字符。
所有 gRPC 调用（包括健康检查）使用 `authorization: Bearer <API_TOKEN>`。
可选的 Agent 模型构造函数读取 `AGENT_PROVIDER`、`AGENT_MODEL` 和 `AGENT_API`，
由应用显式构造并挂载 Agent 能力。

| 选项 | 行为 |
| --- | --- |
| `--module` | 必填，应用自己的小写 Go 模块路径 |
| `--layout monolith` | 默认，生成 `cmd/server` 入口及根目录 `app.go` 装配 |
| `--layout microservices` | 生成仓库骨架，添加服务后再启动进程 |
| `--transport http` | 默认，仅 HTTP，无 Proto、gRPC 或契约 CI |
| `--transport grpc` | 仅 gRPC，启用契约工具和认证 |
| `--transport http,grpc` | 显式启用两种传输；gateway 按 API 选择 |
| `--with agentx` | 在 `infrastructure/agent` 添加模型配置、有界执行器及测试 |
| `--with mysql,postgres,kafka,nats` | 按需添加基础设施构造、配置和固定版本依赖；`postgresql` 是 `postgres` 的别名 |
| `--output` | 尚不存在的目标路径，父目录必须已存在 |

`new`、`add` 和 `api scaffold` 离线完成，不执行 Go、Buf、Shell、Git 或模型请求；
契约构建与检查命令会显式执行工具。新增源码时拒绝已有路径和符号链接，
不生成项目元数据文件。已发布模板通过公开版本解析固定依赖，使用 `GOWORK=off`，
无需同级源码或本地 replace。未启用 agentx 时不生成其源码与依赖。
启用 agentx 也不会自动开放聊天接口或业务工具，这些由应用显式装配。

## 添加上下文和服务

```sh
"$NOSTOSX" add context order --dir .
"$NOSTOSX" add service order --dir .
go run ./services/order/cmd/order -addr 127.0.0.1:8081
```

上下文只声明领域和应用包，不预设用例或持久化。服务从现有服务器构造代码继承传输配置，
创建 app.go、私有适配器和独立入口；默认 HTTP 服务不创建 Proto，也不需要生成步骤。
两种入口都只解析配置、建立信号上下文并调用 `Run(ctx, cfg)`：单体入口调用根应用包，
独立入口调用服务包。根目录 `app.go` 负责单体装配，`services/<name>/app.go` 负责独立服务装配。
在单体中启用服务时，在根目录 `app.go` 的 `app.New` 之前显式调用服务的 `Register`；
新增操作不会自动重写已有装配入口。
选择了 gRPC 的工程会生成版本化服务契约，须安装 Buf 1.72.0，执行 task api ACTION=generate 和 go mod tidy。
新增上下文和服务时读取当前 go.mod 和服务器构造代码，不维护重复元数据，不改动已有装配或依赖。

微服务骨架在添加服务前不生成进程配置。新增 order、payment 服务后，配置随服务保存：

```text
services/
├── order/
│   ├── app.go
│   ├── cmd/order/main.go
│   └── config/config.toml
└── payment/
    ├── app.go
    ├── cmd/payment/main.go
    └── config/config.toml
```

每个文件只包含对应进程的设置，不自动合并单体或其他服务的配置。例如：

```toml
# services/order/config/config.toml
address = "127.0.0.1:8081"
log_level = "info"
log_file = ""

# 仅启用 PostgreSQL 时生成此段；凭据可通过 APP_POSTGRES__DSN 提供。
[postgres]
dsn = ""
max_open_conns = 20
max_idle_conns = 10
conn_max_lifetime = "30m"
conn_max_idle_time = "5m"
timeout = "5s"
```

新服务沿用 8080/9090 的监听默认值；本机同时启动多个进程时，分别修改各自配置中的端口，
或传入 --addr/--grpc-addr。启用文件日志时，各服务设置独立的 log_file。
环境变量使用相同 APP_ 约定，但在各进程的环境中分别设置。
部署时可用 `go run ./services/order/cmd/order --config "/etc/order/config.toml"` 指定挂载配置。

## 可选基础设施

```sh
"$NOSTOSX" new workspace --module example.com/acme/workspace --with mysql,nats
cd workspace
go mod tidy
APP_MYSQL__DSN='user:password@tcp(127.0.0.1:3306)/app?parseTime=true' go run ./cmd/server
```

先准备好所选的基础设施服务。`--with` 可以组合 agentx、数据库和消息系统，
两种布局共用相同的构造代码，也可以同时选择两种数据库：

| 能力 | 生成目录 | 实现 | 连接配置 |
| --- | --- | --- | --- |
| mysql | infrastructure/database/mysql | contrib database/sql + go-sql-driver/mysql | mysql.dsn / APP_MYSQL__DSN |
| postgres | infrastructure/database/postgres | contrib database/sql + pgx/v5 stdlib | postgres.dsn / APP_POSTGRES__DSN |
| kafka | infrastructure/messaging/kafka | contrib broker/kafka + franz-go | kafka.brokers / APP_KAFKA__BROKERS |
| nats | infrastructure/messaging/nats | contrib broker/nats + nats.go JetStream | nats.url / APP_NATS__URL |

单体配置和新增服务的独立配置会包含当前已选择的基础设施配置段。Viper 用双下划线分隔嵌套环境变量，
Kafka brokers 支持逗号分隔列表。SQL DSN 默认为空，通过部署环境提供凭据。
默认 SQL 连接池：最大打开 20、最大空闲 10、连接寿命 30 分钟、空闲超时 5 分钟；
所有资源的连通性检查默认超时 5 秒。

`Config.Values` 保存已合并的配置快照，构造函数接收核心 `config.Reader` 接口。
`app.go` 集中构造和管理资源：SQL 暴露原生 `*sql.DB`，消息系统暴露 contrib Broker，
按业务通过消费方定义的窄接口显式注入。App 在监听端口前检查资源，将它们纳入就绪检查，
关闭时按相反顺序释放。NATS 在构造时连接且需要启用 JetStream，SQL 和 Kafka 在启动时检查连通性。
表、迁移、topic、stream、业务消费者及容器由应用和部署管理。

给已有项目添加能力：

```sh
"$NOSTOSX" add infrastructure postgres --dir .
go mod tidy
```

命令新增构造源码和同目录的 `config.example.toml`，并定点更新 go.mod 必要依赖，
保留现有配置、app.go 和 go.sum。将配置片段合并到主配置，在 app.go 中显式构造资源，
放在传输 hooks 前面。新建或后续添加基础设施后都需显式执行 tidy，生成命令本身不下载依赖。
PostgreSQL 同时约束 golang.org/x/text 至少为已修复的 v0.42.0，覆盖认证路径。
现有更高版本依赖和 go.mod 注释会保留；重复添加、文件冲突和符号链接会被拒绝，
普通错误会回滚本次新增文件。

之后新增的服务会继承已有基础设施构造。旧版生成配置需先补充带 `json:"-"` 的
`Values config.Reader` 字段，并在 Load 中设置 `result.Values = loaded.Snapshot()`；
CLI 会在写入服务文件前提示这项前置条件。配置加载模板的 Load 新增 defaultPath 参数；
已有工程升级模板时需同步修改入口调用。已有装配入口始终由应用显式修改。

## 添加原生 HTTP 接口

```sh
"$NOSTOSX" add api get-order --service order --method GET --path '/v1/orders/{order_id}'
```

默认 --transport http，仅新增 services/order/internal/port/http/get_order.go：请求/响应、
Handler.GetOrder 和 RegisterGetOrder。路径参数初始为 string；按业务选择字段及类型。
实现授权、参数转换和应用调用，并在 HTTP 包的 Register 中显式调用 RegisterGetOrder(server, handler)。
不生成 Proto、OpenAPI、占位测试或业务用例，不执行契约生成，也不自动挂载路由。
重复符号、已有输出及可识别的静态 HTTP 路由冲突在写入前拒绝；动态路由须手工维护。

## 构建可选 Proto 契约

先在新建工程时显式选择 --transport grpc 或 --transport http,grpc。
现有应用的协议变更需自行审查装配与依赖，CLI 不自动改写工程。



源码脚手架与契约构建分开执行：

```sh
"$NOSTOSX" api generate --dir .
"$NOSTOSX" api lint --dir .
"$NOSTOSX" api check --dir .
"$NOSTOSX" api breaking --dir . --against '.git#ref=main'
```

`api generate` 构建 Go、gRPC、HTTP gateway 与 OpenAPI 2 产物。
`api check` 只比较路径及内容，不修改已有产物；`api lint` 执行 Buf lint；
`api breaking` 要求显式指定 Buf 基线。四个命令与新增操作共用锁，
所有插件成功后才替换产物。

生成项目的 Taskfile.yml 默认通过 `go run` 调用 CLI v1.4.0，不将其加入应用模块。
使用源码构建的二进制时，先执行 `task api ACTION=generate NOSTOSX="$NOSTOSX"`，再执行 `task api NOSTOSX="$NOSTOSX"`。
`NOSTOSX_VERSION` 可选择其他经审查的版本；`NOSTOSX` 用于指定本地二进制的绝对路径。
普通构建和测试不调用 CLI。契约命令会执行 Go 插件，可能访问网络及模块缓存；
Go、Buf、工程配置和插件源码必须可信，这些命令不提供沙箱隔离。

## 添加 Proto API

已有上下文和服务后，显式添加操作：

```sh
"$NOSTOSX" add api create-order --transport gateway --service order --method POST --path /v1/orders --dir .
"$NOSTOSX" add api get-order --transport gateway --service order --method GET --path '/v1/orders/{order_id}' --dir .
task api ACTION=generate NOSTOSX="$NOSTOSX"
go get github.com/grpc-ecosystem/grpc-gateway/v2@v2.29.0
go mod tidy
task api NOSTOSX="$NOSTOSX"
```

`create-order` 对应 `OrderService.CreateOrder`。命令只在已有
`api/proto/order/v1/order.proto` 中插入 import 和 RPC 声明，另创建三个文件：

```text
api/proto/order/v1/create_order.proto                 请求与响应消息
api/http/routes/order_create_order.yaml                HTTP 映射
services/order/internal/port/rpc/create_order.go        尚未实现的 Handler 方法
```

显式 `go get` 将 gateway 运行库固定到生成插件版本，初始骨架不依赖它。
已有工程若已选择更新的运行库，应先确认兼容性，再决定是否调整版本。

请求与响应默认没有业务字段；路径中显式声明的 `{order_id}` 会生成对应的
`string` 请求字段。按业务确定字段类型和请求体后重新生成。
支持 `GET`、`POST`、`PUT`、`PATCH`、`DELETE`；POST/PUT/PATCH 使用 `body: "*"`。
路径支持字面量和简单 `{field_name}` 参数，不支持通配符、嵌套字段或查询字符串。
API 名使用小写连字符 slug，目标必须是已存在的生成式 v1 服务。

RPC 入口先返回 gRPC `Unimplemented`；实现真实行为时编写对应测试，不生成占位测试。
命令不推测 CQRS 用例、不修改服务注册，也不自动挂载 HTTP。
对外开放前，在 `app.go` 中明确认证与授权，再显式挂载生成的 gateway 处理器。
OpenAPI 中存在契约并不表示 HTTP 路由已经运行。

生成前会检查工程内的 Proto 符号、Handler 方法及字段、已有输出和 HTTP 路由重叠。
路由判断采用保守规则，同一段的路径参数与字面量也可能重叠。
已有内联 `google.api.http`、通配选择器、自定义方法或不支持的路径模式时会拒绝操作，
应手工维护契约与映射，避免不完整的冲突判断。

仅提供 gRPC 的操作，显式选择 --transport grpc 并省略两个 HTTP 参数：

```sh
"$NOSTOSX" add api ping --transport grpc --service order --dir .
task api ACTION=generate NOSTOSX="$NOSTOSX"
go mod tidy
```

此时只生成消息、适配器和测试，不创建 HTTP 映射，也不要求 gateway 运行库。
`--method` 与 `--path` 必须同时提供或同时省略。

## 从已有契约补齐实现

先在已有 v1 服务中编写 Proto RPC 和消息，再补齐缺失的传输入口：

```sh
"$NOSTOSX" api scaffold --service order --dir .
task api ACTION=generate NOSTOSX="$NOSTOSX"
go mod tidy
```

`api scaffold` 离线解析 `api/proto` 下的本地导入及 Protobuf 内置 well-known types，
不执行 Go、Buf，也不访问网络。其他依赖需要先放入该目录树。
每个缺失的 unary RPC 对应 `services/order/internal/port/rpc` 下一个 Go 文件，
初始返回 `Unimplemented`。签名匹配的已有方法保持不动，重复执行不会新增文件。
该命令不生成占位测试、HTTP 映射、业务用例或服务注册；实现认证、授权和应用调用后，
再编写对应的行为测试。

流式 RPC、已有方法签名不匹配、构建约束、平台专属文件、字段冲突和已有输出路径，
都会在写入前被拒绝。签名检查识别具体生成消息类型及导入别名；类型别名与点导入
需要手工处理。命令共用工程锁，普通写入错误会回滚本次创建的文件。

## 诊断工程环境

```sh
nostosx doctor --dir .
nostosx doctor --dir . --json
```

Doctor 检查本机 Go（至少 1.27.3）、`go.mod`、Buf v2 模块布局及有大小限制的本地
Proto 目录。有契约时检查 Buf 1.72.0，空骨架跳过 Buf。
它仅执行可信的 `go version`、`buf --version`，设置超时并禁止 Go 自动下载工具链；
不安装工具、不下载依赖、不修改工程文件。JSON 只写入标准输出；检查失败退出码为 1，
摘要写入标准错误。Doctor 用于环境准备，代码、契约语义和产物一致性仍由
`task check` 验证代码；启用 gRPC 的工程另外使用 `task api` 验证契约与生成产物。

两种布局遵循相同边界：

```text
app.go                              单体 Run 与显式服务装配
config/config.toml                  单体进程配置（TOML）
services/<name>/config/config.toml   独立服务进程配置（TOML）
api/proto/<context>/v1/               仅 gRPC 工程的源契约
api/gen/go/                          生成的 Go/gRPC/gateway 代码
api/http/                            HTTP 绑定及 OpenAPI 配置
application/<context>/command/       应用自行实现的写用例
application/<context>/query/         应用自行实现的读用例
domain/<context>/                    应用自行实现的领域
services/<name>/app.go               服务 Run 与显式 handler 挂载
services/<name>/internal/port/{http,rpc}/  按需启用的入站适配器
infrastructure/config/              Viper 配置源、类型化配置与校验
infrastructure/logging/             Zap 控制台/文件编码、轮转与关闭
infrastructure/server/              HTTP/gRPC 构造，不接收启动回调
infrastructure/rpc/                  认证及安全 RPC 错误
infrastructure/agent/                可选的模型配置及有界执行
```

上下文和服务名使用小写 Go 包名，工程名使用小写 slug；拒绝关键字与文件系统保留名。
生成过程拒绝覆盖已有输出，普通失败时回滚本次创建的内容。
`add api` 完成校验及新文件写入后才替换目标服务 Proto，保留其他源码；
回滚不涵盖进程崩溃。契约生成和新增操作共用临时
`.nostosx.lock`；仅在确认持有进程已停止后清理残留锁。`new` 不升级已有工程。

## 验证修改

```sh
task check INTEGRATION=true
task lint
task vulncheck
```

公开入口只有 `check`、`test`、`build`、`lint`、`vulncheck`、`fmt`、`fix`、`tidy` 八个，
直接执行 `task` 等同于 `task check`。`task test RACE=true` 开启竞态检测，
`COVER=true` 同时开启竞态检测，写入 `coverage.out` 并显示覆盖率；`check` 复用开启竞态检测的 `test`，
保留调用者的 `INTEGRATION` 选择。默认测试与检查不需要 Buf。
`INTEGRATION=true` 显式开启 Go/Buf 契约验收和十二种布局/agent/传输组合，
覆盖初始工程及添加上下文、服务、API 后的工程；CI 使用该模式。

`lint` 和 `vulncheck` 默认同时检查 CLI 与全部生成模板的扩展前后状态，
可用 `SCOPE=cli` 或 `SCOPE=templates` 选择范围，无需额外的任务名称。
无需 Docker、模型凭据或同级源码。单测、集成测试和模板检查均使用 Go 测试；
契约集成测试直接执行 Go CLI，需要 Buf 1.72.0，无需 Python 或生成工具源码。

命令树在 `internal/cli`，受限写入及内嵌模板在 `internal/scaffold`。
Go 工具不会直接检查 `.tmpl`，模板修改必须验证实际生成项目。
升级 CLI 不会自动修改已生成的应用。

参见[贡献指南](CONTRIBUTING.md)、[安全说明](SECURITY.md)、
[发布说明](RELEASING.md)、[变更记录](CHANGELOG.md)和 [MIT 许可](LICENSE)。
