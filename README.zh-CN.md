# nostosx-cli

`nostosx` 命令创建干净的 Go 工程，随后按需添加业务上下文和服务。
支持单体与多服务布局，以及可选的 agentx 接入；两种布局均使用一个 Go 模块。

[English](README.md)

## CLI 与 Starter

| 项目 | 定位 |
| --- | --- |
| nostosx-cli | 生成工程骨架，再添加自己的业务上下文和服务。 |
| nostosx-starter | 运行和改造包含商品、库存与 agent 集成的参考应用。 |

本仓库维护命令和内嵌模板，生成时不会下载 Starter。生成的应用自行维护源码，
普通构建和运行不依赖这两个仓库；契约开发调用 CLI，不复制工具源码。
目前两个项目都尚未发布。

## 构建

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
export API_TOKEN="$(openssl rand -hex 32)"
go run ./cmd/server
```

在另一终端用 `curl http://127.0.0.1:8080/readyz` 检查就绪状态，Ctrl+C 停止。
默认宿主提供 HTTP 健康检查和需要认证的 gRPC 健康检查，不包含业务路由、数据库、
迁移、示例上下文或架构测试包。配置和扩展步骤见生成项目的中英文 README。

| 选项 | 行为 |
| --- | --- |
| `--module` | 必填，应用自己的小写 Go 模块路径 |
| `--layout monolith` | 默认，生成可启动的 `cmd/server` 宿主 |
| `--layout microservices` | 生成仓库骨架，添加服务后再启动进程 |
| `--with agentx` | 在 `infrastructure/agent` 添加模型配置、有界执行器及测试 |
| `--output` | 尚不存在的目标路径，父目录必须已存在 |

`new`、`add` 和 `api scaffold` 离线完成，不执行 Go、Buf、Shell、Git 或模型请求；
契约构建与检查命令会显式执行工具。新增源码时拒绝已有路径和符号链接，
不生成项目元数据文件。构建通过公开版本解析固定依赖，使用 `GOWORK=off`，
无需同级源码或本地 replace。未启用 agentx 时不生成其源码与依赖。
启用 agentx 也不会自动开放聊天接口或业务工具，这些由应用显式装配。

## 添加上下文和服务

```sh
"$NOSTOSX" add context order --dir .
"$NOSTOSX" add service order --dir .
make generate NOSTOSX="$NOSTOSX"
go mod tidy
go run ./services/order/cmd/order -addr 127.0.0.1:8081 -grpc-addr 127.0.0.1:9091
```

`add context` 通过 `doc.go` 声明领域、应用及 command/query 包，不预设用例或持久化。
`add service` 要求存在同名上下文，新增空的版本化 Proto 服务、RPC 适配器、
注册函数和独立进程入口。HTTP 映射与操作配置最初只包含说明注释，
实现服务时再定义真实 RPC 和对应的 HTTP 绑定。

添加服务后需显式生成契约并整理依赖。先安装 Buf 1.72.0，
契约生成期间会下载固定版本的 Go 插件。初始工程没有契约，构建无需 Buf。
单体注册和 agent 装配方式见生成项目 README。新增操作直接从 `go.mod` 读取模块路径；
添加上下文和服务只创建新文件，不改动已有装配或依赖。

## 构建契约

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

生成项目的 Makefile 默认通过 `go run` 调用 CLI v1.0.0，不将其加入应用模块。
该版本尚未公开发布，当前应使用源码构建的二进制：
`make generate api-check NOSTOSX="$NOSTOSX"`。
`NOSTOSX_VERSION` 可选择其他经审查的版本；`NOSTOSX` 用于指定本地二进制的绝对路径。
普通构建和测试不调用 CLI。契约命令会执行 Go 插件，可能访问网络及模块缓存；
Go、Buf、工程配置和插件源码必须可信，这些命令不提供沙箱隔离。

## 添加 API

已有上下文和服务后，显式添加操作：

```sh
"$NOSTOSX" add api create-order --service order --method POST --path /v1/orders --dir .
"$NOSTOSX" add api get-order --service order --method GET --path '/v1/orders/{order_id}' --dir .
make generate NOSTOSX="$NOSTOSX"
go get github.com/grpc-ecosystem/grpc-gateway/v2@v2.29.0
go mod tidy
make api-check NOSTOSX="$NOSTOSX"
```

