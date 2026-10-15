# Distill

把本机 coding agent（Claude Code、Codex、Pi）的历史会话读出来，交给其中一个 agent 提炼成 Skill、Rule、AGENTS.md、CLAUDE.md，存进资源库，再一键应用到各个 agent。

## 目录

```
Distill/
├─ go.work                    # 本地工作区：agentlog + app
├─ agentlog/                  # 会话读取包，独立可复用（不含 UI、不调 LLM）
│  ├─ agentlog.go             # 统一格式：Session / Message / ToolCall / Transcript，Source 接口
│  ├─ index.go                # 并行扫描 + 增量缓存
│  ├─ markdown.go             # 导出 Markdown
│  ├─ trash.go                # 回收站：移入、恢复、清理
│  ├─ parse/                  # 写解析器用的工具：Builder、逐行读取、文本清洗、工具调用摘要、工具结果过滤
│  ├─ sources/                # sources.All()：登记所有 agent
│  │  ├─ claude/              # Claude Code：~/.claude/projects
│  │  ├─ codex/               # Codex：~/.codex/sessions
│  │  ├─ pi/                  # Pi：~/.pi/agent/sessions
│  │  └─ sources_test.go      # 三家的解析测试（样例在 ../testdata）
│  ├─ cmd/agentlog/           # 命令行：agentlog list / show
│  └─ testdata/               # 合成的样例日志
└─ app/                       # 桌面应用（MyGo NativeUI）
   ├─ main.go                 # 入口
   ├─ builtin/                # 内置 Skill 和资源库说明（编译进程序）
   ├─ resources/              # 应用图标：icon.svg 是源文件，icon.png（1024²）由它导出
   └─ internal/
      ├─ env/                 # ~/.distill 路径、配置、agent 检测（版本、模型）
      ├─ library/             # 资源库读写；应用到 agent（符号链接 / 副本 / 指令文件托管块）
      ├─ runner/              # 调用 claude -p / codex exec / pi -p，解析 JSON 输出
      └─ gui/                 # 界面：概览、Agent 页、会话页、对话抽屉、AI 提炼、资源库
```

## 新增一个 agent

1. `agentlog/sources/<name>/`：实现 `agentlog.Source`（`Agent`、`Files`、`Parse`），用 `parse.Builder` 组装消息：工具调用带上 ID，结果交给 `Builder.Result`（由 `parse.ToolResult` 判断成败、只留报错和简短输出），用户打断时调 `Builder.Interrupted`。在 `init` 里 `agentlog.RegisterLabel` 登记显示名；会话有附属文件时实现 `Relater`；实现 `Summarizer`（跳过工具结果）能让列出会话更快。
2. `agentlog/sources/sources.go`：在 `All` 里加一行。
3. `agentlog/testdata/home/` 加样例日志，`sources_test.go` 加测试。
4. app 里，要让它出现在界面上并能用于提炼：`internal/env/agents.go`（命令名、模型列表）、`internal/runner/runner.go`（非交互调用参数和输出解析）、`internal/library/apply.go`（skills 目录、指令文件）、`internal/gui/widgets.go`（图标和颜色）。

## 数据

都在 `~/.distill/`：

| 路径 | 内容 |
|---|---|
| `library/` | 资源库：`skills/<name>/SKILL.md`、`rules/`、`agents-md/`、`claude-md/`，以及给用户看的 `reports/`（不应用到 agent） |
| `builtin/` | 内置 Skill（每次启动从程序里写出） |
| `runs/` | 每次提炼导出的会话记录和项目指令文件的副本（`instructions/`），agent 从这里读 |
| `trash/` | 删除的会话，30 天后清理 |
| `config.json` | 启用的 agent、模型、Skill 启用方式 |
| `index.gob` | 会话列表缓存 |

提炼时 agent 的工作目录是 `~/.distill`，只允许写 `library/`。

## 命令

```sh
cd app
go run .                                   # 运行
go build -ldflags="-s -w" -tags mygo_noinspector -o ../build/Distill .   # 发布构建（约 11 MB）
go test ./...                              # 单元测试

# 用真实数据或真实 agent 的测试，默认跳过：
DISTILL_SNAPSHOTS=/tmp/shots go test ./internal/gui -run Snapshots   # 各页面截图
DISTILL_RUNNERS=1 go test ./internal/runner -run Runners -v          # 三家 CLI 多轮对话
DISTILL_E2E=1 go test ./internal/gui -run E2E -v                     # 用 Claude 提炼一个真实会话
DISTILL_E2E=1 DISTILL_E2E_PROMPT=项目问题诊断 DISTILL_E2E_MODEL=sonnet go test ./internal/gui -run E2E -v   # 换一个快捷提示

cd ../agentlog
go test ./...
AGENTLOG_REAL=1 go test ./sources -run RealLogs -v   # 读本机全部会话：核对消息数，统计工具调用的成败
go run ./cmd/agentlog list                 # 列出全部会话
go run ./cmd/agentlog show <session.jsonl> # 输出一个会话的 Markdown
```

## 安装

```sh
brew install --cask m2thane/tap/distill
```

或者从 [Releases](https://github.com/M2tHane/Distill/releases) 下载 dmg。应用没有 Apple 签名，直接下载的版本第一次打开会被拦下，到「系统设置 → 隐私与安全性」点「仍要打开」。

## 发布

```sh
cd app
mygo build -platform darwin/universal      # build/darwin-universal/Distill.app 和 dmg；版本号在 mygo.json
```

打包工具：`go install github.com/egoist/mygo/cmd/mygo@v0.4.6`。发布后更新 [homebrew-tap](https://github.com/M2tHane/homebrew-tap) 里 `Casks/distill.rb` 的 `version` 和 `sha256`。
