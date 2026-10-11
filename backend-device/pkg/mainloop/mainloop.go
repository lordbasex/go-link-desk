// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package mainloop serves the main thread when the host runs without a
// window. On macOS, ScreenCaptureKit answers its calls on the main
// thread's event loop, which the window (Fyne) serves otherwise: without
// it, listing the displays and starting a capture never finish.
package mainloop
