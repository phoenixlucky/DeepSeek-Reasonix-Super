package sandbox

import "path/filepath"

// cacheDirsFor are the per-user tool caches a build writes, for a user whose
// local app data is local and whose profile is home. Every entry is a data cache:
// a granted root is labelled Low, and anything run from a Low-labelled file
// starts at Low integrity even outside the sandbox, so no directory that holds
// programs a person runs (a bin folder, global tool installs) may be listed.
func cacheDirsFor(local, home string) []string {
	var dirs []string
	add := func(base string, sub ...string) {
		if base != "" {
			dirs = append(dirs, filepath.Join(append([]string{base}, sub...)...))
		}
	}
	add(local, "go-build")
	add(home, "go", "pkg", "mod")
	add(local, "npm-cache")
	add(local, "pip", "Cache")
	add(home, ".cargo", "registry")
	add(home, ".cargo", "git")
	add(home, ".nuget", "packages")
	return dirs
}

// executableHomesFor are the directories whose contents a person runs.
func executableHomesFor(local, home string) []string {
	var dirs []string
	add := func(base string, sub ...string) {
		if base != "" {
			dirs = append(dirs, filepath.Join(append([]string{base}, sub...)...))
		}
	}
	add(home, "go", "bin")
	add(home, ".cargo", "bin")
	add(home, ".dotnet", "tools")
	add(home, "AppData", "Roaming", "npm")
	add(local, "Microsoft", "WindowsApps")
	add(local, "Programs")
	return dirs
}
