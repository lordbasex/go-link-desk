// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import type { Messages } from "./en";

export const pt: Messages = {
  brand: { name: "go-link", suffix: "Desk", home: "Início do go-link Desk" },
  header: {
    language: "Idioma",
    lightTheme: "Mudar para o tema claro",
    darkTheme: "Mudar para o tema escuro",
  },
  landing: {
    title: "Controle um computador pelo navegador",
    text: "O outro computador abre o go-link Desk e mostra um código. Digite-o aqui para ver e controlar a tela. Nada para instalar deste lado.",
    connect: "Conectar",
    steps: [
      { title: "1. Abra o go-link Desk", text: "No computador a controlar. Não precisa de instalador." },
      { title: "2. Leia o código", text: "Nove dígitos, na janela dele." },
      { title: "3. Digite aqui", text: "A pessoa naquele computador aceita e você entra." },
    ],
  },
  join: {
    title: "Conectar a um computador",
    codeLabel: "Código ou link",
    codeHint: "O código de 9 dígitos que o go-link Desk mostra no outro computador, ou o link.",
    connect: "Conectar",
    invalid: "Isso não é um código nem um link do go-link Desk.",
  },
  session: {
    title: "Sessão",
    pending: "As sessões chegam na próxima versão: esta página vai se conectar ao computador com o código {code}.",
    back: "Usar outro código",
  },
  footer: { source: "Código-fonte" },
};
