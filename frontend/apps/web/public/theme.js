// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// Applies the saved light theme before the page paints (an external file,
// because the site's CSP allows no inline scripts). Dark is the default.
try {
  if (window.localStorage.getItem("go-link-desk.theme") === "light") {
    document.documentElement.dataset.theme = "light";
    document.querySelector('meta[name="theme-color"]').setAttribute("content", "#f3f4f7");
  }
} catch (e) {
  // storage blocked: dark
}
