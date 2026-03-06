// Inlined from github.com/knz/bubbline/editline

//go:build windows
// +build windows

package editline

var canSuspendProcess = false

func suspendProcess() {}
