# M3.4 Settings Screen

## Objective

Implement the Settings modal as a backed runtime-preferences surface for the
current editor shell.

## Architectural Boundary

Settings are owned by the frontend shell for this slice. They are session
runtime preferences, not persisted application preferences. Persistence requires
a separate native preference-store decision and should not be faked with hidden
browser storage.

## Functional Surface

- The title bar exposes a persistent `Settings` control that opens a modal over
  a scrim.
- Settings does not appear in the View ribbon because it is an application
  preference surface, not a document-view command.
- The modal follows the concept structure: left navigation, right content pane,
  setting rows, and Cancel / Save changes footer.
- Only backed sections render:
  - Editor: raw soft wrap and raw line-number visibility.
  - Appearance: light/dark theme, document font family, document font size,
    system-derived code font family, and code ligature support.
  - Shortcuts: read-only list of currently implemented shortcuts.
- Editing settings updates a draft only.
- Save applies the draft to real editor state and visible CSS.
- Cancel discards the draft and leaves the application unchanged.
- Opening Settings again reflects the current saved runtime preferences.
- Code font options come from native installed-font discovery through the Wails
  bridge, with a small fallback list only when native bindings are unavailable
  in browser tests.

## Deferred Subsystems

- Persistent preferences across launches.
- Markdown flavour selection beyond the current CommonMark + GFM status.
- Sync, Git, extension management, preview, share, export, and help settings.
- Per-language code highlighting configuration.

## Acceptance

- End-to-end tests cover opening the modal, applying backed preferences,
  discarding a draft with Cancel, and ensuring future-only sections are absent.
- Raw editor option changes still reconfigure Raw mode when the modal saves.
- Architext Release Truth is updated before completion.
