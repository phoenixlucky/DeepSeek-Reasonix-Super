package i18n

// ResumePickerMessages is the text of the terminal's interactive resume picker, including
// deleting a conversation from it; Messages embeds it.
type ResumePickerMessages struct {
	ResumePickTitle       string // header in the interactive resume picker
	ResumePickHint        string // keyboard hint in the interactive resume picker
	ResumePickFilter      string // typing narrows the list
	ResumePickSearch      string // label before the typed query
	ResumePickNoMatch     string // the query matches no session
	ResumeDeleteAskFmt    string // confirm deleting a conversation — %s its title
	ResumeDeleteFailedFmt string // a delete was refused — %s the reason
}
