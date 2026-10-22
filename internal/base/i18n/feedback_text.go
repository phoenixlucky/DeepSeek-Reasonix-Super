package i18n

// FeedbackText is what /feedback says about the install's standing and about a
// refusal that names its window. Level names are keyed by the level number the
// service reports, limit names by the window identifier it reports; "" in
type FeedbackText struct {
	LevelNames          map[string]string
	LevelNextFmt        string // level, name, shipped, still needed, next name
	LevelTopFmt         string // level, name, shipped
	LimitsFmt           string // reports an hour, reports a day, replies an hour
	StandingStale       string // appended while the service is unreachable
	TrustLegacyFmt      string // the date the higher limits end
	TrustLapsed         string
	LimitNames          map[string]string
	LimitUnknown        string
	RefusedResetsFmt    string // limit name, local reset time
	RefusedLaterFmt     string // limit name
	RefusedPermanentFmt string // limit name

	LinkExplain         string
	LinkDone            string
	LinkBoardHint       string
	UnlinkExplain       string
	UnlinkDone          string
	LinksHeader         string
	LinksNone           string
	LinksRowFmt         string // short id, this-device mark, linked date
	ThisDevice          string
	NeedSignIn          string
	LinkConflict        string
	LinkLimit           string
	NoInstall           string
	OtherDeviceMark     string
	LevelUpFmt          string // level, name, previous level
	BoardHeaderFmt      string // view label
	BoardRowFmt         string // place (or blank), name, credits
	BoardRowReleasesFmt string // the releases a person shipped in
	BoardNone           string
	BoardOff            string
	BoardAllView        string
	BoardReleaseFmt     string // release
	BoardJoinExplainFmt string // the name that would be shown
	BoardJoinDone       string
	BoardLeaveDone      string
	BoardMeListedFmt    string // name
	BoardMeNot          string
	BoardNameInvalid    string
	BoardNameReserved   string
	BoardNameTaken      string
	BoardDelisted       string
	BoardNoLink         string
}

