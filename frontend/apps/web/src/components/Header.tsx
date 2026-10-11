// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { Link } from "react-router-dom";
import { LANGS, getLang, setLang, t, type Lang } from "../i18n";
import { setTheme, useTheme } from "../theme";
import { MoonIcon, ScreenIcon, SunIcon } from "./Icons";
import { Select } from "./ui/Select";

/** The site's header: the brand, the language and the theme. */
export function Header() {
  const theme = useTheme();
  const light = theme === "light";
  return (
    <header className="header">
      <Link to="/" className="brand" aria-label={t.brand.home}>
        <ScreenIcon size={20} />
        {t.brand.name} <span className="brand-suffix">{t.brand.suffix}</span>
      </Link>
      <div className="header-tools">
        <Link to="/devices" className="header-link">
          {t.header.devices}
        </Link>
        <Select<Lang>
          value={getLang()}
          options={LANGS.map((l) => ({ value: l.id, label: l.name }))}
          onChange={setLang}
          ariaLabel={t.header.language}
          className="select-sm"
        />
        <button
          type="button"
          className="icon-button"
          aria-label={light ? t.header.darkTheme : t.header.lightTheme}
          onClick={() => setTheme(light ? "dark" : "light")}
        >
          {light ? <MoonIcon /> : <SunIcon />}
        </button>
      </div>
    </header>
  );
}
