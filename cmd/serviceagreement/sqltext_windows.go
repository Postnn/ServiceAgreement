//go:build windows

package main

import "strings"

// Win32 EDIT controls (used by Walk's TextEdit) require CRLF to display
// explicit line breaks when their text is set through WM_SETTEXT.
// Keep JSON/SQL in normalized LF form; translate only at the UI boundary.
func sqlTextForEditor(text string) string {
	return strings.ReplaceAll(sqlTextFromEditor(text), "\n", "\r\n")
}

// Windows may return CRLF from WM_GETTEXT. Never persist these platform
// line endings to the SQL configuration, and never lose literal tabs.
func sqlTextFromEditor(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}
