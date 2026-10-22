import { faultDetail } from "./fault";
import { heldDraft, holdDraft } from "./feedbackdraft";

// What failed is written into the feedback form's draft, appended to anything
// already there; a defect of the page also carries where it came from.
export function reportProblem(text: string): void {
  const detail = faultDetail(text);
  const note = detail ? `${text}\n\n${detail}` : text;
  const draft = heldDraft();
  holdDraft({ ...draft, category: "bug", body: draft.body ? `${draft.body}\n\n${note}` : note });
}
