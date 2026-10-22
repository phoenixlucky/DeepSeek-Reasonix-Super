// Package gemini speaks Google's Interactions API (POST {base}/interactions) with
// a hand-written net/http client, stateless: every request carries the whole
// conversation and store is false, so the transcript stays Reasonix's own.
//
// Thinking arrives as thought steps whose signatures must be sent back exactly
// as received; the adapter keeps every thought, model_output and function_call
// step of a turn as an opaque provider item and replays them verbatim while the
// message still matches them. Temperature and top_p are not sent: the reference
// page does not list them in generation_config.
package gemini
