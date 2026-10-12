---
owner: @esengine
backup: @SivanCola
status: active
reviewed: 2026-10-12
---

# Intelligent UI implementation

[RFC #12584](https://github.com/esengine/DeepSeek-Reasonix/issues/12584) defines the staged design. The desktop implementation covers recovery and measurement (P0), followed by the hidden `render_surface` tool, typed fields and choice cards (P1). TUI changes remain deferred by the owner.

## Recovery boundary

The extension UI hub retains at most 64 current publications, keyed by plugin and surface identity. A republish replaces that identity. Snapshots are isolated copies of the already validated and credential-redacted payload. Notifications are transient and are never replayed.

`GET /extensions/surfaces` reads the controller snapshot and the session's existing wire log. A window refresh in the same running generation restores the current payload.

A retired generation, crashed plugin, or restarted host returns identity only with `surface.expired`; the desktop removes old controls and displays the reason. It does not resubmit a form or open a question.

The wire log records only plugin, surface, session, generation and kind. It never records the publication body, field defaults or user answers. No new session schema or store is introduced.

A history predating this implementation has no publication identities to recover. A truncated or unreadable recorded history returns `surface.replay_incomplete`, rather than claiming that no surfaces existed.

A recovery snapshot does not owe entrance motion. Live publications received during a snapshot read take precedence for their identity; results from an obsolete read or unmounted pane are discarded.

This endpoint does not expand execution or approval authority.

## Desktop presentation

Recovered publications use the existing Studio `.flow` presentation. Expiry is a plain `restore-notice` status row. It has no tool gutter, vertical line, card frame or visible internal error code; `data-code` retains the typed identity for inspection and tests.

## Measurement definitions

All counters are local to one mounted desktop pane, exposed on its `PaneReport.interactions`. They are reset when that pane is remounted, are not uploaded, and contain no question, option or answer strings. Local deduplication retains prompt identities only for that pane's lifetime.

| Counter | Counts | Anchor and population | Excludes |
| --- | --- | --- | --- |
| `requests` | First observed structured ask request per prompt id | First request event in this mounted pane; model and external structured questions visible to this pane | Duplicate delivery and questions rebuilt from stored history; this is not the number of provider tool calls |
| `questions` | Question entries in those first requests | The same request population | Duplicate delivery and stored history |
| `options` | Declared option entries in those questions | The same request population | The frontend's added free-text affordance and answer content |
| `answers` | First successful local answer submission per prompt id | Acknowledged submission in this pane | Failed submissions, retries after the same acknowledgment, other devices and inferred outcomes |
| `customAnswers` | Those submissions with a selection absent from the corresponding declared option labels | The same acknowledged-answer population; exact option membership | Answer text, empty skips, and any claim that the model asked the same question again |

These counters measure desktop interaction exposure and local answers. They do not establish task success, user friction improvement, or an account-wide rate. A later fixed-task trajectory experiment must compare user turns to the same completed outcome before reporting a benefit from P1.

Surface requests and acknowledged submissions contribute to `requests` and `answers`. The other counters retain their ask-question and option-label definitions; they do not count individual surface fields or inspect typed input values.

## P1 fields and choices

The follow-up branch introduces `surfacespec` version 1. Its node is the existing `ExtensionViewNode` contract, with field metadata added. The desktop mirrors that same tree and the field types under wire parity.

The legacy plugin publisher keeps its existing catalogue and action path. Model-authored inputs use the same node contract, with submissions handled by the controller's ask path.

The model catalogue accepts text, plain-text markdown, semantic containers, key/value rows, meters, pips, dividers, fields and choice cards. It rejects action buttons, executable components and unknown properties.

Fields carry string, number, boolean, choice or multichoice values; choices use stable ids with optional description and preview text.

The parser caps a spec at 64 KiB, 200 nodes, eight tree levels, 16 fields and 16 options per field. Duplicate JSON keys and field ids are refused. Defaults and submitted JSON values are checked against declared types, bounds and option membership, with typed `surface.*` refusals.

Hosts explicitly enable `Options.NativeSurfaces` for the desktop and HTTP native renderer. Other entry points do not register the tool. It is hidden from provider schemas and discovered through `use_capability`; its schema is delivered in the turn tail.

Interactive trees publish a single ask carrying the validated spec and the host-owned tool call id. A desktop submits typed JSON in `AskAnswer.Value`. Validation and removal from the pending queue happen under the same lock; a refusal leaves the prompt pending.

Empty answer lists retain the existing explicit skip semantics.

Reloaded pending requests replace the matching expired history marker and retain local drafts. Submitted results restore settled inputs from the existing call/result record. Missing or invalid recorded answers show `surface.expired`; no historical card can submit.

Replaying or mounting a restored row owes no entrance motion.

The native renderer reuses `ExtensionView`, existing choice controls and input styling. Markdown and previews remain plain text. Both live and historical surface specs pass host credential redaction. Model surfaces cannot publish actions, grant permissions or settle approval prompts.

## Review and security review

The input boundary review covers the pending-ask map, legacy answer wrappers, HTTP attribution, cancel/timeout races and replay identities. Every answer entry reaches the same atomic schema check. Ordinary questions still use selections; typed values on those questions are refused.

The security review covers closed component kinds, bounded decoding, duplicate keys, field ids, required values and choice membership. Display strings and string defaults are redacted before native rendering. Approval policy, permission grants, device authentication and session storage ownership are unchanged.

Real boot effect tests prove hidden discovery, typed refusal/retry, pending replay and persisted resume. Adjacent tests cover ordinary asks, extension actions, recovery, frontend acknowledgment, local drafts and the proxy decision barrier. Full-suite baseline failures are reported separately from these targeted checks.
