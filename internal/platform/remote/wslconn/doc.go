// Package wslconn lets the kernel bootstrap treat a WSL distribution as a
// machine it can run commands on and keep files in, the way it does an SSH host.
// Commands go through `wsl.exe --exec /bin/sh -c`, and file operations are
// small fixed POSIX scripts that take every path as a positional argument, so
// no path is ever spliced into script text. A missing path is reported by an
// exit status this package defines, never by reading a tool's message.
package wslconn
