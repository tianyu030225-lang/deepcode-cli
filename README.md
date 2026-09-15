# Deepcode CLI

Deepcode 是一个运行在 Linux 终端里的 AI 编程助手 CLI。它支持 DeepSeek API 和自定义 OpenAI-compatible API，提供本地会话保存、上下文压缩、阻塞式并行子代理、危险操作授权、会话恢复、Esc 取消和终端动画界面。

项目采用 Go 外壳 + C 核心的结构。Go 负责 CLI 主循环、命令系统、会话、界面编排以及 HTTP/HTTPS + SSE 网络流；C 负责贴近 Linux 系统编程的核心能力，包括文件操作、shell/子代理进程执行、pipe 输出捕获、进程组取消、危险权限菜单、工作区真实路径校验、模型请求 JSON 构造、SSE chunk 解析和 tool 参数解析。

## 功能

- 单文件安装：运行构建出的二进制后，会安装到 `~/.local/bin/deepcode`。
- 首次启动配置：选择 DeepSeek API 或自定义 API，填写 API Key、Base URL 和模型名称。
- 终端界面：蓝色风格启动动画、欢迎面板、命令补全、状态栏和 Esc 取消。
- 本地会话：会话保存在 `~/.deepcode/sessions`，可恢复、压缩和重命名。
- 工具调用：支持读取文件、写文件、删除文件、执行 shell、查看当前目录。
- 权限控制：写文件、删文件、执行 shell 都需要危险权限授权；文件工具校验工作区边界。shell 以当前用户权限运行，不提供文件系统沙箱。
- 子代理：`/子代理` 会让下一次任务拆成多个 Agent 并行执行；主进程等待完成，显示专用动画，完成后自动整合结果。
- C native 核心：文件、进程、管道、信号、终端权限菜单、路径安全和 JSON 处理都在 `mycc/c` 中实现。

## 构建

进入构建目录：

```bash
cd mycc
make
```

构建产物会生成到：

```bash
../build/deepcode
```

运行测试：

```bash
cd mycc
make test
```

可选静态检查：

```bash
cd mycc
make vet
```

Race 检查：

```bash
cd mycc
make race
```

默认 `Makefile` 使用 `PATH` 中的 `go`。如果需要指定其它 Go 安装位置，可以覆盖 `GO`：

```bash
cd mycc
make GO=/usr/local/go/bin/go
```

## 安装和启动

首次运行构建出的二进制：

```bash
./build/deepcode
```

程序会显示安装进度动画，并复制自身到：

```bash
~/.local/bin/deepcode
```

重新运行新构建的二进制进行安装时，会保留已有配置和会话，不清空 `~/.deepcode`。

如果 `~/.local/bin` 不在 `PATH` 中，安装器会尝试写入 shell 配置。也可以手动执行：

```bash
export PATH="$HOME/.local/bin:$PATH"
```

安装后启动：

```bash
deepcode
```

重新显示启动动画：

```bash
deepcode -show
```

重新进入配置向导：

```bash
deepcode -setting
```

## 首次配置

Deepcode 启动时会让你选择 API provider：

- `DeepSeek API`：使用默认 Base URL `https://api.deepseek.com`，只需要填写 DeepSeek API Key。
- `自定义 API`：需要填写 API Key、Base URL、默认模型、轻量模型和最强模型。

当前默认模型常量：

- 默认模型：`deepseek-v4-pro`
- 轻量模型：`deepseek-v4-flash`
- 最强模型：`deepseek-v4-pro`

回复风格支持：

- `简洁优先`：能短则短，不废话。
- `适中`：该详细的地方详细，该短的地方短。
- `详细`：宁可多说，不漏重要信息。

配置文件位置：

```text
~/.deepcode/config.json
```

会话目录：

```text
~/.deepcode/sessions
```

## 命令

在输入框键入 `/` 会显示可用命令。

| 命令 | 作用 |
| --- | --- |
| `/压缩` 或 `/compact` | 压缩当前会话上下文。 |
| `/子代理` 或 `/agent` | 让下一次用户任务强制启动并等待并行子代理，完成后自动整合。 |
| `/思考` 或 `/thinking` | 开关 DeepSeek thinking 模式。自定义 provider 下会提示可能不可用。 |
| `/恢复` 或 `/resume` | 查找并恢复本地历史对话。 |
| `/rename 名称` 或 `/重命名 名称` | 重命名当前会话。也可以只输入命令后交互输入名称。 |
| `/model` 或 `/模型` | 切换当前会话使用的默认、轻量或最强模型。 |
| `/show` 或 `/显示` | 切换炫彩模式和简洁模式。 |
| `/show-thinking` | 显示最近一次保存的 thinking 内容。 |
| `/review` 或 `/审查` | 审查当前工作目录的代码快照。 |
| `/危险` 或 `/danger` | 仅下一次用户任务允许写文件、删除文件和执行 shell。 |
| `/退出` 或 `/exit` | 退出 CLI。 |