var feedbackEnglish = FeedbackText{
	LevelNames: map[string]string{
		"0": "New seed", "1": "Sprout", "2": "Seedling", "3": "Sapling", "4": "Bloom", "5": "Canopy", "6": "Grove",
	},
	LevelNextFmt:   "Level %d, %s: %d shipped, %d more to reach %s.",
	LevelTopFmt:    "Level %d, %s: %d shipped, the highest level.",
	LimitsFmt:      "Limits now: %d reports an hour, %d a day, %d replies an hour.",
	StandingStale:  "(last confirmed - the service is unreachable)",
	TrustLegacyFmt: "Your current higher limits stay until %s.",
	TrustLapsed:    "Your higher limits have lapsed; your level and count are kept, and a later shipped outcome renews them.",
	LimitNames: map[string]string{
		"ip_hourly":      "the hourly limit for this network",
		"install_hourly": "the hourly report limit",
		"install_daily":  "the daily report limit",
		"reply_hourly":   "the hourly reply limit",
		"reply_item":     "the reply limit for this report",
		"global_daily":   "the service's daily capacity",
		"global_burst":   "the service's short-term capacity",
	},
	LimitUnknown:        "a limit",
	RefusedResetsFmt:    "not sent: %s was reached. It resets at %s.",
	RefusedLaterFmt:     "not sent: %s was reached. Try again later.",
	RefusedPermanentFmt: "not sent: %s was reached; it does not reset.",
	LinkExplain:         "Linking ties this device to your signed-in account: its reports and the replies on them also show on your other linked devices, and theirs show here. You can unlink at any time.\nNothing was sent. To link, run the same line again with --yes.",
	LinkDone:            "Linked - reports from your other linked devices now appear in /feedback list.",
	LinkBoardHint:       "You can also join the public contribution board under a name you choose: /feedback board join <name>. Nothing is listed until you run it.",
	UnlinkExplain:       "Unlinking stops this device showing, and being shown on, your other devices. Reports, level and limits do not change.\nNothing was sent. To unlink, run the same line again with --yes.",
	UnlinkDone:          "Unlinked - this device now lists only its own reports.",
	LinksHeader:         "Devices linked to your account:",
	LinksNone:           "No device is linked to your account.",
	LinksRowFmt:         "  %s%s  linked %s",
	ThisDevice:          " (this device)",
	NeedSignIn:          "Sign in first with `reasonix login`, then run this again.",
	LinkConflict:        "This device is already linked to another account; unlink it from that account first.",
	LinkLimit:           "Your account is at its limit of linked devices; unlink one first.",
	NoInstall:           "Send a report from this device first: a device with no report has nothing to link.",
	OtherDeviceMark:     "  [other device]",
	LevelUpFmt:          "Level up: level %d, %s — up from %d.",
	BoardHeaderFmt:      "Contribution board, %s:",
	BoardRowFmt:         "  %s %s - %d adopted%s",
	BoardRowReleasesFmt: " (%s)",
	BoardNone:           "Nobody is listed here yet.",
	BoardOff:            "The service has no contribution board.",
	BoardAllView:        "all releases, by name",
	BoardReleaseFmt:     "release %s, ranked",
	BoardJoinExplainFmt: "Joining shows the name \"%s\" on the public contribution board with how many of your suggestions shipped, and nothing else. You can leave at any time. Nothing was sent: run the same line again with --yes.",
	BoardJoinDone:       "You are on the board.",
	BoardLeaveDone:      "You are off the board.",
	BoardMeListedFmt:    "You are listed as \"%s\". Leave with /feedback board leave.",
	BoardMeNot:          "You are not on the board. Join with /feedback board join <name>.",
	BoardNameInvalid:    "That name is not allowed: use 2 to 30 letters, digits, spaces, underscores or hyphens; no @ and no links.",
	BoardNameReserved:   "That name is reserved because it could be mistaken for a maintainer; pick another.",
	BoardNameTaken:      "Someone already uses that name.",
	BoardDelisted:       "A maintainer removed this account from the board; it cannot join again for now.",
	BoardNoLink:         "Link this device to your account first: /feedback link.",
}

