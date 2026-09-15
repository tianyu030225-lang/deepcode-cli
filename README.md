# Deepcode CLI

Deepcode 是一个 Linux 终端里的 AI 编程助手。进入项目目录后，可以让它读代码、审查文件或执行开发任务。支持 DeepSeek API，也可以配置兼容 OpenAI 接口的模型服务。

程序由 Go 和 C 组成：Go 处理终端交互、会话和网络请求，C 处理文件、进程、管道、信号及 JSON 数据。会话保存在本地，不需要数据库。

## 构建和安装

需要 Linux、Go 1.21+、Make、C 编译器（`cc`）和 `ar`。终端需要支持 TTY 和 ANSI 控制序列。

在仓库根目录执行：

```bash
make -C mycc
./build/deepcode
```

第一次运行构建产物会将程序安装到 `~/.local/bin/deepcode`，随后退出。安装器会尝试把这个目录加入 shell 配置。若当前终端仍找不到命令，执行：

```bash
export PATH="$HOME/.local/bin:$PATH"
```

进入要处理的项目目录，再启动 `deepcode`：

```bash
cd /path/to/your/project
deepcode
```

更新时重新构建并运行 `./build/deepcode` 即可。重新安装会保留已有配置和会话。

要指定 Go 的位置，可以使用 `make -C mycc GO=/usr/local/go/bin/go`。

## 配置

首次启动会进入配置向导：

- **DeepSeek API**：填写 API Key，使用内置的地址和模型配置。
- **自定义 API**：填写 API Key、Base URL，以及默认、轻量、最强三个模型名称。

向导也可以设置回复长度。之后用 `deepcode -setting` 重新配置。

配置保存在 `~/.deepcode/config.json`，历史会话保存在 `~/.deepcode/sessions`。

## 常用命令

输入 `/` 可以查看命令并补全。中文和英文命令作用相同。

| 命令 | 用途 |
| --- | --- |
| `/review`、`/审查` | 读取当前目录的代码快照并交给模型审查 |
| `/resume`、`/恢复` | 打开历史会话 |
| `/rename 名称`、`/重命名 名称` | 修改当前会话名称 |
| `/compact`、`/压缩` | 压缩当前会话的上下文 |
| `/agent`、`/子代理` | 将下一次任务交给并行子代理处理 |
| `/model`、`/模型` | 切换已配置的默认、轻量或最强模型 |
| `/thinking`、`/思考` | 开关 DeepSeek thinking 模式，自定义服务可能不支持 |
| `/show-thinking` | 查看最近一次保存的 thinking 内容 |
| `/show`、`/显示` | 切换终端显示模式 |
| `/danger`、`/危险` | 允许下一次任务写文件、删文件和执行 shell |
| `/exit`、`/退出` | 退出程序 |

`deepcode -show` 可以重新播放启动动画。

## 文件访问与命令执行

默认只允许读取工作区文件和获取当前目录。写文件、删除文件和执行 shell 需要授权，授权菜单会等待选择，按 `Esc` 可以取消。

`/danger` 的授权在下一次任务结束后撤销。另有不出现在补全列表中的 `/very-danger`，会持续授权到当前程序退出，不保存到配置或历史会话中。

文件工具会在实际操作时校验工作区路径，防止通过符号链接访问工作区外的文件。**shell 没有文件系统沙箱**：授权后的命令以当前用户权限运行，可以访问其他目录或联网。

`/review` 会跳过符号链接和非普通文件，并将选中的代码快照发送到你配置的模型服务。使用前应确认这些代码可以发送给该服务。

## 取消任务和子代理

按 `Esc` 可以清空当前输入、停止模型回复，或取消仍在运行的子代理。中断的模型回复不会写入会话历史。

`/agent` 只影响下一次任务。模型会将任务拆成 1–4 个子任务并行处理，主界面等待它们完成后汇总结果；等待期间不能继续输入聊天消息，但可以按 `Esc` 取消。子代理继承启动时的权限，运行期间不会再次弹出授权菜单。

shell 工具会随任务取消，并在调用结束时清理受管的进程组。直接子进程退出后，输出管道最多再读取 1 秒。因此，这个工具不适合启动需要长期运行的后台服务。

## 开发

```text
mycc/
  Makefile    构建入口
  go/         CLI、会话、网络请求和 Go/C 桥接
  c/          文件、进程、权限、路径和 JSON 处理
build/        构建产物
```

现有检查可在仓库根目录运行：

```bash
make -C mycc test
make -C mycc vet
make -C mycc race
```

Go/C 分工及任务处理方式见 [设计记录](1.md)。

## 许可证

项目代码采用 [MIT License](LICENSE)。仓库中的 cJSON 保留原有版权和许可证，见 [第三方说明](THIRD_PARTY_NOTICES.md)。
