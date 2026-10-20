//go:build windows

package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/Postnn/ServiceAgreement/internal/core"
	"github.com/lxn/walk"
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

	form, err := walk.NewMainWindow()
	if err != nil {
		t.Fatalf("create test window: %v", err)
	}
	defer form.Dispose()
	widget, err := walk.NewTextEdit(form)
	if err != nil {
		t.Fatalf("create native TextEdit: %v", err)
	}
	defer widget.Dispose()

	query := core.FormatSQL("SELECT a, b FROM dbo.[ДокументыДанные] WHERE a = @id ORDER BY a DESC")
	if err := widget.SetText(sqlTextForEditor(query)); err != nil {
		t.Fatalf("populate TextEdit: %v", err)
	}
	lineCount := int(widget.SendMessage(win.EM_GETLINECOUNT, 0, 0))
	if lineCount < 4 {
		t.Fatalf("native TextEdit reports %d lines instead of multiple formatted lines; text=%q", lineCount, widget.Text())
	}
	if after := sqlTextFromEditor(widget.Text()); after != query {
		t.Fatalf("native control changed SQL text: got %q, want %q", after, query)
	}
}
