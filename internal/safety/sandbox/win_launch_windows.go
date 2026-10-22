//go:build windows

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The launcher is an entry of whichever host binary imports this package: it
// reads its request from argv, runs the command under the restricted token, and
// exits with the command's status. Nothing else in the process runs.
func init() {
	l, argv, ok := parseWinLaunch(os.Args)
	if !ok {
		return
	}
	if err := launcherMayRun(); err != nil {
		fmt.Fprintln(os.Stderr, "reasonix sandbox:", err)
		os.Exit(126)
	}
	if err := checkWinLaunch(l); err != nil {
		fmt.Fprintln(os.Stderr, "reasonix sandbox:", err)
		os.Exit(126)
	}
	code, err := runRestricted(l, argv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reasonix sandbox:", err)
		os.Exit(126)
	}
	os.Exit(code)
}

// launcherMayRun refuses a launcher that already runs under a restricted token.
// The host's own launcher is the one process that edits DACLs for a command; a
// command that starts the binary itself inherits its restricted token, so the
// token, which a command cannot change, tells the two apart and a forged request
// does nothing.
func launcherMayRun() error {
	restricted, err := windows.GetCurrentProcessToken().IsRestricted()
	if err != nil {
		return fmt.Errorf("%w: cannot read this process's token: %w", errLaunchRefused, err)
	}
	if restricted {
		return fmt.Errorf("%w: a restricted process may not start a launcher", errLaunchRefused)
	}
	return nil
}

// errNoLogonSID marks the session as one that cannot be restricted.
var errNoLogonSID = errors.New("no logon SID")

var procCreateRestrictedToken = windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateRestrictedToken")

const (
	disableMaxPrivilege = 0x1
	writeRestricted     = 0x8
)

// tokenDefaultDACL is TOKEN_DEFAULT_DACL.
type tokenDefaultDACL struct{ defaultDACL *windows.ACL }

type sidAndAttributes struct {
	sid        *windows.SID
	attributes uint32
}

// restrictedToken makes the child's token WRITE_RESTRICTED with every privilege
// but the traverse check removed (backup and restore bypass the access check),
// restricted to the synthetic SID, the logon SID and Everyone; only the roots
// carry the synthetic one. A delete or rename does not consult the restricting
// SIDs, so it is not bounded.
func restrictedToken(sidText string) (windows.Token, error) {
	var cur windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ALL_ACCESS, &cur); err != nil {
		return 0, fmt.Errorf("open process token: %w", err)
	}
	defer cur.Close()
	synthetic, err := windows.StringToSid(sidText)
	if err != nil {
		return 0, err
	}
	logon, err := logonSID(cur)
	if err != nil {
		return 0, err
	}
	everyone, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		return 0, err
	}
	system, _ := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	user, err := cur.GetTokenUser()
	if err != nil {
		return 0, err
	}
	list := []sidAndAttributes{{sid: synthetic}, {sid: logon}, {sid: everyone}}
	var out windows.Token
	r, _, e := procCreateRestrictedToken.Call(uintptr(cur), disableMaxPrivilege|writeRestricted, 0, 0, 0, 0, uintptr(len(list)), uintptr(unsafe.Pointer(&list[0])), uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return 0, fmt.Errorf("create restricted token: %w", e)
	}
	// Objects the child creates take this DACL. The synthetic SID passes the
	// restricted half of the access check and the user the normal half, so the child
	// can open what it made; SYSTEM keeps the service side able to read it.
	if err := setDefaultDACL(out, synthetic, system, user.User.Sid); err != nil {
		out.Close()
		return 0, err
	}
	return out, nil
}

func logonSID(tok windows.Token) (*windows.SID, error) {
	buf := make([]byte, 512)
	var n uint32
	if err := windows.GetTokenInformation(tok, windows.TokenLogonSid, &buf[0], uint32(len(buf)), &n); err != nil {
		return nil, fmt.Errorf("logon SID: %w", err)
	}
	groups := (*windows.Tokengroups)(unsafe.Pointer(&buf[0]))
	if groups.GroupCount == 0 {
		return nil, fmt.Errorf("logon SID: %w", errNoLogonSID)
	}
	return groups.AllGroups()[0].Sid.Copy()
}

func setDefaultDACL(tok windows.Token, sids ...*windows.SID) error {
	var entries []windows.EXPLICIT_ACCESS
	for _, s := range sids {
		if s == nil {
			continue
		}
		entries = append(entries, windows.EXPLICIT_ACCESS{
			AccessPermissions: windows.GENERIC_ALL, AccessMode: windows.GRANT_ACCESS, Inheritance: windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_UNKNOWN, TrusteeValue: windows.TrusteeValueFromSID(s)},
		})
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return fmt.Errorf("default DACL: %w", err)
	}
	info := tokenDefaultDACL{defaultDACL: acl}
	if err := windows.SetTokenInformation(tok, windows.TokenDefaultDacl, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return fmt.Errorf("set default DACL: %w", err)
	}
	return nil
}