var feedbackChinese = FeedbackText{
	LevelNames: map[string]string{
		"0": "新种", "1": "萌芽", "2": "幼苗", "3": "小树", "4": "繁花", "5": "成荫", "6": "共林",
	},
	LevelNextFmt:   "等级 %d「%s」：已落地 %d 条，再落地 %d 条升到「%s」。",
	LevelTopFmt:    "等级 %d「%s」：已落地 %d 条，已是最高等级。",
	LimitsFmt:      "当前额度：每小时 %d 条反馈、每天 %d 条、每小时 %d 条回复。",
	StandingStale:  "（上次确认的结果，反馈服务暂时连不上）",
	TrustLegacyFmt: "你现在较高的额度保留到 %s。",
	TrustLapsed:    "你的较高额度已经失效；等级和计数都保留，之后再有反馈落地就会续上。",
	LimitNames: map[string]string{
		"ip_hourly":      "这个网络的每小时上限",
		"install_hourly": "每小时反馈次数上限",
		"install_daily":  "每日反馈次数上限",
		"reply_hourly":   "每小时回复次数上限",
		"reply_item":     "这份反馈的回复次数上限",
		"global_daily":   "服务今天的总接收量",
		"global_burst":   "服务短时间内的接收量",
	},
	LimitUnknown:        "某个上限",
	RefusedResetsFmt:    "未发送：已达到%s，将在 %s 重置。",
	RefusedLaterFmt:     "未发送：已达到%s，请稍后再试。",
	RefusedPermanentFmt: "未发送：已达到%s，它不会自动重置。",
	LinkExplain:         "关联会把这台设备绑定到你已登录的账号：它的反馈和回复也会出现在你其他已关联的设备上，别的设备上的也会出现在这里。随时可以取消关联。\n什么都没有发出。要关联，请在同一行后面加 --yes 再运行一次。",
	LinkDone:            "已关联——你其他已关联设备上的反馈现在会出现在 /feedback list 里。",
	LinkBoardHint:       "也可以用你自己选的名字加入公开的贡献榜：/feedback board join <名字>。不运行这条就不会上榜。",
	UnlinkExplain:       "取消关联后，这台设备不再显示你其他设备的反馈，也不再被它们显示。反馈、等级和额度都不变。\n什么都没有发出。要取消关联，请在同一行后面加 --yes 再运行一次。",
	UnlinkDone:          "已取消关联——这台设备现在只列出自己的反馈。",
	LinksHeader:         "已关联到你账号的设备：",
	LinksNone:           "没有设备关联到你的账号。",
	LinksRowFmt:         "  %s%s  关联于 %s",
	ThisDevice:          "（这台设备）",
	NeedSignIn:          "请先用 `reasonix login` 登录，再运行一次。",
	LinkConflict:        "这台设备已经关联到另一个账号，请先在那个账号下取消关联。",
	LinkLimit:           "你的账号关联的设备已达上限，请先取消一台。",
	NoInstall:           "请先从这台设备发出一条反馈：没有发过反馈的设备没有可关联的内容。",
	OtherDeviceMark:     "  [其他设备]",
	LevelUpFmt:          "等级提升：现在是等级 %d「%s」（原为 %d）。",
	BoardHeaderFmt:      "贡献榜，%s：",
	BoardRowFmt:         "  %s %s - %d 条被采纳%s",
	BoardRowReleasesFmt: "（%s）",
	BoardNone:           "这里还没有人上榜。",
	BoardOff:            "服务端没有贡献榜。",
	BoardAllView:        "全部版本，按名字排序",
	BoardReleaseFmt:     "版本 %s，有名次",
	BoardJoinExplainFmt: "加入后，公开的贡献榜上会显示名字「%s」和你被采纳的条数，不显示别的。随时可以退出。还没有发送：用同样的内容再加 --yes 运行一次。",
	BoardJoinDone:       "已加入贡献榜。",
	BoardLeaveDone:      "已退出贡献榜。",
	BoardMeListedFmt:    "你在榜上，名字是「%s」。用 /feedback board leave 退出。",
	BoardMeNot:          "你不在榜上。用 /feedback board join <名字> 加入。",
	BoardNameInvalid:    "这个名字不能用：2 到 30 个字符，字母、数字、空格、下划线或连字符；不能含 @ 或链接。",
	BoardNameReserved:   "这个名字是保留的，会让人误以为是维护者，换一个。",
	BoardNameTaken:      "已经有人用这个名字了。",
	BoardDelisted:       "维护者已把这个账号从榜单移除，暂时不能再加入。",
	BoardNoLink:         "请先把这台设备关联到账号：/feedback link。",
}

