package browser

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

var lockLockedIcon = theme.NewThemedResource(
	fyne.NewStaticResource("lock-locked.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"><path fill="#000000" d="M18 8h-1V6c0-2.76-2.24-5-5-5S7 3.24 7 6v2H6c-1.1 0-2 .9-2 2v10c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V10c0-1.1-.9-2-2-2zM12 17c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2zM15.1 8H8.9V6c0-1.71 1.39-3.1 3.1-3.1s3.1 1.39 3.1 3.1v2z"/></svg>`)),
)

var lockUnlockedIcon = theme.NewThemedResource(
	fyne.NewStaticResource("lock-unlocked.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"><path fill="#000000" d="M12 1C9.24 1 7 3.24 7 6v4H6c-1.1 0-2 .9-2 2v10c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V12c0-1.1-.9-2-2-2h-1V6h-2v4H9V6c0-1.66 1.34-3 3-3s3 1.34 3 3v1h2V6c0-2.76-2.24-5-5-5zm0 13c1.1 0 2 .9 2 2s-.9 2-2 2-2-.9-2-2 .9-2 2-2z"/></svg>`)),
)
