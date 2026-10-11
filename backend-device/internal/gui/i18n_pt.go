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
		"Connection request":           "Pedido de conexão",
		"Someone":                      "Alguém",
		"%s wants to see your screen.": "%s quer ver sua tela.",
		"unknown":                      "desconhecido",
		"Browser: %s":                  "Navegador: %s",
		"The name and the browser are what the other side says; go-link Desk cannot check them.": "O nome e o navegador são os que o outro lado declara; o go-link Desk não pode verificá-los.",
		"If you accept, they will be able to:":                                                   "Se você aceitar, a pessoa poderá:",
		"See your screen":                                                                        "Ver sua tela",
		"Did someone call you and ask you to do this? Reject it and hang up. Nobody legitimate calls asking for access to your computer: not your bank, not the police, not a support line.": "Alguém ligou e pediu para você fazer isto? Rejeite e desligue. Ninguém legítimo liga pedindo acesso ao seu computador: nem o seu banco, nem a polícia, nem uma central de suporte.",
		"Reject":                                "Rejeitar",
		"Accept":                                "Aceitar",
		"Rejected on its own in %d s.":          "Rejeitado automaticamente em %d s.",
		"go-link Desk session":                  "Sessão do go-link Desk",
		"Disconnect":                            "Desconectar",
		"Disconnect %s":                         "Desconectar %s",
		"%s is viewing your screen":             "%s está vendo sua tela",
		"Since %s":                              "Desde %s",
		"Can:":                                  "Pode:",
		"In session with %s":                    "Em sessão com %s",
		"Permissions":                           "Permissões",
		"Screen Recording: to show your screen": "Gravação de Tela: para mostrar sua tela",
		"Accessibility: to let the viewer use the mouse and keyboard": "Acessibilidade: para quem se conecta usar o mouse e o teclado",
		"Allow…": "Permitir…",
		"After allowing it in System Settings, quit go-link Desk and open it again.": "Depois de permitir em Ajustes do Sistema, feche o go-link Desk e abra de novo.",
		"Allowed":                     "Permitido",
		"Not allowed":                 "Não permitido",
		"Use your mouse and keyboard": "Usar seu mouse e seu teclado",
	}
}
