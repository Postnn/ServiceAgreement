//go:build windows

package main

import (
	"runtime"
	"strings"
	"testing"

	"syscall"
	"unsafe"

	"github.com/Postnn/ServiceAgreement/internal/core"
	"github.com/lxn/win"
)

func TestSQLTextEditorLineEndingConversion(t *testing.T) {
	original := "SELECT\ta,\tb\nFROM dbo.[ДокументыДанные]\nWHERE code = N'текст'"
	got := sqlTextForEditor(original)
	if strings.Count(got, "\r\n") != 2 || strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Fatalf("Win32 TextEdit requires CRLF line breaks, got %q", got)
	}
	if got != sqlTextForEditor(got) {
		t.Fatal("editor conversion is not idempotent")
	}
	if back := sqlTextFromEditor(got); back != original {
		t.Fatalf("SQL text was changed on round trip: %q", back)
	}
	for _, source := range []string{"SELECT 1\nFROM t", "SELECT 1\r\nFROM t", "SELECT 1\rFROM t"} {
		if back := sqlTextFromEditor(sqlTextForEditor(source)); back != "SELECT 1\nFROM t" {
			t.Fatalf("mixed newline convention was not normalized: %q", back)
		}
	}
}

func TestNativeTextEditRendersFormattedSQLOnMultipleLines(t *testing.T) {
	// Verify the exact Win32 control used by the settings dialog, not just
	// strings. The control's EM_GETLINECOUNT reports visual line breaks.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Walk's TextEdit delegates to the system EDIT window via WM_SETTEXT.
	// Test the same control directly: a full Walk form requires tooltip
	// registration unavailable in non-interactive GitHub Actions sessions.
	hwnd := win.CreateWindowEx(0, syscall.StringToUTF16Ptr("EDIT"), syscall.StringToUTF16Ptr(""),
		win.WS_OVERLAPPED|win.ES_MULTILINE|win.ES_WANTRETURN,
		0, 0, 640, 480, 0, 0, win.GetModuleHandle(nil), nil)
	if hwnd == 0 {
		t.Fatalf("CreateWindowEx EDIT failed: %v", syscall.GetLastError())
	}
	defer win.DestroyWindow(hwnd)

	query := core.FormatSQL("SELECT a, b FROM dbo.[ДокументыДанные] WHERE a = @id ORDER BY a DESC")
	value := syscall.StringToUTF16Ptr(sqlTextForEditor(query))
	if win.SendMessage(hwnd, win.WM_SETTEXT, 0, uintptr(unsafe.Pointer(value))) == 0 {
		t.Fatalf("WM_SETTEXT failed: %v", syscall.GetLastError())
	}
	lineCount := int(win.SendMessage(hwnd, win.EM_GETLINECOUNT, 0, 0))
	if lineCount < 4 {
		t.Fatalf("native EDIT reports %d lines instead of multiple formatted lines", lineCount)
	}
	textLength := int(win.SendMessage(hwnd, win.WM_GETTEXTLENGTH, 0, 0))
	buf := make([]uint16, textLength+1)
	win.SendMessage(hwnd, win.WM_GETTEXT, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	if after := sqlTextFromEditor(syscall.UTF16ToString(buf)); after != query {
		t.Fatalf("native control changed SQL text: got %q, want %q", after, query)
	}
}
