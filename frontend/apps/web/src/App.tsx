// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { Navigate, Route, Routes } from "react-router-dom";
import { Header } from "./components/Header";
import { APP_VERSION, REPO_URL } from "./config";
import { t } from "./i18n";
import { JoinPage } from "./pages/JoinPage";
import { LandingPage } from "./pages/LandingPage";
import { SessionPage } from "./pages/SessionPage";

export function App() {
  return (
    <div className="app">
      <Header />
      <main className="main">
        <Routes>
          <Route path="/" element={<LandingPage />} />
          <Route path="/c" element={<JoinPage />} />
          <Route path="/c/:target" element={<SessionPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
      <footer className="footer">
        <a href={REPO_URL} rel="noreferrer">
          {t.footer.source}
        </a>{" "}
        · {APP_VERSION}
      </footer>
    </div>
  );
}
