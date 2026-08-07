// Adapter over the Wails bindings (window.go.main.App in the real app).
// Resolved lazily so e2e tests can install a stub at any time, and the app
// degrades gracefully in a plain browser.
const wails = () => globalThis.go?.main?.App ?? null

function missing(name) {
  console.warn(`bridge: Wails binding unavailable for ${name} (not running under Wails?)`)
  return null
}

export const bridge = {
  available: () => wails() !== null,
  openDocument: () => wails()?.OpenDocument() ?? missing('OpenDocument'),
  saveDocument: (path, content) =>
    wails()?.SaveDocument(path, content) ?? missing('SaveDocument'),
  saveDocumentAs: (content) =>
    wails()?.SaveDocumentAs(content) ?? missing('SaveDocumentAs'),
  openRecentDocument: (path) =>
    wails()?.OpenRecentDocument?.(path) ?? missing('OpenRecentDocument'),
  importImage: (documentPath) =>
    wails()?.ImportImage?.(documentPath) ?? missing('ImportImage'),
  importDroppedImage: (documentPath, sourcePath) =>
    wails()?.ImportDroppedImage?.(documentPath, sourcePath) ?? missing('ImportDroppedImage'),
  loadImageAsset: (documentPath, markdownPath) =>
    wails()?.LoadImageAsset?.(documentPath, markdownPath) ?? missing('LoadImageAsset'),
  revealImageAsset: (documentPath, markdownPath) =>
    wails()?.RevealImageAsset?.(documentPath, markdownPath) ?? missing('RevealImageAsset'),
  setDirty: (d) => wails()?.SetDirty(d),
  updateContent: (c) => wails()?.UpdateContent(c),
  listFontFamilies: () => wails()?.ListFontFamilies() ?? missing('ListFontFamilies'),
  loadPreferences: () => wails()?.LoadPreferences?.() ?? null,
  savePreferences: (prefs) => wails()?.SavePreferences?.(prefs) ?? null,
  resolveUnsavedChanges: () =>
    wails()?.ResolveUnsavedChanges() ?? missing('ResolveUnsavedChanges'),
}
