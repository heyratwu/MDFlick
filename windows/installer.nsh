; MyGo supplies the directory/finish pages, per-user Start menu shortcut,
; ProgIDs/OpenWithProgids, and Settings > Apps uninstall entry.
; Keep these ProgIDs in sync with identifier/fileAssociations in mygo.json.
!define MDFLICK_CAPABILITIES "Software\com.mdflick.app\Capabilities"

; Supply a default only for extensions with no existing handler. Windows
; UserChoice remains authoritative; changing it requires the system UI.
!macro AssociateIfUnclaimed EXT
  ReadRegStr $0 HKCR ".${EXT}" ""
  ${If} $0 == ""
    WriteRegStr HKCU "Software\Classes\.${EXT}" "" "com.mdflick.app.${EXT}"
  ${EndIf}
!macroend

!macro RemoveOwnedDefault EXT
  ReadRegStr $0 HKCU "Software\Classes\.${EXT}" ""
  ${If} $0 == "com.mdflick.app.${EXT}"
    DeleteRegValue HKCU "Software\Classes\.${EXT}" ""
    DeleteRegKey /ifempty HKCU "Software\Classes\.${EXT}"
  ${EndIf}
!macroend

!macro customInstall
  WriteRegStr HKCU "${MDFLICK_CAPABILITIES}" "ApplicationName" "MDFlick"
  WriteRegStr HKCU "${MDFLICK_CAPABILITIES}" "ApplicationDescription" "Markdown document viewer"
  WriteRegStr HKCU "${MDFLICK_CAPABILITIES}" "ApplicationIcon" '$\"$INSTDIR\MDFlick.exe$\",0'
  WriteRegStr HKCU "${MDFLICK_CAPABILITIES}\FileAssociations" ".md" "com.mdflick.app.md"
  WriteRegStr HKCU "${MDFLICK_CAPABILITIES}\FileAssociations" ".markdown" "com.mdflick.app.markdown"
  WriteRegStr HKCU "Software\RegisteredApplications" "MDFlick" "${MDFLICK_CAPABILITIES}"
  !insertmacro AssociateIfUnclaimed md
  !insertmacro AssociateIfUnclaimed markdown
  CreateShortCut "$SMPROGRAMS\MDFlick - Default Apps.lnk" "$WINDIR\explorer.exe" "ms-settings:defaultapps"
  CreateShortCut "$SMPROGRAMS\MDFlick - Uninstall.lnk" "$INSTDIR\Uninstall.exe"
  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
!macroend

!macro customUnInstall
  !insertmacro RemoveOwnedDefault md
  !insertmacro RemoveOwnedDefault markdown
  DeleteRegValue HKCU "Software\RegisteredApplications" "MDFlick"
  DeleteRegKey HKCU "${MDFLICK_CAPABILITIES}"
  DeleteRegKey /ifempty HKCU "Software\com.mdflick.app"
  Push "$SMPROGRAMS\MDFlick - Default Apps.lnk"
  Call un.DeleteShortcut
  Push "$SMPROGRAMS\MDFlick - Uninstall.lnk"
  Call un.DeleteShortcut
  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
!macroend
