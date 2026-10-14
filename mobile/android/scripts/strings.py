#!/usr/bin/env python3
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
# Writes the app's texts (app/src/main/res/values*/strings.xml) in English,
# Spanish and Portuguese. The English is below; the translations are the iOS
# app's (mobile/ios/DeskViewer/Resources/*.lproj/Localizable.strings), so both
# apps say the same, and ANDROID holds the few texts only this app has.
# Fails when a text has no translation. Run it after changing a text:
#   python3 scripts/strings.py
import os
import re
import sys

here = os.path.dirname(os.path.abspath(__file__))
root = os.path.dirname(here)
ios = os.path.join(root, "..", "ios", "DeskViewer", "Resources")

# name: English (the iOS app's key when the text is the same).
EN = {
    "app_name": "go-link Desk",
    "remote_desktop": "Remote desktop",
    "settings": "Settings",
    "connect_to_another": "Connect to another computer",
    "its_code": "Its 9-digit code",
    "connect": "Connect",
    "try_demo": "Try the demo",
    "demo_unavailable": "The demo computer is not available right now. Try again in a minute.",
    "home_note": "Ask the person at the other computer for the 9-digit code go-link Desk shows there. They accept the session on their screen.",
    "my_computers": "My computers",
    "link": "Link",
    "computers_empty": "Link your own computers once with their pairing code, and open them later with %@, without anyone there.",
    "recent_sessions": "Recent sessions",
    "clear": "Clear",
    "remove_from_list": "Remove from the list",
    "forget_computer": "Forget this computer",
    "linked_computer": "Linked computer",
    "link_a_computer": "Link a computer",
    "pairing_code": "The computer's pairing code",
    "link_footer": "On the computer: go-link Desk › Settings › Unattended access, turned on. The person there accepts once; from then on this device opens it with %@, without anyone accepting. Link only your own computers.",
    "link_and_connect": "Link and connect",
    "cannot_open_it": "Cannot open it",
    "ok": "OK",
    "cancel": "Cancel",
    "cancelled": "Cancelled.",
    "done": "Done",
    "back": "Back",
    "connect_to_x": "Connect to %@",
    "the_computer": "the computer",
    "the_computer_cap": "The computer",
    "your_name": "Your name",
    "your_name_footer": "Shown to the computer you connect to. Empty: this device's name.",
    "fingers": "Fingers",
    "trackpad": "Trackpad",
    "touch": "Touch",
    "touch_explained": "The pointer goes where you touch, and a tap clicks there. Two fingers scroll.",
    "trackpad_explained": "Slide a finger to move the pointer and tap to click; long press then slide to drag. Two fingers: tap to right click, pinch to zoom. Three fingers scroll.",
    "message_sound": "Message sound",
    "message_sound_footer": "A short sound when the person at the computer writes to you in the chat.",
    "signaling_server": "Signaling server",
    "signaling_footer": "Empty: go-link's. Your own signalhub server starts with wss://; it is checked before it is saved.",
    "checking": "Checking…",
    "save": "Save",
    "use_official_server": "Use go-link's server",
    "server_not_address": "That is not a server address: it starts with wss://.",
    "server_no_answer": "That server did not answer.",
    "version": "Version",
    "connection": "Connection",
    "test_connection": "Test my connection",
    "connection_footer": "Is your connection direct or through the relay? The test shows the path through your routers and how to improve it.",
    "about_footer": "Free and open source software (MIT license). The picture, sound and controls go straight between the devices, encrypted end to end.",
    # The session
    "st_connecting": "Connecting…",
    "st_waiting": "Waiting for the person at the computer to accept…",
    "st_accepted": "Accepted. Connecting…",
    "st_reconnecting": "The connection dropped. Reconnecting…",
    "st_rej_timeout": "Nobody answered at the computer.",
    "st_rej_busy": "The computer is in another session.",
    "st_rej_off": "The computer is not taking sessions now.",
    "st_rej_declined": "The person at the computer did not accept.",
    "st_end_viewer": "You ended the session.",
    "st_end_network": "The connection was lost.",
    "st_end_idle": "The session ended after a while without use.",
    "st_end_host": "The person at the computer ended the session.",
    "st_not_found": "That code does not lead anywhere. It may have changed: ask for the current one.",
    "st_rate_limited": "Too many tries. Wait a minute and try again.",
    "st_unreachable": "The server does not answer. Check the connection.",
    "st_offline": "The computer is not on, or go-link Desk is closed there.",
    "st_unlinked": "That computer no longer has this link (it was removed there). It was forgotten here: link it again with its pairing code.",
    "st_untrusted": "Whoever answered could not prove it is your linked computer. Nothing was sent to it.",
    "st_pw_locked": "Too many wrong passwords. Try again in %d minutes.",
    "st_pw_blocked": "Too many wrong passwords.",
    "linked_notice": "Linked: next time, it opens with %@, without anyone accepting.",
    "show_toolbar": "Show the toolbar",
    "hide_toolbar": "Hide the toolbar",
    "disconnect": "Disconnect",
    "keyboard": "Keyboard",
    "chat": "Chat",
    "files": "Files",
    "screen": "Screen",
    "lock": "Lock",
    "how_to_use": "How to use it",
    "hold_to_unlock": "Hold to unlock",
    "special_keys": "Special keys",
    "paste_clipboard": "Paste this device's clipboard",
    "display": "Display",
    "picture": "Picture",
    "speed": "Speed",
    "balanced": "Balanced",
    "quality": "Quality",
    "sound_on": "Turn the sound on",
    "mute": "Mute the sound",
    "show_details": "Show the details",
    "capture": "Screenshot and recording",
    "take_screenshot": "Take a screenshot",
    "record_screen": "Record the screen",
    "stop_recording": "Stop recording",
    "capture_shown": "The person at the computer sees when you take a screenshot or record.",
    "rec_refused": "The computer did not allow it: the person there must tick “Take screenshots and record the screen”.",
    "rec_failed": "The recording could not be saved.",
    "hide_details": "Hide the details",
    "copy": "Copy",
    "paste": "Paste",
    "undo": "Undo",
    "select_all": "Select all",
    "switch_apps": "Switch apps",
    "spotlight": "Spotlight",
    "force_quit": "Force quit",
    "lock_screen": "Lock the screen",
    "screenshot": "Screenshot",
    "start_menu": "Start menu",
    "task_manager": "Task manager",
    "size": "size",
    "loss": "loss",
    "path": "path",
    "relay": "relay",
    "direct": "direct",
    "password_asked": "This computer asks for its unattended access password.",
    "password": "Password",
    "wrong_password": "Wrong password. %d tries left.",
    "unlock": "Unlock",
    # The chat
    "chat_empty": "Write to the person at the computer. They see it in a window on their screen.",
    "message": "Message",
    "send": "Send",
    "is_typing": "%@ is typing…",
    # The files
    "computers_files": "The computer's files",
    "folder_empty": "This folder is empty",
    "cannot_open_folder": "Cannot open this folder",
    "try_again": "Try again",
    "from_files": "From Files",
    "from_photos": "From Photos",
    "file_unreadable": "That file cannot be read.",
    "sent_to_downloads": "Sent to the computer's Downloads/go-link-desk:",
    # The guide
    "guide_in_touch": "You are in touch mode: the pointer goes where you touch.",
    "guide_in_trackpad": "You are in trackpad mode: the screen works like a laptop's trackpad.",
    "trackpad_mode": "Trackpad mode",
    "touch_mode": "Touch mode",
    "g_slide": "One finger, slide",
    "g_slide_text": "Moves the pointer. Slow for precision, quick to go far.",
    "g_tap": "One finger, tap",
    "g_tap_text": "Click. Two quick taps: double click.",
    "g_hold": "One finger, hold then slide",
    "g_hold_text": "Drag: select text, move a window or a file.",
    "g_two_tap": "Two fingers, tap",
    "g_two_tap_text": "Right click (the menu).",
    "g_pinch": "Two fingers, pinch",
    "g_pinch_text": "Zoom in and out. Zoomed in, slide two fingers to move around.",
    "g_three": "Three fingers, slide",
    "g_three_text": "Scroll up, down and sideways.",
    "g_touch": "Touch",
    "g_touch_text": "The pointer goes there and clicks.",
    "g_touch_hold": "Hold then slide",
    "g_touch_hold_text": "Drag.",
    "g_touch_right": "Right click.",
    "g_two_slide": "Two fingers, slide",
    "g_two_slide_text": "Scroll.",
    "g_kb_button": "The keyboard button",
    "g_kb_button_text": "Opens this device's keyboard: what you type is written on the computer.",
    "g_kb_bar": "The bar over the keyboard",
    "g_kb_bar_text": "Esc, Tab, the arrows, Delete and F1 to F12. Ctrl, Alt (⌥) and Win (⌘) stay pressed for the next key: Ctrl, then C, is Ctrl+C.",
    "g_kb_hw_text": "Its keys work as on the computer, shortcuts included.",
    "the_toolbar": "The toolbar",
    "g_keys_text": "Shortcuts of the computer (switch apps, Ctrl+Alt+Del, Spotlight…) and Paste this device's clipboard there.",
    "g_chat_text": "Write to the person at the computer; a number shows messages you have not read.",
    "g_files_text": "Browse the computer's files: tap one to download it to this device, or send one of yours.",
    "g_screen_text": "The computer's displays, the picture's quality, trackpad or touch, the sound and the details.",
    "g_lock_text": "To watch without touching anything: the screen stops sending. Hold the lock for a second to unlock.",
    "g_hide": "Hide",
    "g_hide_text": "Hides the toolbar; the arrow brings it back.",
    "g_mouse": "Move and click",
    "g_mouse_text": "The pointer follows it; the buttons click (the secondary one is right click) and scrolling scrolls.",
}

