// What holds the tokens of a prompt: the six classes by size, largest first,
// and the biggest single messages. active is the running turn, which a fold
// cannot take.
export interface ContextComposition {
  total: number;
  at: number;
  active: number;
  parts: { name: string; tokens: number }[];
  largest?: { role: string; tool?: string; tokens: number; active?: boolean }[];
}