// runRestricted starts argv suspended under the token, puts it in a job that
// ends the tree when the launcher does, resumes it, and returns its exit code.
// The command's standard handles are the launcher's own.
func runRestricted(l winLaunch, argv []string) (int, error) {
	// A session temp directory is new for each lease and not under any root, so the
	// launcher grants it here; it goes away with its lease, and so does the grant.
	for _, dir := range l.Grant {
		if err := WindowsGrantOps().Grant(dir, l.SID); err != nil {
			return 0, err
		}
	}
	tok, err := restrictedToken(l.SID)
	if err != nil {
		return 0, err
	}
	defer tok.Close()
	job, err := killOnCloseJob()
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(job)

	si := windows.StartupInfoEx{}
	si.Flags = windows.STARTF_USESTDHANDLES
	si.Cb = uint32(unsafe.Sizeof(si))
	var inherit []windows.Handle
	for i, id := range []uint32{windows.STD_INPUT_HANDLE, windows.STD_OUTPUT_HANDLE, windows.STD_ERROR_HANDLE} {
		h, err := windows.GetStdHandle(id)
		if err != nil || h == 0 || h == windows.InvalidHandle {
			continue
		}
		_ = windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT)
		if !slices.Contains(inherit, h) {
			inherit = append(inherit, h)
		}
		switch i {
		case 0:
			si.StdInput = h
		case 1:
			si.StdOutput = h
		default:
			si.StdErr = h
		}
	}
	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(argv))
	if err != nil {
		return 0, err
	}
	// The command inherits this process's environment, so the host's overrides are
	// set here rather than merged into a copy.
	for k, v := range l.Env {
		if err := os.Setenv(k, v); err != nil {
			return 0, err
		}
	}
	// Only the standard handles cross into the child: an inherited handle is not
	// checked against the restricted token, so everything else stays behind.
	flags := uint32(windows.CREATE_SUSPENDED | windows.CREATE_UNICODE_ENVIRONMENT)
	if len(inherit) > 0 {
		attrs, err := windows.NewProcThreadAttributeList(1)
		if err != nil {
			return 0, fmt.Errorf("handle list: %w", err)
		}
		defer attrs.Delete()
		if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&inherit[0]), uintptr(len(inherit))*unsafe.Sizeof(inherit[0])); err != nil {
			return 0, fmt.Errorf("handle list: %w", err)
		}
		si.ProcThreadAttributeList = attrs.List()
		flags |= windows.EXTENDED_STARTUPINFO_PRESENT
	}
	var pi windows.ProcessInformation
	if err := windows.CreateProcessAsUser(tok, nil, cmdline, nil, nil, len(inherit) > 0, flags, nil, nil, &si.StartupInfo, &pi); err != nil {
		return 0, fmt.Errorf("start %s: %w", argv[0], err)
	}
	defer windows.CloseHandle(pi.Process)
	defer windows.CloseHandle(pi.Thread)
	if err := windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		_ = windows.TerminateProcess(pi.Process, 1)
		return 0, fmt.Errorf("assign to job: %w", err)
	}
	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		_ = windows.TerminateProcess(pi.Process, 1)
		return 0, fmt.Errorf("resume: %w", err)
	}
	if _, err := windows.WaitForSingleObject(pi.Process, windows.INFINITE); err != nil {
		return 0, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(pi.Process, &code); err != nil {
		return 0, err
	}
	return int(code), nil
}

func killOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("create job: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE}}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return 0, fmt.Errorf("limit job: %w", err)
	}
	// The child cannot reach windows, clipboard, atoms or display settings of the
	// user's session that live outside the job.
	ui := windows.JOBOBJECT_BASIC_UI_RESTRICTIONS{UIRestrictionsClass: windows.JOB_OBJECT_UILIMIT_HANDLES | windows.JOB_OBJECT_UILIMIT_READCLIPBOARD | windows.JOB_OBJECT_UILIMIT_WRITECLIPBOARD | windows.JOB_OBJECT_UILIMIT_SYSTEMPARAMETERS | windows.JOB_OBJECT_UILIMIT_DISPLAYSETTINGS | windows.JOB_OBJECT_UILIMIT_GLOBALATOMS | windows.JOB_OBJECT_UILIMIT_DESKTOP | windows.JOB_OBJECT_UILIMIT_EXITWINDOWS}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectBasicUIRestrictions, uintptr(unsafe.Pointer(&ui)), uint32(unsafe.Sizeof(ui))); err != nil {
		windows.CloseHandle(job)
		return 0, fmt.Errorf("limit job UI: %w", err)
	}
	return job, nil
}
