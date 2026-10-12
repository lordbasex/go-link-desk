// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux

package awake

import "github.com/godbus/dbus/v5"

// Linux asks the desktop not to go idle, over the session's D-Bus, every
// service it has of these: GNOME's session manager, the screen saver
// service (KDE Plasma, Xfce and other X11 desktops), the power manager's
// (KDE Plasma), the Inhibit portal. Each keeps it for as long as the
// connection that asked stays open: releasing is closing it.

var conn *dbus.Conn

const reason = "A go-link Desk session is on"

// idle is the inhibit flag of GNOME's session manager and of the portal.
const idle = uint32(8)

// inhibitors are the services asked, each by name (for the logs and
// tests).
var inhibitors = []struct {
	name string
	ask  func(*dbus.Conn) error
}{
	{"gnome-session", func(c *dbus.Conn) error {
		var cookie uint32
		return c.Object("org.gnome.SessionManager", "/org/gnome/SessionManager").
			Call("org.gnome.SessionManager.Inhibit", 0, "go-link-desk", uint32(0), reason, idle).Store(&cookie)
	}},
	{"screensaver", func(c *dbus.Conn) error {
		var cookie uint32
		return c.Object("org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver").
			Call("org.freedesktop.ScreenSaver.Inhibit", 0, "go-link Desk", reason).Store(&cookie)
	}},
	{"power-management", func(c *dbus.Conn) error {
		var cookie uint32
		return c.Object("org.freedesktop.PowerManagement", "/org/freedesktop/PowerManagement/Inhibit").
			Call("org.freedesktop.PowerManagement.Inhibit.Inhibit", 0, "go-link Desk", reason).Store(&cookie)
	}},
	{"portal", func(c *dbus.Conn) error {
		var handle dbus.ObjectPath
		return c.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop").
			Call("org.freedesktop.portal.Inhibit.Inhibit", 0, "", idle, map[string]dbus.Variant{"reason": dbus.MakeVariant(reason)}).
			Store(&handle)
	}},
}

func hold() bool {
	c, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	for _, in := range inhibitors {
		if in.ask(c) == nil {
			conn = c
		}
	}
	if conn == nil {
		c.Close()
		return false
	}
	return true
}

func release() {
	if conn != nil {
		conn.Close()
		conn = nil
	}
}
