package testenv

import "golang.org/x/sys/windows"

func init() {
	removalRaceErrors = append(removalRaceErrors, windows.ERROR_SHARING_VIOLATION, windows.ERROR_ACCESS_DENIED)
}
