// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// Line icons drawn inline (no icon font, no third-party requests).

type P = { size?: number };
const base = { fill: "none", stroke: "currentColor", strokeLinecap: "round", strokeLinejoin: "round", "aria-hidden": true } as const;

export const ScreenIcon = ({ size = 18 }: P) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth={2} {...base}>
    <rect x="2" y="4" width="20" height="13" rx="2" />
    <path d="M8 21h8M12 17v4" />
  </svg>
);

export const SunIcon = ({ size = 16 }: P) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth={2} {...base}>
    <circle cx="12" cy="12" r="4" />
    <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
  </svg>
);

export const MoonIcon = ({ size = 16 }: P) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth={2} {...base}>
    <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
  </svg>
);

export const InfoIcon = ({ size = 16 }: P) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth={2.2} {...base}>
    <circle cx="12" cy="12" r="9" />
    <path d="M12 11v5M12 7.5v.5" />
  </svg>
);

export const ExpandIcon = ({ size = 16 }: P) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth={2} {...base}>
    <path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />
  </svg>
);

export const HangUpIcon = ({ size = 16 }: P) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth={2} {...base}>
    <path d="M18 6 6 18M6 6l12 12" />
  </svg>
);

export const SoundIcon = ({ size = 16, off = false }: P & { off?: boolean }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth={2} {...base}>
    <path d="M4 9v6h4l5 4V5L8 9H4z" />
    {off ? <path d="m17 9 5 6M22 9l-5 6" /> : <path d="M16.5 8.5a5 5 0 0 1 0 7M19 6a8.5 8.5 0 0 1 0 12" />}
  </svg>
);

export const ChatIcon = ({ size = 16 }: P) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth={2} {...base}>
    <path d="M4 5h16v11H9l-5 4V5z" />
  </svg>
);

export const CameraIcon = ({ size = 16 }: P) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth={2} {...base}>
    <rect x="3" y="6" width="13" height="12" rx="2" />
    <path d="m16 10 5-3v10l-5-3z" />
  </svg>
);
