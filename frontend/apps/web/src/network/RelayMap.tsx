// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { t } from "../i18n";
import { COUNTRIES, MAP } from "./americas.gen";

/** The relays signalhub hands out, and those on the way. */
export const RELAYS: readonly { id: "virginia" | "saopaulo"; lon: number; lat: number; active: boolean; labelLeft?: boolean }[] = [
  { id: "virginia", lon: -77.49, lat: 39.04, active: true },
  { id: "saopaulo", lon: -46.63, lat: -23.55, active: false, labelLeft: true },
];

const merc = (lat: number) => Math.log(Math.tan(Math.PI / 4 + (lat * Math.PI) / 360));

/** Longitude and latitude to map units, as scripts/relay-map.py projected the countries. */
export function project(lon: number, lat: number): [number, number] {
  const k = MAP.width / (((MAP.lon1 - MAP.lon0) * Math.PI) / 180);
  return [(((lon - MAP.lon0) * Math.PI) / 180) * k, (merc(MAP.lat1) - merc(lat)) * k];
}

/** The Americas (Natural Earth), with go-link Desk's relays. */
export default function RelayMap() {
  const r = t.network.relays;
  const pins = RELAYS.map((p) => ({ ...p, at: project(p.lon, p.lat) }));
  const [a, b] = pins;
  return (
    <>
      <svg className="nt-mapsvg" viewBox={`0 0 ${MAP.width} ${MAP.height}`} role="img" aria-label={r.mapLabel}>
        <g className="land">
          {COUNTRIES.map(([name, d]) => (
            <path key={name} d={d}>
              <title>{name}</title>
            </path>
          ))}
        </g>
        {a && b && <path className="link" d={`M${a.at[0]} ${a.at[1]} Q ${(a.at[0] + b.at[0]) / 2 + 40} ${(a.at[1] + b.at[1]) / 2 - 20} ${b.at[0]} ${b.at[1]}`} />}
        {pins.map((p) => (
          <g key={p.id} className={p.active ? "pin" : "pin soon"}>
            <circle className="ring" cx={p.at[0]} cy={p.at[1]} r="6" />
            <circle cx={p.at[0]} cy={p.at[1]} r="4.5" />
            <text x={p.at[0] + (p.labelLeft ? -9 : 9)} y={p.at[1] + (p.labelLeft ? 18 : -4)} textAnchor={p.labelLeft ? "end" : "start"}>
              {p.id === "virginia" ? "Virginia" : "São Paulo"}
            </text>
          </g>
        ))}
      </svg>
      <p className="nt-legend">
        <span>
          <span className="nt-dot" />
          {r.active}
        </span>
        <span>
          <span className="nt-dot soon" />
          {r.soon}
        </span>
        <span className="src">{r.source}</span>
      </p>
    </>
  );
}
