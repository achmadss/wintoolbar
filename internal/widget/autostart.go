package widget

import (
	"os"
	"strings"

	"wintoolbar/internal/win"
)

// Autostart uses the per-user Run key. Windows lists it under Settings >
// Apps > Startup and in Task Manager, so it can be toggled there too; the
// checkbox reads the key each time the settings window opens.
const (
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue = "wintoolbar"
	// Windows keeps the Startup-apps on/off state here, separate from Run.
	approvedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
)

func exePath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}

// autostartEnabled reports whether the Run entry points at this executable
// and Windows has not disabled it in Startup apps.
func autostartEnabled() bool {
	v, ok := win.RegString(runKey, runValue)
	if !ok || !strings.EqualFold(strings.Trim(v, `"`), exePath()) {
		return false
	}
	var state [12]byte
	if n, ok := win.RegBinary(approvedKey, runValue, state[:]); ok && n > 0 {
		return state[0]&1 == 0 // 2 = enabled, 3 = disabled by the user
	}
	return true
}

func setAutostart(on bool) error {
	// Clear a "disabled" mark from Startup apps either way, so the checkbox
	// and Windows agree.
	_ = win.RegDeleteValue(approvedKey, runValue)
	if !on {
		return win.RegDeleteValue(runKey, runValue)
	}
	return win.RegSetString(runKey, runValue, `"`+exePath()+`"`)
}
