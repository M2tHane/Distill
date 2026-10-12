---
name: distill-skill
description: 从 Distill 导出的 agent 会话记录中提炼可复用的 Skill，写入 Distill 资源库（library/skills/<name>/SKILL.md）。当用户要求“根据这些会话生成 skill”“把这个流程做成 skill”时使用。
---

# 从会话提炼 Skill

## 输入

用户在消息里引用了若干会话记录（`runs/<id>/*.md`）。每份记录只包含：
- `## User`：用户的输入
- `## Assistant`：助手的回复，以及 `- **工具名** 摘要` 形式的工具调用（没有工具输出）

## 步骤

1. 读 `library/README.md`，了解资源库的格式。
2. 读完所有被引用的会话记录。先浏览 `library/skills/` 下已有的 skill，避免重复；能扩充已有 skill 时就改它。
3. 找出**反复出现、步骤稳定、值得复用**的流程，例如：固定的排查顺序、发布步骤、某个工具链的正确用法、用户多次纠正后才做对的做法。一次性的任务不要做成 skill。
4. 每个 skill 写成 `library/skills/<name>/SKILL.md`：
   - front matter：`name`（小写连字符，与目录同名）、`description`（说清楚**做什么**和**什么时候用**，这决定 agent 会不会调用它）。
   - 正文：目标、前置条件、具体步骤（可直接执行的命令或检查项）、常见坑（尤其是会话里出过错的地方）、完成标准。
   - 只写会话里有依据的内容，不要编造命令、路径或参数。
   - 内容长时，把细节放进 `references/*.md`，SKILL.md 里写明何时读它。
5. 用用户使用的语言写作。

## 结束

列出新建或修改的文件，每个附一句说明。如果会话里找不到值得做成 skill 的流程，直接说明原因，不要硬写。