# Texts only the Android app has: English, Spanish, Portuguese.
ANDROID = {
    "unlock_biometrics": ("your fingerprint or face", "tu huella o tu rostro", "sua digital ou seu rosto"),
    "unlock_screen_lock": ("the screen lock", "el bloqueo de pantalla", "o bloqueio de tela"),
    "unlock_subtitle": (
        "So that only you can open the computers you linked.",
        "Para que solo vos puedas abrir las computadoras que vinculaste.",
        "Para que só você possa abrir os computadores que vinculou.",
    ),
    "link_no_lock": (
        "Set a screen lock on this device first: linked computers are protected with it and with your fingerprint or face.",
        "Primero poné un bloqueo de pantalla en este equipo: las computadoras vinculadas se protegen con él y con tu huella o tu rostro.",
        "Primeiro defina um bloqueio de tela neste aparelho: os computadores vinculados são protegidos com ele e com sua digital ou seu rosto.",
    ),
    "link_keystore": ("The device's keystore refused it.", "El almacén de claves del equipo lo rechazó.", "O armazenamento de chaves do aparelho recusou."),
    "saved_in_downloads": ("Saved in Downloads › go-link-desk:", "Guardado en Descargas › go-link-desk:", "Salvo em Downloads › go-link-desk:"),
    "share": ("Share", "Compartir", "Compartilhar"),
    "shot_saved": ("Screenshot saved in Pictures › go-link-desk.", "Captura guardada en Imágenes › go-link-desk.", "Captura salva em Imagens › go-link-desk."),
    "shot_failed": ("The screenshot could not be saved.", "No se pudo guardar la captura.", "Não foi possível salvar a captura."),
    "rec_saved": ("Recording saved in Movies › go-link-desk.", "Grabación guardada en Películas › go-link-desk.", "Gravação salva em Filmes › go-link-desk."),
    "rec_limit": (
        "The recording reached its 2-hour limit and was saved in Movies › go-link-desk.",
        "La grabación llegó a su límite de 2 horas y quedó guardada en Películas › go-link-desk.",
        "A gravação chegou ao limite de 2 horas e foi salva em Filmes › go-link-desk.",
    ),
    "rec_host_stopped": (
        "The person at the computer stopped the recording; what was recorded is in Movies › go-link-desk.",
        "La persona en la computadora detuvo la grabación; lo grabado está en Películas › go-link-desk.",
        "A pessoa no computador parou a gravação; o que foi gravado está em Filmes › go-link-desk.",
    ),
    "rec_session_ended": (
        "The session ended; the recording is in Movies › go-link-desk.",
        "La sesión terminó; la grabación está en Películas › go-link-desk.",
        "A sessão terminou; a gravação está em Filmes › go-link-desk.",
    ),
    "g_kb_hw": ("A keyboard connected to this device", "Un teclado conectado a este equipo", "Um teclado conectado a este aparelho"),
    "g_mouse_title": ("A mouse or trackpad", "Un mouse o trackpad", "Um mouse ou trackpad"),
}


