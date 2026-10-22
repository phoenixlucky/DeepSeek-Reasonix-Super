import Papa from "papaparse";

export function parseDelimited(text: string, delimiter: string, maxCells = 100_000): string[][] {
  const rows: string[][] = [];
  let cells = 0;
  let failure = "";
  Papa.parse<string[]>(text.replace(/^\uFEFF/, ""), {
    delimiter, dynamicTyping: false, skipEmptyLines: false,
    step(result, parser) {
      if (result.errors.length) failure = "invalid";
      cells += result.data.length;
      if (cells > maxCells) failure = "limit";
      if (failure) parser.abort();
      else rows.push(result.data);
    },
  });
  if (failure) throw new Error(failure);
  return rows;
}
