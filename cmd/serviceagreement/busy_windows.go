//go:build windows

package main

import (
    "github.com/lxn/walk"
    "github.com/lxn/win"
)

// beginSQLWait shows the real system wait cursor immediately, even when the
// subsequent synchronous SQL request blocks the UI message loop. The returned
// function restores both Win32 cursor and Walk window cursor. Call it before
// every database operation and restore before displaying errors or dialogs.
func beginSQLWait(owner walk.Form) func() {
    if owner == nil {
        return func() {}
    }
    former := owner.Cursor()
    owner.SetCursor(walk.CursorWait())
    oldSystem := win.SetCursor(win.LoadCursor(0, win.MAKEINTRESOURCE(win.IDC_WAIT)))
    return func() {
        owner.SetCursor(former)
        if oldSystem != 0 {
            win.SetCursor(oldSystem)
        } else {
            win.SetCursor(win.LoadCursor(0, win.MAKEINTRESOURCE(win.IDC_ARROW)))
        }
    }
}
