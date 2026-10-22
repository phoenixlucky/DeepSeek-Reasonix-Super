// Package opendoc opens one file with the operating system's own default
// program, the way a double click would: an HTML page in the browser, a PDF in
// the reader.
//
// It starts only the platform's opener (open, rundll32's FileProtocolHandler,
// xdg-open), with the path as a single argument of an argument list and never
// through a shell. It decides nothing about whether a file may be opened: the
// caller has already confined and typed it, and refusing a relative path or
// anything that is not an absolute path here is only the last guard against a
// value that could be read as an option or a URL.
package opendoc
