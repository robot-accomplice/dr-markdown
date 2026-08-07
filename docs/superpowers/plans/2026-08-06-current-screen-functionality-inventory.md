# M3.1 Current Screen Functionality Inventory

## Objective

Make the current editor shell screen honest and usable before moving to the next screen. A visible enabled control must be backed by behavior the application can perform today. Controls for future subsystems should not appear as decorative, enabled UI.

## Architectural Boundary

This screen is the document editor shell inside a Wails macOS application. The webview owns editor state, markdown transforms, local document rail state, outline extraction, and mode switching. Native filesystem operations stay behind the Wails bridge. Platform sharing, export formats, file-system workspace indexing, comments, and full rendered split preview are separate subsystems and must not be faked in this pass.

## Functional Surface

- Title area: document title/save state must reflect the active buffer.
- Mode controls: Formatted and Raw must switch the active editor without losing markdown.
- Ribbon tabs: Home, Insert, Format, and View must expose backed commands only.
- Home ribbon: structure, text, list, quote, link, table, code block, and diagram actions must mutate the active markdown buffer.
- Home ribbon formatting commands must transform the current selection or current block. Heading buttons and the paragraph-style dropdown are formatting controls, not insert controls, and must not append a new heading when text is selected or when the cursor is inside an existing paragraph.
- List, quote, and code-block controls must transform the current block when the cursor is inside an existing paragraph. Insert behavior is acceptable only when no editor selection/current block exists.
- Insert ribbon: block insertion commands must mutate the active markdown buffer.
- Format ribbon: markdown formatting commands must mutate the active markdown buffer.
- View ribbon: raw toggle, focus mode, and theme toggle must update the workspace.
- File rail: visible rows must represent actual open buffers; search must filter those buffers; New document must create a blank buffer.
- Empty state: Start writing, Open a file, Paste markdown, and templates must perform real actions.
- Outline side panel: Outline, Comments, and Links tabs must switch real panel content. Outline and Links should derive from markdown. Comments should truthfully show that no local comments exist yet.
- Status bar: mode, position placeholder, syntax dialect, and local sync state must reflect application state.

## Removed From This Screen Until Subsystems Exist

- Preview: formatted mode already fills the preview role; a separate preview command needs a defined preview surface.
- Share: native macOS share requires a bridge-backed share subsystem.
- Export: export format selection and file writing need a defined export subsystem.
- Help tab: reference content and shortcut documentation need their own backed screen/panel.
- Fake docs rows: the rail must not imply a workspace index before one exists.
- Decorative Split toggle: split mode must not appear until it can show synchronized raw/formatted panes.

## Acceptance

- No enabled visible button is decorative.
- Opening, editing, switching modes, creating documents, filtering documents, templates, and outline/link panels are covered by end-to-end tests.
- The screen launches without example content.
- The macOS default app window still shows the full backed ribbon surface.