def read_strings(lang):
    text = open(os.path.join(ios, f"{lang}.lproj", "Localizable.strings"), encoding="utf-8").read()
    out = {}
    for m in re.finditer(r'^"((?:[^"\\]|\\.)*)"\s*=\s*"((?:[^"\\]|\\.)*)";', text, re.M):
        out[m.group(1).replace('\\"', '"')] = m.group(2).replace('\\"', '"')
    return out


def android(text):
    """iOS's %@ and %d as Android's numbered arguments, and XML-safe."""
    n = [0]

    def arg(m):
        n[0] += 1
        return f"%{n[0]}${'s' if m.group(1) == '@' else 'd'}"

    t = re.sub(r"%([@d])", arg, text.replace("%%", "\x00"))
    t = t.replace("\x00", "%%")
    t = t.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
    return t.replace("\\", "\\\\").replace("'", "\\'").replace('"', '\\"').replace("\n", "\\n")


def write(folder, values):
    path = os.path.join(root, "app", "src", "main", "res", folder, "strings.xml")
    os.makedirs(os.path.dirname(path), exist_ok=True)
    lines = [
        '<?xml version="1.0" encoding="utf-8"?>',
        "<!-- Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com> -->",
        "<!-- Generated by scripts/strings.py from the iOS app's texts; DO NOT EDIT. -->",
        "<resources>",
    ]
    for name, text in values.items():
        extra = ' translatable="false"' if name == "app_name" and folder == "values" else ""
        lines.append(f'    <string name="{name}"{extra}>{android(text)}</string>')
    lines.append("</resources>")
    open(path, "w", encoding="utf-8").write("\n".join(lines) + "\n")


missing = 0
english = dict(EN)
for k, v in ANDROID.items():
    english[k] = v[0]
write("values", english)
for i, lang in ((1, "es"), (2, "pt")):
    have = read_strings(lang)
    out = {}
    for name, text in EN.items():
        if name == "app_name":
            continue
        if text in have:
            out[name] = have[text]
        else:
            print(f"no {lang} for {name}: {text!r}")
            missing += 1
    for k, v in ANDROID.items():
        out[k] = v[i]
    write(f"values-{lang}", out)
sys.exit(1 if missing else 0)
