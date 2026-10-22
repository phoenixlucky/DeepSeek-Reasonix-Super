export type BoardNameProblem = "short" | "long" | "chars";

const ALLOWED = /^[\p{L}\p{N} _-]+$/u;

// What the service accepts before it looks the name up: 2 to 30 characters of
// letters, digits, space, underscore or hyphen, so no "@" and no ".". Whether a
// name is taken or reserved is only the service's to say.
export function boardNameProblem(raw: string): BoardNameProblem | null {
  const name = raw.trim();
  const length = [...name].length;
  if (length < 2) return "short";
  if (length > 30) return "long";
  return ALLOWED.test(name) ? null : "chars";
}

// A name the person already uses for feedback, when it is one the board takes.
export const boardPrefill = (nickname: string | undefined): string => (nickname && boardNameProblem(nickname) === null ? nickname.trim() : "");

const OFFER_KEY = "rx-board-offer";

// The offer to join is made once on an install that was linked before it
// existed; whatever the answer, it is not made again.
export function boardOfferSeen(): boolean {
  try {
    return localStorage.getItem(OFFER_KEY) === "1";
  } catch {
    return false;
  }
}

export function markBoardOfferSeen(): void {
  try {
    localStorage.setItem(OFFER_KEY, "1");
  } catch {
    /* the offer may be made once more on a machine that cannot remember */
  }
}
