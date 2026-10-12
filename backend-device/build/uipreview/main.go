// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Command uipreview opens the Fyne main window (pkg/fyneview) with sample
// data, on any system: to compare it with the Mac app's while it is
// built.
//
//	go run ./build/uipreview [-light] [-devices]
package main

import (
	"flag"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/fyneview"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/remoteview"
)

type handler struct{}

func (handler) MainAction(name, arg string) { fmt.Println("action", name, arg) }

func main() {
	lightTheme := flag.Bool("light", false, "the light theme")
	devices := flag.Bool("devices", false, "with linked computers and recent sessions")
	view := flag.Int("view", 0, "0 cards, 1 list, 2 detailed list")
	iconSheet := flag.Bool("icons", false, "only the icons, to check how they draw")
	flag.Parse()
	a := app.NewWithID("org.go-link.desk.uipreview")
	if *iconSheet {
		w := a.NewWindow("icons")
		w.SetContent(fyneview.IconSheet())
		w.Resize(fyne.NewSize(900, 500))
		w.ShowAndRun()
		return
	}
	a.Preferences().SetInt("org.go-link.desk.view", *view)
	labels := map[string]string{
		"thisComputer": "Esta computadora", "copy": "Copiar", "copied": "Copiado", "newCode": "Nuevo código",
		"hint":     "Compartí este código solo con alguien que conozcas. Nadie legítimo te va a llamar para pedirte acceso a tu computadora.",
		"settings": "Ajustes", "address": "Su código de 9 dígitos", "doorSession": "Sesión", "doorLink": "Vincular (desatendido)",
		"connect": "Conectar", "myComputers": "Mis equipos", "favorites": "Favoritos", "recents": "Sesiones recientes",
		"clear": "Borrar", "empty": "Las computadoras que vincules con su código de vinculación aparecen acá, para conectarte sin código.",
		"colName": "Nombre", "colCode": "Código", "colSystem": "Sistema", "colWhen": "Última conexión",
		"emptyRecents":   "Las computadoras a las que te conectás con un código aparecen acá.",
		"emptyFavorites": "Marcá una computadora vinculada con la estrella para verla acá.",
	}
	if err := fyneview.OpenMain(labels, handler{}); err != nil {
		panic(err)
	}
	if *lightTheme {
		fyneview.SetTheme(remoteview.ThemeLight)
	}
	h := remoteview.Home{Code: "456 257 529", Link: "https://desk.go-link.org/c/uOxMUHDgzYn10kl0Qu_45Q", Status: "Conectado", StatusOK: true, CanNew: true, Version: "v1.0.0"}
	if *devices {
		h.Devices = []remoteview.Tile{
			{ID: "a", Title: "Home Mac", Subtitle: "11 oct 2026, 13:07", Name: "Home Mac", Code: "", System: "macOS", When: "11 oct 2026, 13:07", Favorite: true},
			{ID: "b", Title: "Workstation", Subtitle: "9 oct 2026, 21:07", Name: "Workstation", System: "Windows", When: "9 oct 2026, 21:07"},
		}
		h.Recents = []remoteview.Tile{{ID: "464786468", Title: "464 786 468", Subtitle: "Studio Mac · 11 oct 2026, 18:07", Name: "Studio Mac", Code: "464 786 468", System: "macOS", When: "11 oct 2026, 18:07"}}
	}
	fyneview.SetHome(h)
	fyneview.ShowMain()
	a.Run()
}
