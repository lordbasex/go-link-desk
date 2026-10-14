// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !darwin || headless

package remoteview

// Supported reports whether this build has the remote view.
const Supported = false

// View is one open session window (none in this build).
type View struct{}

// Open fails: no remote view in this build.
func Open(Config, Handler) (*View, error) { return nil, ErrUnsupported }

func (v *View) SetTitle(string)                                        {}
func (v *View) Overlay(string, bool, string, bool)                     {}
func (v *View) HUD(string)                                             {}
func (v *View) Screenshot(string)                                      {}
func (v *View) StartRecording(string, bool)                            {}
func (v *View) RecordAudio([]int16)                                    {}
func (v *View) StopRecording()                                         {}
func (v *View) Video([]byte)                                           {}
func (v *View) Picture(int, int, []byte, []byte, []byte, int, int)     {}
func (v *View) CursorShape([]byte, float64, float64, float64, float64) {}
func (v *View) CursorPos(float64, float64, bool)                       {}
func (v *View) SetControl(bool)                                        {}
func (v *View) Menu(string, []MenuItem)                                {}
func (v *View) ToolState(string, bool, bool)                           {}
func (v *View) ToggleFullScreen()                                      {}
func (v *View) ChooseFiles(string)                                     {}
func (v *View) Thumbnail()                                             {}
func (v *View) Focus()                                                 {}
func (v *View) Close()                                                 {}

// OpenMain fails: no main window in this build.
func OpenMain(map[string]string, MainHandler) error { return ErrUnsupported }

// SetHome does nothing in this build.
func SetHome(Home) {}

// ShowMain does nothing in this build.
func ShowMain() {}

// SetPrefs does nothing in this build.
func SetPrefs(Prefs, bool) {}

// SetDockIcon does nothing in this build.
func SetDockIcon([]byte) {}

// Themes of the windows.
const (
	ThemeDark   = 0
	ThemeLight  = 1
	ThemeSystem = 2
)

// SetTheme does nothing in this build.
func SetTheme(int) {}

// ShowAbout does nothing here: the app shows its own About window.
func ShowAbout(string, string) {}

// SetAppMenu does nothing here.
func SetAppMenu(map[string]string) {}
