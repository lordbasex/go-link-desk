// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

func init() {
	catalogs["es"] = map[string]string{
		"Your code":              "Tu código",
		"Copy":                   "Copiar",
		"Copied":                 "Copiado",
		"New code":               "Nuevo código",
		"Or open %s":             "O abrí %s",
		"Connected":              "Conectado",
		"Connecting…":            "Conectando…",
		"Offline, reconnecting…": "Sin conexión, reconectando…",
		"Code: %s":               "Código: %s",
		"No code yet":            "Todavía sin código",
		"Open go-link Desk":      "Abrir go-link Desk",
		"Share this code only with someone you know. Nobody legitimate will call you asking for access to your computer.": "Compartí este código solo con alguien que conozcas. Nadie legítimo te va a llamar para pedirte acceso a tu computadora.",
	}
}
