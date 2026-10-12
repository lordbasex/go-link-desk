// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { Link, Navigate, Route, Routes, useLocation } from "react-router-dom";
import { Header } from "./components/Header";
import { APP_VERSION, REPO_URL } from "./config";
import { t, useLang } from "./i18n";
import { DevicePage } from "./pages/DevicePage";
import { DocsPage } from "./pages/DocsPage";
import { LandingPage } from "./pages/LandingPage";
import { LinkPage } from "./pages/LinkPage";
import { PrivacyPage } from "./pages/PrivacyPage";
import { SessionPage } from "./pages/SessionPage";
import { WebHomePage } from "./pages/WebHomePage";

/**
 * desk.go-link.org: the landing (/), the docs (/docs) and the web app's
 * Home (/web) have their own look; the session, linking and privacy pages keep the site's
 * header and footer. /c and /devices, the old ways in, lead to /web.
 */
export function App() {
  useLang();
  const path = useLocation().pathname;
  const ownLook = path === "/" || path === "/web" || path === "/docs" || path.startsWith("/docs/");
  const routes = (
    <Routes>
      <Route path="/" element={<LandingPage />} />
      <Route path="/web" element={<WebHomePage />} />
      <Route path="/docs" element={<DocsPage />} />
      <Route path="/docs/:slug" element={<DocsPage />} />
      <Route path="/c" element={<Navigate to="/web" replace />} />
      <Route path="/c/:target" element={<SessionPage />} />
      <Route path="/link" element={<LinkPage />} />
      <Route path="/link/:code" element={<LinkPage />} />
      <Route path="/devices" element={<Navigate to="/web" replace />} />
      <Route path="/d/:deviceId" element={<DevicePage />} />
      <Route path="/privacy" element={<PrivacyPage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
  if (ownLook) return <div className="app app-own">{routes}</div>;
  return (
    <div className="app">
      <Header />
      <main className="main">{routes}</main>
      <footer className="footer">
        <a href={REPO_URL} rel="noreferrer">
          {t.footer.source}
        </a>{" "}
        · <Link to="/privacy">{t.footer.privacy}</Link> · {APP_VERSION}
      </footer>
    </div>
  );
}
