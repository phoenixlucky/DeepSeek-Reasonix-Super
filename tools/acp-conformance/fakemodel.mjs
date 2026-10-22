import http from "node:http";

// An OpenAI-compatible chat endpoint that answers from a script, so the agent
// under test runs a real turn without a network or a key.
export function fakeModel(script) {
  const seen = [];
  const server = http.createServer((req, res) => {
    let body = "";
    req.on("data", (chunk) => (body += chunk));
    req.on("end", () => {
      const request = body ? JSON.parse(body) : {};
      seen.push(request);
      const step = script(seen.length - 1, request);
      setTimeout(() => answer(step, res), step.delayMs ?? 0);
    });
  });
  const answer = (step, res) => {
    {
      res.writeHead(200, { "content-type": "text/event-stream" });
      const send = (delta, finish = null, extra = {}) =>
        res.write(`data: ${JSON.stringify({ id: "chatcmpl-fake", object: "chat.completion.chunk", created: 0, model: "m", choices: [{ index: 0, delta, finish_reason: finish }], ...extra })}\n\n`);
      send({ role: "assistant", content: "" });
      if (step.text) send({ content: step.text });
      (step.calls ?? []).forEach((call, index) =>
        send({ tool_calls: [{ index, id: call.id, type: "function", function: { name: call.name, arguments: JSON.stringify(call.args) } }] }));
      send({}, step.calls?.length ? "tool_calls" : "stop");
      res.write(`data: ${JSON.stringify({ id: "chatcmpl-fake", object: "chat.completion.chunk", created: 0, model: "m", choices: [], usage: { prompt_tokens: 100, completion_tokens: 10, total_tokens: 110 } })}\n\n`);
      res.end("data: [DONE]\n\n");
    }
  };
  return new Promise((resolve) => server.listen(0, "127.0.0.1", () => resolve({
    url: `http://127.0.0.1:${server.address().port}/v1`,
    seen,
    close: () => new Promise((done) => server.close(done)),
  })));
}
