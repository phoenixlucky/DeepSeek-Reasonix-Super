export interface MuxOpened {
  kind: "mux_open";
  conn: string;
}

export interface MuxAttachRequest {
  after?: number | null;
}

export interface MuxEnvelope {
  pane: string;
  frame: unknown;
}
