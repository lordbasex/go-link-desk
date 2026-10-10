// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import type { Messages } from "./en";

export const es: Messages = {
  brand: { name: "go-link", suffix: "Desk", home: "Inicio de go-link Desk" },
  header: {
    language: "Idioma",
    lightTheme: "Cambiar al tema claro",
    darkTheme: "Cambiar al tema oscuro",
  },
  landing: {
    title: "Controlá una computadora desde el navegador",
    text: "La otra computadora abre go-link Desk y muestra un código. Escribilo acá para ver y controlar su pantalla. De este lado no se instala nada.",
    connect: "Conectar",
    steps: [
      { title: "1. Abrí go-link Desk", text: "En la computadora a controlar. No necesita instalador." },
      { title: "2. Leé el código", text: "Nueve dígitos, en su ventana." },
      { title: "3. Escribilo acá", text: "La persona en esa computadora acepta y entrás." },
    ],
  },
  join: {
    title: "Conectarse a una computadora",
    codeLabel: "Código o link",
    codeHint: "El código de 9 dígitos que muestra go-link Desk en la otra computadora, o su link.",
    connect: "Conectar",
    invalid: "Eso no es un código ni un link de go-link Desk.",
  },
  session: {
    title: "Sesión",
    pending: "Las sesiones llegan en la próxima versión: esta página se va a conectar a la computadora con el código {code}.",
    back: "Usar otro código",
  },
  footer: { source: "Código fuente" },
};
