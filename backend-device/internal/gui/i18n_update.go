// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

// The texts of the update window and its menu items.
func init() {
	add := func(lang string, m map[string]string) {
		for k, v := range m {
			catalogs[lang][k] = v
		}
	}
	add("es", map[string]string{
		"Try the demo":                "Probar la demo",
		"Test My Connection…":         "Probar mi conexión…",
		"Check for Updates…":          "Buscar actualizaciones…",
		"Update to %s…":               "Actualizar a la %s…",
		"Software update":             "Actualización",
		"Checking for updates…":       "Buscando actualizaciones…",
		"Could not check for updates": "No se pudo buscar actualizaciones",
		"There is no connection to GitHub, where the versions are. Try again later.": "No hay conexión con GitHub, donde están las versiones. Probá de nuevo más tarde.",
		"go-link Desk is up to date":       "go-link Desk está al día",
		"You have %s, the newest version.": "Tenés la %s, la versión más nueva.",
		"You have %s.":                     "Tenés la %s.",
		"go-link Desk %s is available":     "Hay una actualización: go-link Desk %s",
		"This copy cannot replace itself: download the new one from its page and install it as before.": "Esta copia no se puede reemplazar sola: bajá la nueva desde su página e instalala como la primera vez.",
		"Updating ends the session in progress.":                                                        "Actualizar corta la sesión en curso.",
		"What's new":                                                                                    "Novedades",
		"Later":                                                                                         "Más tarde",
		"Download":                                                                                      "Descargar",
		"Update":                                                                                        "Actualizar",
		"Downloading go-link Desk %s…":                                                                  "Descargando go-link Desk %s…",
		"Installing…":                                                                                   "Instalando…",
		"go-link Desk opens again by itself.":                                                           "go-link Desk se vuelve a abrir solo.",
		"The update was not installed":                                                                  "La actualización no se instaló",
		"The download is not the one go-link Desk published (its signature or its contents differ), so it was thrown away.": "Lo descargado no es lo que publicó go-link Desk (su firma o su contenido no coinciden), así que se descartó.",
		"Something went wrong while downloading or installing it. Try again, or download it from its page.":                 "Algo falló al descargarla o instalarla. Probá de nuevo, o bajala desde su página.",
		"Open its page": "Abrir su página",
		"Close":         "Cerrar",
		"OK":            "Aceptar",
	})
	add("pt", map[string]string{
		"Try the demo":                "Experimentar a demo",
		"Test My Connection…":         "Testar minha conexão…",
		"Check for Updates…":          "Procurar atualizações…",
		"Update to %s…":               "Atualizar para a %s…",
		"Software update":             "Atualização",
		"Checking for updates…":       "Procurando atualizações…",
		"Could not check for updates": "Não foi possível procurar atualizações",
		"There is no connection to GitHub, where the versions are. Try again later.": "Não há conexão com o GitHub, onde estão as versões. Tente de novo mais tarde.",
		"go-link Desk is up to date":       "O go-link Desk está atualizado",
		"You have %s, the newest version.": "Você tem a %s, a versão mais nova.",
		"You have %s.":                     "Você tem a %s.",
		"go-link Desk %s is available":     "Há uma atualização: go-link Desk %s",
		"This copy cannot replace itself: download the new one from its page and install it as before.": "Esta cópia não pode se substituir sozinha: baixe a nova na página dela e instale como da primeira vez.",
		"Updating ends the session in progress.":                                                        "Atualizar encerra a sessão em andamento.",
		"What's new":                                                                                    "Novidades",
		"Later":                                                                                         "Mais tarde",
		"Download":                                                                                      "Baixar",
		"Update":                                                                                        "Atualizar",
		"Downloading go-link Desk %s…":                                                                  "Baixando o go-link Desk %s…",
		"Installing…":                                                                                   "Instalando…",
		"go-link Desk opens again by itself.":                                                           "O go-link Desk abre de novo sozinho.",
		"The update was not installed":                                                                  "A atualização não foi instalada",
		"The download is not the one go-link Desk published (its signature or its contents differ), so it was thrown away.": "O que foi baixado não é o que o go-link Desk publicou (a assinatura ou o conteúdo não batem), então foi descartado.",
		"Something went wrong while downloading or installing it. Try again, or download it from its page.":                 "Algo deu errado ao baixar ou instalar. Tente de novo, ou baixe na página dela.",
		"Open its page": "Abrir a página",
		"Close":         "Fechar",
		"OK":            "OK",
	})
}
