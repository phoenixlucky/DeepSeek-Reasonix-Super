---
owner: @esengine
backup: @SivanCola
status: active
reviewed: 2026-10-17
---

# Relay harness

A hosted page driving a real Studio through a stand-in relay.

- Real: the production page bundle, its encrypted channel, `reasonix-studio-host` with its kernel, and Chromium.
- Replaced: the accounts API and the message relay (`tools/relayfake`), and the model provider (a scripted stream in `stack.mjs`).

The page still dials `id.reasonix.io` and `remote.reasonix.io`; the browser
context answers those two hosts with the stand-ins.

```
go build -o relay/bin/studio-host ../../cmd/reasonix-studio-host
go build -o relay/bin/relayfake ../../tools/relayfake
pnpm exec vite build --outDir dist-relay --emptyOutDir
RELAY_HOST_BIN=$PWD/relay/bin/studio-host RELAY_FAKE_BIN=$PWD/relay/bin/relayfake \
  RELAY_DIST=$PWD/dist-relay node relay/live.mjs
```

`relay/storm.mjs` is the other scenario: a page served by the Studio itself (no relay), nine panes on the shared stream, and the browser going offline and back across three turns. Run it the same way, with `RELAY_DIST` pointing at the same build.

`CHROMIUM_PATH` selects a browser other than the one Playwright installed.
A change to how events travel between a page and a kernel is judged by this
run, not only by a unit test of one side. Add a scenario to `live.mjs` rather
than a second harness.
