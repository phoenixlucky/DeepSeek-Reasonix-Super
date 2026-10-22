package sandbox

import "errors"

// Why the Windows sandbox could not be prepared. Each is an identity a caller
// branches on and a frontend says in its own words; none is a sentence to match.
var (
	// ErrWinNotPrepared: enforcement was asked for and nothing prepared the launch.
	ErrWinNotPrepared = errors.New("sandbox: the Windows sandbox was not prepared for this launch")
	// ErrWinGrantUnreadable: the grant record exists and cannot be trusted.
	ErrWinGrantUnreadable = errors.New("sandbox: the Windows grant record is unreadable")
	// ErrWinGrantDenied: Windows refused to change a root's permissions.
	ErrWinGrantDenied = errors.New("sandbox: Windows refused to change the permissions of a root")
	// ErrWinNoLogonSID: the session has no interactive logon, so no token can be restricted.
	ErrWinNoLogonSID = errors.New("sandbox: this Windows session has no logon SID to restrict with")
	// ErrWinRecordExposed: the grant record sits inside a folder the sandbox may
	// write, so a command could rewrite what the host later grants.
	ErrWinRecordExposed = errors.New("sandbox: the grant record is inside a folder the sandbox may write")
	// ErrWinLauncher: the restricted token or the job could not be made.
	ErrWinLauncher = errors.New("sandbox: the restricted token could not be made")
)

func windowsUnavailableCode(fault error) string {
	switch {
	case errors.Is(fault, ErrWinGrantUnreadable):
		return "sandbox.win_grant_unreadable"
	case errors.Is(fault, ErrWinGrantDenied):
		return "sandbox.win_grant_denied"
	case errors.Is(fault, ErrWinNoLogonSID):
		return "sandbox.win_no_logon_sid"
	case errors.Is(fault, ErrWinRecordExposed):
		return "sandbox.win_record_exposed"
	case errors.Is(fault, ErrWinLauncher):
		return "sandbox.win_launcher"
	case errors.Is(fault, ErrWinNotPrepared):
		return "sandbox.win_not_prepared"
	}
	return "sandbox.unsupported_on_windows"
}

func windowsRemediation(fault error) string {
	switch {
	case errors.Is(fault, ErrWinGrantUnreadable):
		return "The Windows sandbox's record of the folders it has granted is damaged, so it will not start over silently. Delete `windows-grants.json` in the `sandbox` folder of the Reasonix state folder to let it start over (permission entries it left stay on your folders, inert), or set [sandbox] windows_integrity = false."
	case errors.Is(fault, ErrWinGrantDenied):
		return "Windows refused to change the permissions of a folder the sandbox needs to write to. Check that you own the workspace folder, or set [sandbox] windows_integrity = false; shell commands then run unconfined."
	case errors.Is(fault, ErrWinNoLogonSID):
		return "The Windows sandbox needs an interactive desktop session; this one (for example an SSH or service session) has no logon identity to restrict. Run Reasonix from the desktop, or set [sandbox] windows_integrity = false; shell commands then run unconfined."
	case errors.Is(fault, ErrWinRecordExposed):
		return "The Windows sandbox keeps a record of what it has granted, and that record lies inside a folder the sandbox may write (for example the workspace is your whole user folder). A command could then rewrite what the host grants next. Open a narrower folder, or set [sandbox] windows_integrity = false."
	case errors.Is(fault, ErrWinLauncher):
		return "Windows would not create the restricted token the sandbox runs commands under. Set [sandbox] windows_integrity = false to run shell commands unconfined."
	case errors.Is(fault, ErrWinNotPrepared):
		return "The Windows sandbox is on but this launch was not prepared for it. Set [sandbox] windows_integrity = false to run shell commands unconfined."
	}
	return "Windows enforces the bash sandbox only when [sandbox] windows_integrity = true; otherwise shell commands run unconfined."
}
