// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Every visible text of the web lives here, in English (the default).
// Other languages (es.ts, pt.ts) are files with the same shape: Messages.

export const en = {
  brand: { name: "go-link", suffix: "Desk", home: "go-link Desk home" },
  header: {
    language: "Language",
    lightTheme: "Switch to the light theme",
    darkTheme: "Switch to the dark theme",
  },
  landing: {
    title: "Control a computer from your browser",
    text: "The other computer runs go-link Desk and shows a code. Type it here to see and control its screen. Nothing to install on this side.",
    connect: "Connect",
    steps: [
      { title: "1. Open go-link Desk", text: "On the computer to control. It needs no installer." },
      { title: "2. Read the code", text: "Nine digits, shown in its window." },
      { title: "3. Type it here", text: "The person at that computer accepts, and you are in." },
    ],
  },
  join: {
    title: "Connect to a computer",
    codeLabel: "Code or link",
    codeHint: "The 9-digit code shown by go-link Desk on the other computer, or its link.",
    connect: "Connect",
    invalid: "That is not a code or a go-link Desk link.",
  },
  session: {
    title: "Session",
    pending: "Sessions arrive in the next version: this page will connect to the computer with code {code}.",
    back: "Use another code",
  },
  footer: { source: "Source code" },
};

export type Messages = typeof en;
