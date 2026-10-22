import { SseBackup } from "./sse_backup";
import type { FeedbackBoard, FeedbackBoardMe, FeedbackEnv, FeedbackImage, FeedbackLinkedInstall, FeedbackMine, FeedbackReceipt, FeedbackReplyReceipt, FeedbackRequest } from "./feedback";

export class SseFeedback extends SseBackup {
  feedbackEnv(locale?: string) {
    return this.get<FeedbackEnv>("/feedback/env" + (locale ? "?locale=" + encodeURIComponent(locale) : ""));
  }
  sendFeedback(req: FeedbackRequest) {
    return this.post0<FeedbackReceipt>("/feedback", req);
  }
  myFeedback() {
    return this.get<FeedbackMine>("/feedback/mine");
  }
  replyFeedback(receipt: string, body: string, images: FeedbackImage[] = []) {
    return this.post0<FeedbackReplyReceipt>("/feedback/" + encodeURIComponent(receipt) + "/reply", images.length > 0 ? { body, images } : { body });
  }
  withdrawFeedback(receipt: string) {
    return this.post("/feedback/" + encodeURIComponent(receipt) + "/withdraw", {});
  }
  announceFeedbackReply() {
    return this.post("/notifications/feedback-reply", {});
  }
  ackFeedbackPromotion(level: number) {
    return this.post("/feedback/promotion/ack", { level });
  }
  feedbackSeen(receipt: string, upTo: number) {
    return this.post("/feedback/" + encodeURIComponent(receipt) + "/seen", { upTo });
  }
  async feedbackLinks() {
    return (await this.get<{ installs: FeedbackLinkedInstall[] }>("/feedback/links")).installs;
  }
  linkFeedbackAccount() {
    return this.post("/feedback/link");
  }
  unlinkFeedbackAccount() {
    return this.del("/feedback/link");
  }
  feedbackBoard(release?: string) {
    return this.get<FeedbackBoard>("/feedback/board" + (release ? "?release=" + encodeURIComponent(release) : ""));
  }
  feedbackBoardMe() {
    return this.get<FeedbackBoardMe>("/feedback/board/me");
  }
  async joinFeedbackBoard(displayName: string) {
    await this.post("/feedback/board/join", { displayName });
    return this.feedbackBoardMe();
  }
  async leaveFeedbackBoard() {
    await this.post("/feedback/board/leave", {});
    return this.feedbackBoardMe();
  }
  unlinkFeedbackInstall(id: string) {
    return this.del("/feedback/links/" + encodeURIComponent(id));
  }
}