隐藏命令：

```text
/very-danger
```

它不会出现在自动补全中。开启后，当前终端会话持续允许写文件、删除文件和执行 shell。

## 权限模型

普通权限下，模型只能：

- 获取当前工作目录。
- 读取当前工作目录内的文件。

以下工具属于危险操作：

- `write_file`
- `delete_file`
- `execute_bash`

危险操作会触发授权菜单。菜单默认处于等待选择状态，用户不操作不会被当成拒绝；只有明确选择拒绝才会拒绝，按 Esc 会取消。

`/危险` 只对下一次用户任务生效。任务结束后会撤销。

`/very-danger` 只对当前终端窗口生效，不持久化到配置文件或会话文件。

文件工具在实际操作时校验工作区真实路径。`/review` 的代码快照跳过符号链接和非普通文件。

`execute_bash` 不是文件系统沙箱：被授权的命令可以访问当前用户有权访问的其他路径，也可以联网。工作目录只决定命令从哪里开始执行，不能阻止命令访问目录外部。运行 `/review` 会把选中的工作区代码快照发送给配置的模型服务，请仅在可以发送这些代码的项目中使用。

## Esc 取消

- 输入框中按裸 `Esc` 会取消当前输入。
- 模型流式回复时按 `Esc` 会停止本次请求，已显示的半截回复不写入会话历史。
- 子代理执行期间按 `Esc` 会取消所有仍在运行的子代理。

Shell 工具接入当前任务的取消信号。一次调用结束时会清理其受管进程组；输出管道在直接子进程退出后最多继续排空 1 秒，因此不应将该工具用于启动需要长期驻留的后台服务。

## 子代理

输入 `/子代理` 后，下一次用户消息会被拆分成 1 到 4 个可并行处理的子任务。Deepcode 会启动多个 Agent 并行执行，并让主进程进入等待状态。

等待期间：

- 主输入框不会继续接受普通聊天输入。
- 界面显示区别于主模型推理的子代理专用动画。
- 可以按 `Esc` 取消所有仍在运行的子代理。

所有 Agent 完成后，CLI 会把 done、failed、canceled 状态和结果注入上下文，并由主模型自动生成最终答复。子代理状态也会注入系统上下文，模型回答“子代理是否在干活”时应以真实状态为准。

如果启动子代理时处于 `/危险` 或 `/very-danger` 权限下，子代理会继承当时的权限快照。子代理运行中不会再次弹出授权菜单。

## 开发结构

主要目录：

```text
Second_project_v2/
  1.md             需求记录
  build/deepcode   构建产物
  mycc/
    Makefile        构建入口
    go/             Go 外壳
    c/              C native 核心和 cJSON
```

`mycc/go` 中的关键模块：

- `main.go`：入口、安装判断、主循环。
- `commands.go`：slash 命令、恢复、重命名、审查命令。
- `agent.go`：用户回合、工具循环、上下文压缩、并行子代理。
- `openai.go`：HTTP 请求、SSE 读取、模型流式响应处理。
- `tools.go`：工具定义、参数校验、工具执行分发。
- `native.go`：Go 调用 C 的薄桥接层。
- `session.go`：本地会话结构和保存恢复。
- `ui.go` / `animation.go` / `render.go`：终端界面和动画。

`mycc/c` 中的关键模块：

- `native.h`：Go/C 桥接使用的公共类型和函数声明。
- `file_ops.c` / `path_utils.c`：文件读写、删除与工作区路径处理。
- `exec.c` / `sub_agent.c`：进程执行、输出管道、取消与子代理启动。
- `permission.c` / `json_utils.c` / `util.c`：权限菜单、JSON 封装和公共辅助函数。
- `cJSON.c` / `cJSON.h`：vendored cJSON。
- `touxiang.c`：头像字符画的独立演示文件，不参与主程序链接。

## 注意事项

- 当前项目面向 Linux 终端环境，依赖 TTY 和 ANSI 控制序列。
- 构建需要 Go 1.21+、`cc`、`ar` 和可用的 C 编译环境，因为启用了 CGO。
- HTTP/TLS/SSE 网络层仍由 Go 负责；C 负责系统编程核心能力和结构化 JSON 构造/解析。
- 当前会话存储使用本地 JSON 文件，不依赖 MySQL、SQLite 或其它数据库。

## 许可证

项目作者有权授权的自有代码采用 [MIT License](LICENSE)。cJSON 的原始版权和许可文本继续保留，详见 [第三方说明](THIRD_PARTY_NOTICES.md)。构建产物、本机索引、测试临时文件和凭据不属于公开源码包。
