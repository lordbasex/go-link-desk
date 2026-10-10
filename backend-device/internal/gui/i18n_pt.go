// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

func init() {
	catalogs["pt"] = map[string]string{
		"Your code":              "Seu código",
		"Copy":                   "Copiar",
		"Copied":                 "Copiado",
		"New code":               "Novo código",
		"Or open %s":             "Ou abra %s",
		"Connected":              "Conectado",
		"Connecting…":            "Conectando…",
		"Offline, reconnecting…": "Sem conexão, reconectando…",
		"Code: %s":               "Código: %s",
		"No code yet":            "Ainda sem código",
		"Open go-link Desk":      "Abrir go-link Desk",
		"Share this code only with someone you know. Nobody legitimate will call you asking for access to your computer.": "Compartilhe este código só com alguém que você conhece. Ninguém legítimo vai ligar pedindo acesso ao seu computador.",
	}
}
