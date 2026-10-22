package i18n

var queueEN = QueueMessages{
	QueueImageWait:       "Has an image: sent when this turn ends",
	TUIQueueUnrecovered:  "inbox contents UNKNOWN — the queue is unreadable; paused for inspection",
	QueueRowsMoreFmt:     "… +%d more (/queue list)",
	QueueRowsPaused:      "⏸ inbox paused (space to resume)",
	QueueTagFmt:          "✎ %d in inbox",
	QueueDeleteAgainFmt:  "press d again to delete #%s",
	QueueDeletedFmt:      "deleted #%s",
	QueueRetriedFmt:      "retry queued #%s",
	QueueSavedFmt:        "saved #%s",
	QueuePausedNote:      "inbox paused",
	QueueResumedNote:     "inbox resumed",
	QueueActionFailedFmt: "queue: %s",
	QueueGoneFmt:         "#%s already left the queue; the edit was not saved",
}

var queueZH = QueueMessages{
	QueueImageWait:       "含图片，本轮结束后发送",
	TUIQueueUnrecovered:  "待发送内容未知 —— 队列读不出来，已暂停等待检查",
	QueueRowsMoreFmt:     "… 还有 %d 条（/queue list）",
	QueueRowsPaused:      "⏸ 队列已暂停（按空格继续）",
	QueueTagFmt:          "✎ 队列中 %d 条",
	QueueDeleteAgainFmt:  "再按一次 d 删除 #%s",
	QueueDeletedFmt:      "已删除 #%s",
	QueueRetriedFmt:      "已重试 #%s",
	QueueSavedFmt:        "已保存 #%s",
	QueuePausedNote:      "队列已暂停",
	QueueResumedNote:     "队列已继续",
	QueueActionFailedFmt: "队列操作失败：%s",
	QueueGoneFmt:         "#%s 已经离开队列，改动没有保存",
}

var queueZHTW = QueueMessages{
	QueueImageWait:       "含圖片，本輪結束後發送",
	TUIQueueUnrecovered:  "待發送內容未知 —— 佇列讀不出來，已暫停等待檢查",
	QueueRowsMoreFmt:     "… 還有 %d 條（/queue list）",
	QueueRowsPaused:      "⏸ 佇列已暫停（按空白鍵繼續）",
	QueueTagFmt:          "✎ 佇列中 %d 條",
	QueueDeleteAgainFmt:  "再按一次 d 刪除 #%s",
	QueueDeletedFmt:      "已刪除 #%s",
	QueueRetriedFmt:      "已重試 #%s",
	QueueSavedFmt:        "已儲存 #%s",
	QueuePausedNote:      "佇列已暫停",
	QueueResumedNote:     "佇列已繼續",
	QueueActionFailedFmt: "佇列操作失敗：%s",
	QueueGoneFmt:         "#%s 已經離開佇列，改動沒有儲存",
}
