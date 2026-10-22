---
owner: @SivanCola
backup: @esengine
status: active
reviewed: 2026-10-18
---

# System persona / 系统人设

The user configuration may declare `[agent.persona]` with `prefix`, `suffix`
and `include_runtime_context`. Prefix and suffix supplement first-party guidance;
they cannot replace it. The retired `complete` field has no execution effect.
An absent persona preserves the existing default prompt.

用户配置可通过 `[agent.persona]` 设置 `prefix`、`suffix` 和
`include_runtime_context`。前后缀补充内置指令，不能替换它；旧 `complete`
字段不再影响执行。未配置人设时保持原有默认提示词。

Project `reasonix.toml` cannot set these fields. Loading restores the user's
values and reports an ignored user-only setting. This applies even when the
user has no persona configured.

项目 `reasonix.toml` 不能设置这些字段。加载时恢复用户配置，并报告忽略了项目中
的用户级设置；用户未配置人设时也同样如此。

`GET /persona` returns the user values and built-in starting points.
`POST /persona` saves `prefix`, `suffix` and optional `include_runtime_context`.
The text is trimmed and limited to 32 KiB per field, 64 KiB combined.

Saving does not rebuild a running session. New sessions and the next rebuild
of an existing session (for example, a model switch) read the saved persona.

`GET /persona` 返回用户设置和内置起始模板。`POST /persona` 保存 `prefix`、
`suffix` 及可选的 `include_runtime_context`。文本会去除首尾空白，每段最多
32 KiB，总计最多 64 KiB。保存本身不重建会话；新会话及现有会话下一次重建
（例如切换模型）时读取已保存的人设。
