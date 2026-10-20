# Studio Feature Port Design

## Goal

Port the user-facing capabilities introduced on `main-v2` to the latest `studio` branch while keeping Studio's current architecture and native behavior. The work covers managed VISION-LIAO/Cognee memory, the shipped WJ Forge and Figma skills, the Chrome MCP bridge, and Windows desktop packaging support.

## Existing Studio boundaries

Studio's turn lifecycle belongs to `internal/session/control.Controller`. Its local memory store under `internal/state/memory` remains the canonical local store, with automatic recall projected into the current turn. Skills are managed under `internal/ext/skill`, and the desktop UI is `desktop/frontend-next`. MCP configuration, authorization, connection state, and tool access already have their own control and UI paths. Windows release packaging already has signing and artifact verification scripts.

The source branch uses different controller, skill, desktop frontend, and packaging layouts. Its commits are not a safe unit to cherry-pick into Studio. Port behavior through Studio's existing owners and contracts.

## Design

### 1. Cognee memory is an optional external integration

Keep Studio's local memory store and `/remember` behavior unchanged. Cognee supplies a separate, opt-in external recall and proposal channel; it does not replace or mirror the local store.

On an eligible visible user turn, the controller may make one bounded recall through an authorized MCP server named `cognee`. Recall uses only the supported `get_guidance` or `search_memory` tool, after validating its schema. Retrieved data is sanitized, length-bounded, and attached to the request as transient turn context. It does not enter the session transcript or stable system-prompt prefix. Treat it as prior evidence: it cannot override the current request, standing instructions, permissions, or safety rules.

After a successful task, summarize only the task request and visible final outcome. Do not send the transcript or tool chatter to the summarizer. Redact credentials and absolute paths before any external call. Submit a concise private proposal when reusable information exists. Submit a separate global proposal only when the task has complete TODOs, if any, and reliable project-level evidence. Cognee owns proposal review; Reasonix reports submission status without treating a pending proposal as approved memory. Persist separate one-attempt markers for private and global destinations; uncertain network outcomes are not retried automatically. Cognee errors remain fail-open for the task.

Expose only the managed recall and proposal tools to this lifecycle. Do not make arbitrary Cognee tools, deletion, or approval actions available to the model.

### 2. Settings use Studio's MCP and credential owners

Add a Cognee settings surface in `desktop/frontend-next` that shows skill enablement and distinct service states: not configured, disabled, authorization required, connection failure, incompatible tools, and ready. Reuse Studio's existing MCP install/update, authorization, reconnect, source ownership, and secret storage paths. Support HTTPS and loopback HTTP, store the bearer token in the credential store, and keep only an environment reference in MCP configuration. Setup may inspect tool schemas but must not read or write memory.

Seed a default local Cognee MCP declaration only when no user-global declaration already exists. Keep it disconnected until use. Do not overwrite project or plugin declarations. Do not bundle or install a Cognee backend.

### 3. Skills use the existing skill registry

Embed the WJ Forge skills and Figma design skill under `internal/ext/skill/builtincontent`, with their categories and source links. Keep enablement in the canonical Studio skill configuration and add update metadata/actions to the current Skills settings surface.

Only skills with fixed, reviewed HTTPS sources are updateable. Fetch a bounded Markdown document, verify its expected skill identity and allowed frontmatter, and atomically save it under the existing Reasonix state home. Updates change the installed body but preserve enablement preferences. A skill remains instructions only and does not grant tools or permissions.

### 4. Chrome MCP bridge uses Studio's MCP lifecycle and packaging

Add the Streamable HTTP-to-stdio bridge to the desktop package and configure it through the existing MCP server lifecycle. The bridge may target the local Chrome MCP service, but the Chrome backend itself is not bundled. Preserve protocol negotiation, bounded timeouts, and clear connection errors. The settings UI should use Studio's MCP status rather than invent a second connection state.

### 5. Windows packaging follows Studio's release pipeline

Compare the existing Studio Windows release path with the source branch's x64 installer/ZIP entry point. Add only the missing build capability, using Studio's artifact naming, signing, and verification conventions. Do not create a parallel unsigned release path if the existing pipeline already covers the requested output.

## Data flow and failure behavior

1. Studio's controller evaluates the current task and memory eligibility.
2. Local memory recall continues through the current store. If Cognee is enabled and its MCP server is authorized and ready, one bounded external recall is added as transient context.
3. The normal task runs without depending on Cognee availability.
4. On successful completion, an isolated summary is generated and redacted. Private and eligible global proposals are submitted independently.
5. Desktop notices report recall and proposal outcomes; they never claim a proposal was approved or saved as active memory.

Credentials, raw transcripts, absolute workspace paths, and unreviewed proposal content must not be exposed in logs or UI. Project configuration keeps precedence over user defaults. Runtime changes take effect at the existing Studio lifecycle boundary rather than changing an active turn.

## Scope and non-goals

In scope: the features listed above, their settings and MCP wiring, skill update support, documentation, and the minimum integration required by Studio's desktop and packaging paths.

Out of scope: replacing Studio's local memory system; uploading source files or transcripts; direct Cognee approval or deletion; bundling a Cognee database/server; general-purpose skill auto-updates from user-provided URLs; porting unrelated `main-v2` changes; changing Studio's broader release/signing policy.

## Acceptance criteria

- Studio's local memory behavior remains authoritative and works when Cognee is absent.
- Cognee recall is opt-in, occurs at most once per eligible task, is bounded and sanitized, and never enters the transcript or stable prompt prefix.
- Successful tasks can produce separate private and evidence-gated global proposals; incomplete, failed, cancelled, empty, or trivial tasks do not produce proposals.
- Cognee setup stores secrets safely, distinguishes readiness states, reuses MCP authorization, and does not invoke memory tools during setup.
- Models cannot call arbitrary Cognee tools or bypass human proposal review.
- Embedded skills are discoverable and individually configurable; only fixed validated sources can update them.
- The Chrome bridge is packaged and launched through Studio's existing MCP lifecycle without bundling its backend.
- Windows outputs use the existing Studio release conventions and do not create a competing release path.
- Desktop, CLI, and other controller-backed frontends continue to share the same memory lifecycle and error behavior.
