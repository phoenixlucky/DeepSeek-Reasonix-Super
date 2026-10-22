package i18n

var settingEN = SettingMessages{
	SettingApprovalFmt:     "The assistant asks to change the setting %[1]s from %[2]s to %[3]s. %[4]s",
	SettingReasonFmt:       "Its reason: %s",
	SettingChangedFmt:      "Setting %[1]s changed from %[2]s to %[3]s. %[4]s",
	SettingOpenFmt:         "The assistant needs %s changed, which only you can do.",
	SettingEffectImmediate: "Takes effect right away.",
	SettingEffectNextTurn:  "Takes effect from the next turn.",
	SettingEffectOnOpen:    "Takes effect in sessions opened afterwards.",
	SettingEffectRestart:   "Takes effect after a restart.",
	EffortUndeclaredFmt:    "%s declares no reasoning levels for this model. List the levels its provider documents as supported_efforts = [\"...\"] on its [[providers]] entry to configure them",
}

var settingZH = SettingMessages{
	SettingApprovalFmt:     "助手请求把设置 %[1]s 从 %[2]s 改为 %[3]s。%[4]s",
	SettingReasonFmt:       "它给出的理由：%s",
	SettingChangedFmt:      "设置 %[1]s 已从 %[2]s 改为 %[3]s。%[4]s",
	SettingOpenFmt:         "助手需要改动 %s，只有你本人能改。",
	SettingEffectImmediate: "立即生效。",
	SettingEffectNextTurn:  "从下一回合起生效。",
	SettingEffectOnOpen:    "之后新打开的会话生效。",
	SettingEffectRestart:   "重启后生效。",
	EffortUndeclaredFmt:    "%s 没有声明这个模型的推理档位。把服务商文档里的档位写进它的 [[providers]] 条目：supported_efforts = [\"...\"]，就能配置",
}

var settingZHTW = SettingMessages{
	SettingApprovalFmt:     "助手請求把設定 %[1]s 從 %[2]s 改為 %[3]s。%[4]s",
	SettingReasonFmt:       "它給出的理由：%s",
	SettingChangedFmt:      "設定 %[1]s 已從 %[2]s 改為 %[3]s。%[4]s",
	SettingOpenFmt:         "助手需要改動 %s，只有你本人能改。",
	SettingEffectImmediate: "立即生效。",
	SettingEffectNextTurn:  "從下一回合起生效。",
	SettingEffectOnOpen:    "之後新開啟的工作階段生效。",
	SettingEffectRestart:   "重新啟動後生效。",
	EffortUndeclaredFmt:    "%s 沒有宣告這個模型的推理檔位。把服務商文件裡的檔位寫進它的 [[providers]] 條目：supported_efforts = [\"...\"]，就能設定",
}
