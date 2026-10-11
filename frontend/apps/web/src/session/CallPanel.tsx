// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import type { Session, SessionView } from "./session";

/** What the browser asks the camera and microphone for: a face, and a voice without echo. */
const MEDIA: MediaStreamConstraints = {
  video: { width: { ideal: 640 }, height: { ideal: 480 }, frameRate: { ideal: 24 } },
  audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
};

/** Opens this computer's camera and microphone; without a camera, just the microphone. */
async function openLocalMedia(): Promise<MediaStream> {
  try {
    return await navigator.mediaDevices.getUserMedia(MEDIA);
  } catch {
    return await navigator.mediaDevices.getUserMedia({ audio: MEDIA.audio });
  }
}

/**
 * A video call with the person at the host, in a panel over the session:
 * their camera large, this viewer's own small, and the microphone, camera
 * and hang-up buttons. The call is asked for when the panel opens; the
 * camera and microphone go out only once the person at the host takes it.
 */
export function CallPanel({ view, session, onClose }: { view: SessionView; session: Session; onClose: () => void }) {
  const [local, setLocal] = useState<MediaStream | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [micOn, setMicOn] = useState(true);
  const [camOn, setCamOn] = useState(true);
  const remoteRef = useRef<HTMLVideoElement>(null);
  const selfRef = useRef<HTMLVideoElement>(null);
  const voiceRef = useRef<HTMLAudioElement>(null);
  const call = view.call;
  const active = call.state === "active";

  // Open the devices and ask for the call, once.
  useEffect(() => {
    let stream: MediaStream | null = null;
    let cancelled = false;
    void openLocalMedia().then(
      (s) => {
        if (cancelled) {
          s.getTracks().forEach((tr) => tr.stop());
          return;
        }
        stream = s;
        setLocal(s);
        session.requestCall(s.getVideoTracks().length > 0, s.getAudioTracks().length > 0);
      },
      () => setError(t.call.noDevices),
    );
    return () => {
      cancelled = true;
      stream?.getTracks().forEach((tr) => tr.stop());
    };
  }, [session]);

  // Once taken, this side's camera and microphone go out.
  useEffect(() => {
    if (!active || !local) return;
    void session.setCallMedia(local.getVideoTracks()[0] ?? null, local.getAudioTracks()[0] ?? null);
  }, [active, local, session]);

  useEffect(() => {
    if (selfRef.current && local) selfRef.current.srcObject = local;
  }, [local]);
  useEffect(() => {
    const v = remoteRef.current;
    if (v && v.srcObject !== view.callVideo) v.srcObject = view.callVideo;
  }, [view.callVideo]);
  useEffect(() => {
    const a = voiceRef.current;
    if (!a) return;
    if (a.srcObject !== view.callAudio) a.srcObject = view.callAudio;
    if (active) void Promise.resolve(a.play()).catch(() => undefined);
  }, [view.callAudio, active]);

  const toggle = (kind: "audio" | "video") => {
    if (!local) return;
    const track = kind === "audio" ? local.getAudioTracks()[0] : local.getVideoTracks()[0];
    if (!track) return;
    track.enabled = !track.enabled;
    if (kind === "audio") setMicOn(track.enabled);
    else setCamOn(track.enabled);
    if (active) void session.setCallMedia(local.getVideoTracks()[0] ?? null, local.getAudioTracks()[0] ?? null);
  };

  const hangUp = () => {
    if (call.state === "ringing" || active) session.endCall();
    onClose();
  };

  // Turned down or hung up on the other side: the panel goes after a moment.
  useEffect(() => {
    if (call.state !== "declined" && call.state !== "ended") return;
    const timer = setTimeout(onClose, 3000);
    return () => clearTimeout(timer);
  }, [call.state, onClose]);

  const status =
    error ??
    (call.state === "ringing"
      ? t.call.ringing
      : call.state === "declined"
        ? t.call.declined
        : call.state === "ended"
          ? t.call.ended
          : active && !call.hostVideo
            ? t.call.hostCameraOff
            : null);
  const hasCam = (local?.getVideoTracks().length ?? 0) > 0;

  return (
    <aside className="call-panel" role="dialog" aria-label={t.call.title}>
      <div className="call-remote">
        <video ref={remoteRef} className={active && call.hostVideo ? "" : "is-hidden"} autoPlay playsInline muted aria-label={t.call.hostCamera} />
        {status && <p className="call-status">{status}</p>}
        <video ref={selfRef} className={`call-self${hasCam && camOn ? "" : " is-hidden"}`} autoPlay playsInline muted aria-label={t.call.myCamera} />
      </div>
      <audio ref={voiceRef} autoPlay />
      <div className="call-bar">
        <button type="button" className={`button button-secondary button-small${micOn ? "" : " is-off"}`} aria-pressed={micOn} onClick={() => toggle("audio")} disabled={!local}>
          {micOn ? t.call.micOn : t.call.micOff}
        </button>
        {hasCam && (
          <button type="button" className={`button button-secondary button-small${camOn ? "" : " is-off"}`} aria-pressed={camOn} onClick={() => toggle("video")}>
            {camOn ? t.call.camOn : t.call.camOff}
          </button>
        )}
        <button type="button" className="button button-danger button-small" onClick={hangUp}>
          {t.call.hangUp}
        </button>
      </div>
    </aside>
  );
}
