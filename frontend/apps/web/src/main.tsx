// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
// Fonts are served by the site itself (never a third-party font service,
// which would receive every visitor's IP address).
import "@fontsource/chakra-petch/600.css";
import "@fontsource/chakra-petch/700.css";
import "@fontsource/ibm-plex-sans/400.css";
import "@fontsource/ibm-plex-sans/500.css";
import "@fontsource/ibm-plex-sans/600.css";
import "@fontsource/jetbrains-mono/500.css";
import "@go-link-desk/shared/tokens.css";
import "./styles/global.css";
import { guardHistory } from "./analytics";
import { App } from "./App";
import { useLang } from "./i18n";

/** Re-renders the whole tree when the language changes. */
function LangRoot() {
  useLang();
  return (
    <BrowserRouter>
      <App />
    </BrowserRouter>
  );
}

// Before anything navigates and before Google's script could load.
guardHistory();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <LangRoot />
  </StrictMode>,
);
