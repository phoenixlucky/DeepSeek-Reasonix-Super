# Compaction prefix probe

## Method

1. Measured on 2026-10-17 against the official `https://api.deepseek.com` OpenAI adapter, using the model returned by `/models`: `deepseek-flash`. Thinking was disabled. Inputs were synthetic; no workspace history or credentials are recorded here.
2. Each of three text and three rich-history trials sent a main request to warm the prefix, then called the actual agent summary path, then an independent summary control. Rich history included a synthetic closed `read_file` call, result and reasoning. Warmup outputs were discarded, never executed.
3. A summary had a 1,024-token output cap; warmups had 64. Success means a heading, preservation of the fixture's `ParseDuration` and zero-value constraint, no emitted tool calls, and `finish_reason=stop`. This checks the fixture's contract, not arbitrary-summary factual completeness.
4. First-attempt reuse success counts successful reused summaries / six reused summaries. Fallback rate counts independent requests initiated by the reuse coordinator / six reuse trials; the six explicit independent controls are excluded. Both were 6/6 success and 0/6 fallbacks. Independent controls also succeeded 6/6.
5. The raw result file records provider-reported usage and full stream latency, measured from adapter `Stream` until channel close. Token rows below sum only the three indicated summary requests; they exclude warmup, model listing and any earlier exploratory requests. Cache residency was uncontrolled; prior probes had warmed these synthetic prefixes.

## Results

| History | Shape | N | Prompt | Hit | Miss | Output | Median ms | Estimated USD |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Text | Reused | 3 | 3,714 | 3,072 | 642 | 636 | 1,713 | 0.000487116 |
| Text | Independent | 3 | 1,218 | 768 | 450 | 644 | 1,610 | 0.000456204 |
| Rich | Reused | 3 | 3,780 | 3,072 | 708 | 742 | 1,774 | 0.000560616 |
| Rich | Independent | 3 | 1,341 | 768 | 573 | 921 | 1,815 | 0.000640854 |

1. Estimates use the [official pricing](https://api-docs.deepseek.com/quick_start/pricing/) checked on the measurement date: off-peak Flash USD per million tokens, hit 0.003, miss 0.15, output 0.6. October 17 was a Saturday. Formula: `(hit*0.003 + miss*0.15 + output*0.6)/1_000_000`. These are token estimates, not account deductions.
2. Reuse hit fractions were 82.7% (text) and 81.3% (rich), versus 63.1% and 57.3% for controls. This small fixture does **not** establish cost savings: retaining the system/catalog increased both total input and input cost. Text total cost increased 6.8%; rich total decreased 12.5% because output was shorter, not because input was cheaper.
3. The six warmups consumed 7,032 prompt and 231 output tokens, estimated USD 0.000346680. All 18 recorded generation calls together cost an estimated USD 0.002491470. No failed-digest retry occurred in these live trials; the deterministic fallback tests establish its behavior and accounting.
4. A rejected nonempty reused answer earns one independent-shape fallback. A separate truncation retry may enlarge the output cap, for at most three logical model calls total. Every actual attempt emits and records its own spend; returned usage is their sum. Empty output, stream errors and cancellation do not earn shape retries.
5. These sequential, six-pair trials establish interoperability for one model, mode and short fixture. They do not estimate production failure probability, thinking-mode behavior, other models, cold-cache cost, or large-history savings.

## Reproduction

1. Provide a test key through an existing secret environment configuration. Do not put credentials in source, command arguments or captured logs. Select a model currently returned by `/models`.
2. Run the opt-in probe; it skips without `DEEPSEEK_API_KEY`. The dedicated build tag avoids enabling unrelated live tests.

```sh
REASONIX_LIVE_MODEL=deepseek-flash go test -tags live_compaction ./internal/runtime/agent -run '^TestLiveCompactionPrefix$' -v -count=1
```

3. This probe exercises actual summary construction and the HTTP adapter. The hermetic `TestEffectCompactionSharesThePreviouslySentPrefix` separately exercises manual, automatic pressure and rich history through `boot.Build`, comparing actual JSON request-prefix and schema bytes. Its sparse-history case also proves an interleaved retained user turn survives the fold.
