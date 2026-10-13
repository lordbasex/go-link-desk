// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect } from "react";
import { Link, Navigate, Route, Routes, useLocation } from "react-router-dom";
import { analyticsLoaded, askConsent, isPublicPath, startAnalytics, useConsent } from "./analytics";
import { CookieConsent } from "./components/CookieConsent";
import { Header } from "./components/Header";
import { APP_VERSION, REPO_URL } from "./config";
import { t, useLang } from "./i18n";
import { DevicePage } from "./pages/DevicePage";
import { DocsPage } from "./pages/DocsPage";
import { LandingPage } from "./pages/LandingPage";
import { LinkPage } from "./pages/LinkPage";
import { PrivacyPage } from "./pages/PrivacyPage";
import { DemoPage } from "./pages/DemoPage";
import { SessionPage } from "./pages/SessionPage";
import { WebHomePage } from "./pages/WebHomePage";

/**
 * desk.go-link.org: the landing (/), the docs (/docs) and the web app's
 * Home (/web) have their own look; the session, linking and privacy pages keep the site's
 * header and footer. /c and /devices, the old ways in, lead to /web.
 * The public pages (landing, docs, privacy) carry the cookie notice and,
 * with consent, Google Analytics; the app's pages never do (analytics.ts).
 */
export function App() {
  useLang();
  const path = useLocation().pathname;
  const { consent } = useConsent();
  const counted = isPublicPath(path);
  useEffect(() => {
    if (counted && consent === "granted") startAnalytics();
  }, [counted, consent]);
  // Google's script is in this page: an app page waits for the full page
  // load guardHistory started instead of rendering next to it.
  if (!counted && analyticsLoaded()) return null;
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
      <Route path="/demo" element={<DemoPage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
  const notice = counted && <CookieConsent />;
  if (ownLook)
    return (
      <div className="app app-own">
        {routes}
        {notice}
      </div>
    );
  return (
    <div className="app">
      <Header />
      <main className="main">{routes}</main>
      <footer className="footer">
        <a href={REPO_URL} rel="noreferrer">
          {t.footer.source}
        </a>{" "}
        · <Link to="/privacy">{t.footer.privacy}</Link>
        {counted && (
          <>
            {" "}
            ·{" "}
            <button type="button" className="linkish" onClick={askConsent}>
              {t.consent.settings}
            </button>
          </>
        )}{" "}
        · {APP_VERSION}
      </footer>
      {notice}
    </div>
  );
}
