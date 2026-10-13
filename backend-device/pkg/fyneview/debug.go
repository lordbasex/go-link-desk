// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package fyneview

import (
	"io/fs"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// IconSheet is every icon, named, for checking how they draw
// (build/uipreview -icons).
func IconSheet() fyne.CanvasObject {
	var cells []fyne.CanvasObject
	entries, _ := fs.ReadDir(icons, "assets/icons")
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".svg")
		img := canvas.NewImageFromResource(iconRes(name, c().text))
		img.FillMode = canvas.ImageFillContain
		img.SetMinSize(fyne.NewSize(15, 15))
		big := canvas.NewImageFromResource(iconRes(name, c().accent))
		big.FillMode = canvas.ImageFillContain
		big.SetMinSize(fyne.NewSize(30, 30))
		cells = append(cells, container.NewVBox(row(8, img, big), text(name, 10, regular, c().muted)))
	}
	return container.NewStack(canvas.NewRectangle(c().bg), pad(12, 12, 12, 12, container.NewGridWithColumns(8, cells...)))
}