var feedbackTraditional = FeedbackText{
	LevelNames: map[string]string{
		"0": "新種", "1": "萌芽", "2": "幼苗", "3": "小樹", "4": "繁花", "5": "成蔭", "6": "共林",
	},
	LevelNextFmt:   "等級 %d「%s」：已落地 %d 條，再落地 %d 條升到「%s」。",
	LevelTopFmt:    "等級 %d「%s」：已落地 %d 條，已是最高等級。",
	LimitsFmt:      "目前額度：每小時 %d 條回饋、每天 %d 條、每小時 %d 條回覆。",
	StandingStale:  "（上次確認的結果，回饋服務暫時連不上）",
	TrustLegacyFmt: "你現在較高的額度保留到 %s。",
	TrustLapsed:    "你的較高額度已經失效；等級和計數都保留，之後再有回饋落地就會續上。",
	LimitNames: map[string]string{
		"ip_hourly":      "這個網路的每小時上限",
		"install_hourly": "每小時回饋次數上限",
		"install_daily":  "每日回饋次數上限",
		"reply_hourly":   "每小時回覆次數上限",
		"reply_item":     "這份回饋的回覆次數上限",
		"global_daily":   "服務今天的總接收量",
		"global_burst":   "服務短時間內的接收量",
	},
	LimitUnknown:        "某個上限",
	RefusedResetsFmt:    "未傳送：已達到%s，將在 %s 重置。",
	RefusedLaterFmt:     "未傳送：已達到%s，請稍後再試。",
	RefusedPermanentFmt: "未傳送：已達到%s，它不會自動重置。",
	LinkExplain:         "關聯會把這台裝置綁定到你已登入的帳號：它的回饋和回覆也會出現在你其他已關聯的裝置上，別的裝置上的也會出現在這裡。隨時可以取消關聯。\n什麼都沒有送出。要關聯，請在同一行後面加 --yes 再執行一次。",
	LinkDone:            "已關聯——你其他已關聯裝置上的回饋現在會出現在 /feedback list 裡。",
	LinkBoardHint:       "也可以用你自己選的名字加入公開的貢獻榜：/feedback board join <名字>。不執行這條就不會上榜。",
	UnlinkExplain:       "取消關聯後，這台裝置不再顯示你其他裝置的回饋，也不再被它們顯示。回饋、等級和額度都不變。\n什麼都沒有送出。要取消關聯，請在同一行後面加 --yes 再執行一次。",
	UnlinkDone:          "已取消關聯——這台裝置現在只列出自己的回饋。",
	LinksHeader:         "已關聯到你帳號的裝置：",
	LinksNone:           "沒有裝置關聯到你的帳號。",
	LinksRowFmt:         "  %s%s  關聯於 %s",
	ThisDevice:          "（這台裝置）",
	NeedSignIn:          "請先用 `reasonix login` 登入，再執行一次。",
	LinkConflict:        "這台裝置已經關聯到另一個帳號，請先在那個帳號下取消關聯。",
	LinkLimit:           "你的帳號關聯的裝置已達上限，請先取消一台。",
	NoInstall:           "請先從這台裝置送出一則回饋：沒有送過回饋的裝置沒有可關聯的內容。",
	OtherDeviceMark:     "  [其他裝置]",
	LevelUpFmt:          "等級提升：現在是等級 %d「%s」（原為 %d）。",
	BoardHeaderFmt:      "貢獻榜，%s：",
	BoardRowFmt:         "  %s %s - %d 條被採納%s",
	BoardRowReleasesFmt: "（%s）",
	BoardNone:           "這裡還沒有人上榜。",
	BoardOff:            "服務端沒有貢獻榜。",
	BoardAllView:        "全部版本，按名字排序",
	BoardReleaseFmt:     "版本 %s，有名次",
	BoardJoinExplainFmt: "加入後，公開的貢獻榜上會顯示名字「%s」和你被採納的條數，不顯示別的。隨時可以退出。還沒有送出：用同樣的內容再加 --yes 執行一次。",
	BoardJoinDone:       "已加入貢獻榜。",
	BoardLeaveDone:      "已退出貢獻榜。",
	BoardMeListedFmt:    "你在榜上，名字是「%s」。用 /feedback board leave 退出。",
	BoardMeNot:          "你不在榜上。用 /feedback board join <名字> 加入。",
	BoardNameInvalid:    "這個名字不能用：2 到 30 個字元，字母、數字、空格、底線或連字號；不能含 @ 或連結。",
	BoardNameReserved:   "這個名字是保留的，會讓人誤以為是維護者，換一個。",
	BoardNameTaken:      "已經有人用這個名字了。",
	BoardDelisted:       "維護者已把這個帳號從榜單移除，暫時不能再加入。",
	BoardNoLink:         "請先把這台裝置關聯到帳號：/feedback link。",
}
