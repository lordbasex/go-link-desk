// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// The sound of a chat message from the person at the host: two short
// soft notes, made with Web Audio (no file to load). It can be turned off
// in the chat; the choice stays in this browser.

const KEY = "go-link-desk.chatSound";

let ctx: AudioContext | null = null;

export function chatSoundOn(): boolean {
  try {
    return localStorage.getItem(KEY) !== "off";
  } catch {
    return true;
  }
}

export function setChatSound(on: boolean): void {
  try {
    localStorage.setItem(KEY, on ? "on" : "off");
  } catch {
    // private browsing: it stays on for this visit
  }
}

/** Plays the message sound, if it is on. */
export function playChatSound(): void {
  if (!chatSoundOn()) return;
  try {
    ctx ??= new AudioContext();
    if (ctx.state === "suspended") void ctx.resume();
    const now = ctx.currentTime;
    [880, 1320].forEach((freq, i) => {
      const at = now + i * 0.11;
      const osc = ctx!.createOscillator();
      const gain = ctx!.createGain();
      osc.type = "sine";
      osc.frequency.value = freq;
      gain.gain.setValueAtTime(0, at);
      gain.gain.linearRampToValueAtTime(0.18, at + 0.01);
      gain.gain.exponentialRampToValueAtTime(0.001, at + 0.16);
      osc.connect(gain).connect(ctx!.destination);
      osc.start(at);
      osc.stop(at + 0.17);
    });
  } catch {
    // no Web Audio: no sound
  }
}
