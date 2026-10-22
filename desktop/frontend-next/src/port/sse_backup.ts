import { SseIntake } from "./sse_intake";
import type { BackupApplyResult, BackupCatalog, BackupCategory, BackupCreated, BackupCreateRequest, BackupFileCatalog, BackupFileExport, BackupPlan } from "./backup";

// Backups kept in the signed-in account. Every call is refused while signed
// out, so a caller shows the section only when the account says signed in.
export class SseBackup extends SseIntake {
  backups() {
    return this.get<BackupCatalog>("/backups");
  }
  createBackup(req: BackupCreateRequest) {
    return this.post0<BackupCreated>("/backups", req);
  }
  deleteBackup(id: string) {
    return this.del("/backups/" + encodeURIComponent(id));
  }
  previewBackup(id: string, passphrase: string) {
    return this.post0<BackupPlan>("/backups/" + encodeURIComponent(id) + "/preview", { passphrase });
  }
  applyBackup(planId: string, items: string[], consented: string[]) {
    return this.post0<BackupApplyResult>("/backups/apply", { planId, items, consented });
  }
  backupFileCatalog() {
    return this.get<BackupFileCatalog>("/backups/file");
  }
  exportBackupFile(categories: BackupCategory[], passphrase: string) {
    return this.post0<BackupFileExport>("/backups/file/export", { categories, passphrase });
  }
  previewBackupFile(file: string, passphrase: string) {
    return this.post0<BackupPlan>("/backups/file/preview", { file, passphrase });
  }
  applyBackupFile(planId: string, items: string[], consented: string[]) {
    return this.post0<BackupApplyResult>("/backups/file/apply", { planId, items, consented });
  }
}
