// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

// The texts of a session with go-link-desk-terminal (remote_terminal.go).
func init() {
	add := func(lang string, m map[string]string) {
		for k, v := range m {
			catalogs[lang][k] = v
		}
	}
	add("es", map[string]string{
		"New terminal":                            "Nueva terminal",
		"Close “{name}” (ends its shell)":         "Cerrar “{name}” (termina su shell)",
		"Terminal name":                           "Nombre de la terminal",
		"new output":                              "salida nueva",
		"Resumed: {n} terminals were still open.": "Retomado: {n} terminales seguían abiertas.",
		"Resumed: the terminal was still open.":   "Retomado: la terminal seguía abierta.",
		"You can watch this terminal, but not type: the computer lets you only see.": "Podés mirar esta terminal, pero no escribir: la computadora solo te deja ver.",
		"The shell ended.":                "El shell terminó.",
		"The shell ended (exit code %d).": "El shell terminó (código de salida %d).",
		"Text size: %d":                   "Tamaño del texto: %d",
		"A+  Bigger text":                 "A+  Texto más grande",
		"A−  Smaller text":                "A−  Texto más chico",
		"Normal size":                     "Tamaño normal",
		"Ctrl+C (interrupt)":              "Ctrl+C (interrumpir)",
		"Ctrl+D (end of input)":           "Ctrl+D (fin de la entrada)",
		"Ctrl+Z (suspend)":                "Ctrl+Z (suspender)",
		"Ctrl+L (clear)":                  "Ctrl+L (limpiar)",
	})
	add("pt", map[string]string{
		"New terminal":                            "Novo terminal",
		"Close “{name}” (ends its shell)":         "Fechar “{name}” (encerra o shell)",
		"Terminal name":                           "Nome do terminal",
		"new output":                              "saída nova",
		"Resumed: {n} terminals were still open.": "Retomado: {n} terminais continuavam abertos.",
		"Resumed: the terminal was still open.":   "Retomado: o terminal continuava aberto.",
		"You can watch this terminal, but not type: the computer lets you only see.": "Você pode ver este terminal, mas não digitar: o computador só deixa ver.",
		"The shell ended.":                "O shell terminou.",
		"The shell ended (exit code %d).": "O shell terminou (código de saída %d).",
		"Text size: %d":                   "Tamanho do texto: %d",
		"A+  Bigger text":                 "A+  Texto maior",
		"A−  Smaller text":                "A−  Texto menor",
		"Normal size":                     "Tamanho normal",
		"Ctrl+C (interrupt)":              "Ctrl+C (interromper)",
		"Ctrl+D (end of input)":           "Ctrl+D (fim da entrada)",
		"Ctrl+Z (suspend)":                "Ctrl+Z (suspender)",
		"Ctrl+L (clear)":                  "Ctrl+L (limpar)",
	})
}
