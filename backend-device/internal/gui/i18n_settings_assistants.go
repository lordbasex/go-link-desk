// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

// The texts of Settings › Assistants (assistants.go).
func init() {
	add := func(lang string, m map[string]string) {
		for k, v := range m {
			catalogs[lang][k] = v
		}
	}
	add("es", map[string]string{
		"Let AI assistants use your computers": "Permitir que los asistentes de IA usen tus computadoras",
		"Adds go-link Desk to Claude, Claude Code and Cursor on this computer (as an MCP server). Open them again after turning it on.": "Agrega go-link Desk a Claude, Claude Code y Cursor en esta computadora (como servidor MCP). Abrilos de nuevo después de activarlo.",
		"Could not add it":               "No se pudo agregar",
		"Added":                          "Agregado",
		"Not installed":                  "No instalado",
		"Assistants":                     "Asistentes",
		"What they may do":               "Qué pueden hacer",
		"Let them type and run commands": "Permitirles escribir y ejecutar comandos",
		"Off: they only list your computers, look at screenshots and read terminals.": "Desactivado: solo ven la lista de tus computadoras, capturas de pantalla y lo que muestran las terminales.",
		"Allowed: its password is kept in this computer's keychain.":                  "Permitida: su contraseña está guardada en el llavero de esta computadora.",
		"Stop allowing": "Dejar de permitir",
		"Its password":  "Su contraseña",
		"Allow":         "Permitir",
		"No computer is linked yet: link one from My computers first.": "Todavía no hay computadoras vinculadas: vinculá una desde Mis computadoras.",
		"Computers they may use": "Computadoras que pueden usar",
		"Each computer's unattended password is kept in this computer's keychain: assistants never see it. They come in by name, with that link's permissions, and the computer shows it.": "La contraseña de acceso desatendido de cada computadora se guarda en el llavero de esta: los asistentes nunca la ven. Entran con su nombre, con los permisos de ese vínculo, y la computadora lo muestra.",
		"Everything they do is written to an activity log.": "Todo lo que hacen queda en un registro de actividad.",
		"Show the log":                   "Ver el registro",
		"Type its password first.":       "Escribí primero su contraseña.",
		"The keychain did not keep it: ": "El llavero no la guardó: ",
		"This computer has no keychain go-link Desk can use (on Linux, install secret-tool).": "Esta computadora no tiene un llavero que go-link Desk pueda usar (en Linux, instalá secret-tool).",
	})
	add("pt", map[string]string{
		"Let AI assistants use your computers": "Permitir que assistentes de IA usem seus computadores",
		"Adds go-link Desk to Claude, Claude Code and Cursor on this computer (as an MCP server). Open them again after turning it on.": "Adiciona o go-link Desk ao Claude, Claude Code e Cursor neste computador (como servidor MCP). Abra-os de novo depois de ativar.",
		"Could not add it":               "Não foi possível adicionar",
		"Added":                          "Adicionado",
		"Not installed":                  "Não instalado",
		"Assistants":                     "Assistentes",
		"What they may do":               "O que podem fazer",
		"Let them type and run commands": "Permitir que digitem e executem comandos",
		"Off: they only list your computers, look at screenshots and read terminals.": "Desativado: só veem a lista dos seus computadores, capturas de tela e o que os terminais mostram.",
		"Allowed: its password is kept in this computer's keychain.":                  "Permitido: a senha dele fica no chaveiro deste computador.",
		"Stop allowing": "Deixar de permitir",
		"Its password":  "A senha dele",
		"Allow":         "Permitir",
		"No computer is linked yet: link one from My computers first.": "Ainda não há computadores vinculados: vincule um em Meus computadores.",
		"Computers they may use": "Computadores que podem usar",
		"Each computer's unattended password is kept in this computer's keychain: assistants never see it. They come in by name, with that link's permissions, and the computer shows it.": "A senha de acesso não supervisionado de cada computador fica no chaveiro deste: os assistentes nunca a veem. Eles entram com o nome deles, com as permissões desse vínculo, e o computador mostra isso.",
		"Everything they do is written to an activity log.": "Tudo o que fazem fica num registro de atividade.",
		"Show the log":                   "Ver o registro",
		"Type its password first.":       "Digite a senha primeiro.",
		"The keychain did not keep it: ": "O chaveiro não a guardou: ",
		"This computer has no keychain go-link Desk can use (on Linux, install secret-tool).": "Este computador não tem um chaveiro que o go-link Desk possa usar (no Linux, instale o secret-tool).",
	})
}