`create-order` 对应 `OrderService.CreateOrder`。命令只在已有
`api/proto/order/v1/order.proto` 中插入 import 和 RPC 声明，另创建四个文件：

```text
api/proto/order/v1/create_order.proto                 请求与响应消息
api/http/routes/order_create_order.yaml                HTTP 映射
services/order/internal/port/rpc/create_order.go        尚未实现的 Handler 方法
services/order/internal/port/rpc/create_order_test.go   占位行为的回归测试
```

显式 `go get` 将 gateway 运行库固定到生成插件版本，初始骨架不依赖它。
已有工程若已选择更新的运行库，应先确认兼容性，再决定是否调整版本。

请求与响应默认没有业务字段；路径中显式声明的 `{order_id}` 会生成对应的
`string` 请求字段。按业务确定字段类型和请求体后重新生成。
支持 `GET`、`POST`、`PUT`、`PATCH`、`DELETE`；POST/PUT/PATCH 使用 `body: "*"`。
路径支持字面量和简单 `{field_name}` 参数，不支持通配符、嵌套字段或查询字符串。
API 名使用小写连字符 slug，目标必须是已存在的生成式 v1 服务。

RPC 入口先返回 gRPC `Unimplemented`；实现时将生成的占位测试改为真实行为测试。
命令不推测 CQRS 用例、不修改服务注册，也不自动挂载 HTTP。
对外开放前，在 `module.go` 中明确认证与授权，再显式挂载生成的 gateway 处理器。
OpenAPI 中存在契约并不表示 HTTP 路由已经运行。

生成前会检查工程内的 Proto 符号、Handler 方法及字段、已有输出和 HTTP 路由重叠。
路由判断采用保守规则，同一段的路径参数与字面量也可能重叠。
已有内联 `google.api.http`、通配选择器、自定义方法或不支持的路径模式时会拒绝操作，
应手工维护契约与映射，避免不完整的冲突判断。

仅提供 gRPC 的操作，省略两个 HTTP 参数即可：

```sh
"$NOSTOSX" add api ping --service order --dir .
make generate NOSTOSX="$NOSTOSX"
go mod tidy
```

此时只生成消息、适配器和测试，不创建 HTTP 映射，也不要求 gateway 运行库。
`--method` 与 `--path` 必须同时提供或同时省略。

## 从已有契约补齐实现

先在已有 v1 服务中编写 Proto RPC 和消息，再补齐缺失的传输入口：

```sh
"$NOSTOSX" api scaffold --service order --dir .
make generate NOSTOSX="$NOSTOSX"
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
`make check` 与 `make api-check` 验证。

两种布局遵循相同边界：

```text
api/proto/<context>/v1/               添加服务后创建的契约
api/gen/go/                          生成的 Go/gRPC/gateway 代码
api/http/                            HTTP 绑定及 OpenAPI 配置
application/<context>/command/       应用自行实现的写用例
application/<context>/query/         应用自行实现的读用例
domain/<context>/                    应用自行实现的领域
services/<name>/module.go            显式服务装配
services/<name>/internal/port/rpc/    入站适配器
infrastructure/bootstrap/            传输配置及生命周期
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
make check
make lint
make vulncheck
make smoke
make templates-lint
make templates-vulncheck
```

`make check` 验证 CLI 及四种布局/agent 组合，同时检查初始工程和添加上下文、服务、API 后的工程。
`make smoke` 在临时 loopback 端口运行宿主及新增服务，验证单体显式装配、信号退出，
验证 API 认证和未实现响应，并通过本地模型 fixture 验证 agentx。
模板 lint 和漏洞检查也覆盖扩展前后两种状态。
无需 Docker、模型凭据或同级源码。单测、进程冒烟和模板检查均使用 Go 测试；
契约集成测试直接执行 Go CLI，需要 Buf 1.72.0，无需 Python 或生成工具源码。

命令树在 `internal/cli`，受限写入及内嵌模板在 `internal/scaffold`。
Go 工具不会直接检查 `.tmpl`，模板修改必须验证实际生成项目。
修改 Starter 不会自动改变模板或已生成的应用。

参见[贡献指南](CONTRIBUTING.md)、[安全说明](SECURITY.md)、
[发布说明](RELEASING.md)、[变更记录](CHANGELOG.md)和 [MIT 许可](LICENSE)。
