// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

/** The project's public signaling server (shared with go-link). */
export const OFFICIAL_SIGNAL_URL = "wss://signal.go-link.org/ws";

/** Default server for this build (VITE_SIGNAL_URL), else the official one. */
export const DEFAULT_SIGNAL_URL = import.meta.env.VITE_SIGNAL_URL || OFFICIAL_SIGNAL_URL;

/** The project's source repository. */
export const REPO_URL = "https://github.com/lordbasex/go-link-desk";

/** This build of the website (set by `make web-build`). */
export const APP_VERSION = import.meta.env.VITE_APP_VERSION || "dev";
