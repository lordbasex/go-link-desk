// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

/**
 * A short description of the browser for the host's consent dialog:
 * "Chrome 141 on macOS". It comes from the user agent, which the host
 * shows as declared, never as verified.
 */
export function browserLabel(ua: string): string {
  const browsers: [RegExp, string][] = [
    [/Edg(?:e|A|iOS)?\/(\d+)/, "Edge"],
    [/OPR\/(\d+)/, "Opera"],
    [/Firefox\/(\d+)/, "Firefox"],
    [/FxiOS\/(\d+)/, "Firefox"],
    [/CriOS\/(\d+)/, "Chrome"],
    [/Chrome\/(\d+)/, "Chrome"],
    [/Version\/(\d+)[.\d]* (?:Mobile\/\S+ )?Safari\//, "Safari"],
  ];
  let browser = "";
  for (const [re, name] of browsers) {
    const m = re.exec(ua);
    if (m) {
      browser = `${name} ${m[1]}`;
      break;
    }
  }
  const systems: [RegExp, string][] = [
    [/iPhone|iPad|iPod/, "iOS"],
    [/Android/, "Android"],
    [/CrOS/, "ChromeOS"],
    [/Mac OS X|Macintosh/, "macOS"],
    [/Windows/, "Windows"],
    [/Linux/, "Linux"],
  ];
  const system = systems.find(([re]) => re.test(ua))?.[1] ?? "";
  if (browser && system) return `${browser} on ${system}`;
  return browser || system || "Browser";
}
