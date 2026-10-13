; Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
;
; go-link Desk's Windows installer (NSIS, zlib license). Like the macOS
; and Linux ones it needs no administrator: it installs for this user in
; %LOCALAPPDATA%\Programs\go-link Desk, adds a Start menu entry and an
; entry in Settings > Apps (with its uninstaller), and starts the app.
; Uninstalling removes all of it and the "start with Windows" entry, and
; keeps the user's settings and pairings. The app updates itself by
; running a newer one with /S /UPDATE (silent, then it opens the app).
;
; build/windows.Dockerfile runs it:
;   makensis -DVERSION=v1.2.3 -DVERSION4=1.2.3.0 -DARCH=amd64 -DOUT=... installer.nsi

Unicode true
SetCompressor /SOLID lzma
RequestExecutionLevel user
ManifestDPIAware true

!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"

!define APP "go-link Desk"
!define EXE "go-link-desk.exe"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\go-link-desk"
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"

Name "${APP}"
OutFile "${OUT}"
InstallDir "$LOCALAPPDATA\Programs\${APP}"
InstallDirRegKey HKCU "${UNINSTALL_KEY}" "InstallLocation"
BrandingText "${APP} ${VERSION}"

VIProductVersion "${VERSION4}"
VIAddVersionKey /LANG=1033 "ProductName" "${APP}"
VIAddVersionKey /LANG=1033 "FileDescription" "${APP} setup"
VIAddVersionKey /LANG=1033 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=1033 "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=1033 "CompanyName" "go-link"
VIAddVersionKey /LANG=1033 "LegalCopyright" "Copyright (c) 2026 Federico Pereira. MIT License."

!define MUI_ICON "desk.ico"
!define MUI_UNICON "desk.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "LICENSE.txt"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; The system's language picks one of these; English otherwise.
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Spanish"
!insertmacro MUI_LANGUAGE "PortugueseBR"

LangString Running ${LANG_ENGLISH} "go-link Desk is running: it will be closed to update it."
LangString Running ${LANG_SPANISH} "go-link Desk está abierto: se va a cerrar para actualizarlo."
LangString Running ${LANG_PORTUGUESEBR} "O go-link Desk está aberto: ele será fechado para atualizá-lo."

; Closes a running go-link Desk: Windows does not replace a program in use.
!macro CloseApp
  nsExec::ExecToStack 'tasklist /FI "IMAGENAME eq ${EXE}" /NH'
  Pop $0
  Pop $1
  ${If} $0 == 0
    Push $1
    Push "${EXE}"
    Call ${UN}StrContains
    Pop $3
    ${If} $3 != ""
      MessageBox MB_OK|MB_ICONINFORMATION "$(Running)" /SD IDOK
      nsExec::Exec 'taskkill /IM "${EXE}" /F'
      Sleep 1000
    ${EndIf}
  ${EndIf}
!macroend

!macro StrContainsFunc UN
Function ${UN}StrContains
  Exch $R0 ; the needle
  Exch
  Exch $R1 ; the haystack
  Push $R2
  Push $R3
  Push $R4
  StrLen $R3 $R0
  StrCpy $R4 0
  StrCpy $R2 ""
  loop:
    StrCpy $R2 $R1 $R3 $R4
    StrCmp $R2 $R0 found
    StrCmp $R2 "" notfound
    IntOp $R4 $R4 + 1
    Goto loop
  found:
    StrCpy $R0 $R2
    Goto done
  notfound:
    StrCpy $R0 ""
  done:
  Pop $R4
  Pop $R3
  Pop $R2
  Pop $R1
  Exch $R0
FunctionEnd
!macroend
!insertmacro StrContainsFunc ""
!insertmacro StrContainsFunc "un."

Section "go-link Desk"
  !define UN ""
  !insertmacro CloseApp
  !undef UN
  SetOutPath "$INSTDIR"
  File "${EXE}"
  File "LICENSE.txt"
  File "THIRD_PARTY_NOTICES.md"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  CreateShortcut "$SMPROGRAMS\${APP}.lnk" "$INSTDIR\${EXE}"

  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "${APP}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$\"$INSTDIR\${EXE}$\""
  WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "go-link"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "URLInfoAbout" "https://desk.go-link.org"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegStr HKCU "${UNINSTALL_KEY}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "EstimatedSize" 40000

  ; The app's own update runs this silently with /UPDATE: open the new
  ; version, as the finish page's checkbox does otherwise.
  ${GetParameters} $R0
  ClearErrors
  ${GetOptions} $R0 "/UPDATE" $R1
  ${IfNot} ${Errors}
    Exec '"$INSTDIR\${EXE}"'
  ${EndIf}
SectionEnd

Section "Uninstall"
  !define UN "un."
  !insertmacro CloseApp
  !undef UN
  Delete "$SMPROGRAMS\${APP}.lnk"
  Delete "$INSTDIR\${EXE}"
  Delete "$INSTDIR\LICENSE.txt"
  Delete "$INSTDIR\THIRD_PARTY_NOTICES.md"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  ; "Start with Windows" points at the program just removed.
  DeleteRegValue HKCU "${RUN_KEY}" "${APP}"
  DeleteRegKey HKCU "${UNINSTALL_KEY}"
SectionEnd
