//go:build windows

package sandbox

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// grantMask is read, write, execute and delete without WRITE_DAC and
// WRITE_OWNER: the synthetic SID is the write bound, and it does not let a command
// rewrite the ACLs of the user's own files in a root.
const grantMask = windows.ACCESS_MASK(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.FILE_GENERIC_EXECUTE | 0x10000 | 0x40)

// WindowsGrantOps edits real DACLs. Granting and revoking propagate to the
// children of a root, which costs time proportional to the tree (about 0.7 s
// per 20,000 files measured), so the book grants a workspace once.
func WindowsGrantOps() grantOps { return daclOps{} }

type daclOps struct{}

func (daclOps) Grant(root, sid string) error {
	if err := refuseLink(root); err != nil {
		return err
	}
	return editDACL(root, sid, windows.GRANT_ACCESS)
}
func (daclOps) Revoke(root, sid string) error { return editDACL(root, sid, windows.REVOKE_ACCESS) }

// refuseLink: a root that is a junction or symbolic link names a tree somewhere
// else, and granting it would put the SID there.
func refuseLink(root string) error {
	p, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return fmt.Errorf("sandbox: %s: %w", root, err)
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("sandbox: %s is a link, not a folder to grant", root)
	}
	return nil
}

func editDACL(root, sidText string, mode windows.ACCESS_MODE) error {
	sid, err := windows.StringToSid(sidText)
	if err != nil {
		return fmt.Errorf("sandbox: parse SID: %w", err)
	}
	sd, err := windows.GetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("sandbox: read DACL of %s: %w", root, err)
	}
	old, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("sandbox: read DACL of %s: %w", root, err)
	}
	entry := windows.EXPLICIT_ACCESS{
		AccessPermissions: grantMask,
		AccessMode:        mode,
		Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee:           windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_UNKNOWN, TrusteeValue: windows.TrusteeValueFromSID(sid)},
	}
	next, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{entry}, old)
	if err != nil {
		return fmt.Errorf("sandbox: build DACL for %s: %w", root, err)
	}
	if err := windows.SetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, next, nil); err != nil {
		return fmt.Errorf("sandbox: write DACL of %s: %w", root, err)
	}
	return nil
}

// Granted reports whether root's DACL has an allow entry for the SID.
func (daclOps) Granted(root, sidText string) (bool, error) {
	sid, err := windows.StringToSid(sidText)
	if err != nil {
		return false, err
	}
	sd, err := windows.GetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, err
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return false, err
	}
	for i := range uint32(acl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			return false, err
		}
		if ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE && windows.EqualSid(sid, (*windows.SID)(unsafe.Pointer(&ace.SidStart))) {
			return true, nil
		}
	}
	return false, nil
}
