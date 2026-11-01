Captures of go-link's real windows, to compare against: Fyne's offscreen test driver draws the real code.
Copy zz_ref_test.go.txt to backend-device/internal/gui/zz_ref_test.go and zz_ref_fyneview.go.txt to
backend-device/pkg/fyneview/zz_ref.go, then, from backend-device:
    REF_OUT=<folder> go test ./internal/gui -run TestRefAll
Remove both afterwards: they are not part of go-link. Never committed under pkg/ or internal/.
The session toolbar, overlay and terminal only exist in Cocoa (pkg/remoteview), so they have no capture.
tools/harness/: test theme, driver and compare tool for an isolated headless LordOS session.
