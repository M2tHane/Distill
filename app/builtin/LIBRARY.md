# Distill 资源库

这个目录是 Distill 的资源库。Distill 会把这里的资源应用到 Claude Code、Codex、Pi。

## 结构

```
library/
├─ skills/<name>/SKILL.md      Skill：一个目录，SKILL.md 必需，可附带 references/、scripts/
├─ rules/<name>.md             Rule：一条或一组相关的规则
├─ agents-md/<name>.md         AGENTS.md 片段：通用的长期指令
└─ claude-md/<name>.md         CLAUDE.md 片段：给 Claude Code 的长期指令
```

- `<name>` 用小写英文和连字符，例如 `go-error-handling`，并与 front matter 里的 `name` 一致。
- 每个文件以 front matter 开头，至少有 `name` 和 `description`：

```markdown
---
name: go-error-handling
description: 一句话说明这是什么、什么时候用。
---
```

- Rule、AGENTS.md、CLAUDE.md 应用时，front matter 会被去掉，正文写进 agent 的指令文件，所以正文要能独立读懂。
- 不要修改 `library/` 之外的文件；`runs/` 下是只读的会话记录。
