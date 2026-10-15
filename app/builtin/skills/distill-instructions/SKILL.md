---
name: distill-instructions
description: 从 Distill 导出的 agent 会话记录中提炼长期有效的工作指令，写成 AGENTS.md 或 CLAUDE.md 片段存入 Distill 资源库（library/agents-md/ 或 library/claude-md/）。当用户要求“生成 AGENTS.md / CLAUDE.md”“整理给 agent 的说明”时使用。
---

# 从会话提炼 AGENTS.md / CLAUDE.md

## 输入

用户引用了若干会话记录（`runs/<id>/*.md`），其中 `## User` 是用户的输入，`## Assistant` 是助手的回复和工具调用。工具调用后的 `→ failed` 带报错摘录，`→ rejected by the user` 后的 `>` 是用户拒绝的理由，`_(interrupted by the user)_` 是用户打断——这些往往就是用户的纠正。

## 选择类型

- 要给 Codex、Pi 等多个 agent 用：写 `library/agents-md/<name>.md`。
- 只给 Claude Code 用，或内容依赖 Claude Code 的功能：写 `library/claude-md/<name>.md`。
- 用户指定了类型时按用户说的写。

## 步骤

1. 读 `library/README.md` 和同类型已有的片段，避免重复。如果有 `runs/<id>/instructions/README.md`，也读项目现有的指令文件，不要写和它们重复或矛盾的内容；发现项目的指令文件已经过时，在回复里提醒用户用 distill-instructions-audit 检查。
2. 读完所有被引用的会话，整理出 agent 每次开工都该知道的内容：
   - 用户是谁、偏好的沟通方式和语言；
   - 常用技术栈、构建/测试/运行命令；
   - 必须遵守的约束和反复出现的纠正；
   - 常见坑和正确做法（看 `→ failed` 的报错和最后成功的做法）。
3. 写成简洁的 Markdown：front matter（`name`、`description`）+ 正文。正文会被原样插入 agent 的指令文件，所以：
   - 用小标题分组，每条一行，能执行、能检查；
   - 不写会话中没有依据的内容；
   - 控制篇幅，宁缺毋滥（指令文件每次都会占用上下文）。
4. 用用户使用的语言写作。

## 结束

列出新建或修改的文件，并概括写入的要点。
