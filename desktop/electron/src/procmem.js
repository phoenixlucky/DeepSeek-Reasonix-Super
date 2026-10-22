"use strict";

const TYPES = { Tab: "renderer", Browser: "browser", GPU: "gpu" };
// The kernel adds its own row, and a report holds a dozen.
const MAX_ROWS = 11;

const kb = (v) => (Number.isFinite(v) && v > 0 ? Math.min(Math.round(v), 2 ** 31) : 0);

// What app.getAppMetrics says, cut down to what a report may carry: a fixed
// type per process and two whole numbers. No pid, name, path or command line
// is read, so none can be sent.
function processMemoryRows(metrics) {
  return (Array.isArray(metrics) ? metrics : []).slice(0, MAX_ROWS).map((m) => ({
    process: TYPES[m?.type] ?? "utility",
    workingSetKB: kb(m?.memory?.workingSetSize),
    privateKB: kb(m?.memory?.privateBytes),
  }));
}

function processMemoryHandler(app, fromWindow) {
  return (event) => {
    if (!fromWindow(event)) return [];
    try {
      return processMemoryRows(app.getAppMetrics());
    } catch {
      return [];
    }
  };
}

module.exports = { processMemoryRows, processMemoryHandler };
