// Adapter over the native bindings the host installs as globalThis.drmd.native.
//
// Resolved lazily so e2e tests can install a stub at any time, and so the app
// degrades gracefully in a plain browser with no host at all.
//
// CAUTION, and it is load-bearing: the optional chaining below guards the HOST
// being absent, not a METHOD being missing. Several entries are written
// `native()?.Method(x)` rather than `native()?.Method?.(x)`, so once the object
// exists a missing method is a TypeError and boot dies. A host must therefore
// install the WHOLE surface or none of it — there is no partial host.
const native = () => globalThis.drmd?.native ?? null

function missing(name) {
  console.warn(`bridge: native binding unavailable for ${name} (no host?)`)
  return null
}

export const bridge = {
  available: () => native() !== null,
  openDocument: () => native()?.OpenDocument() ?? missing('OpenDocument'),
  saveDocument: (path, content) =>
    native()?.SaveDocument(path, content) ?? missing('SaveDocument'),
  saveDocumentAs: (content) =>
    native()?.SaveDocumentAs(content) ?? missing('SaveDocumentAs'),
  openRecentDocument: (path) =>
    native()?.OpenRecentDocument?.(path) ?? missing('OpenRecentDocument'),
  importImage: (documentPath) =>
    native()?.ImportImage?.(documentPath) ?? missing('ImportImage'),
  importDroppedImage: (documentPath, sourcePath) =>
    native()?.ImportDroppedImage?.(documentPath, sourcePath) ?? missing('ImportDroppedImage'),
  loadImageAsset: (documentPath, markdownPath) =>
    native()?.LoadImageAsset?.(documentPath, markdownPath) ?? missing('LoadImageAsset'),
  recordEvent: (event, fields) => native()?.RecordClientEvent?.(event, fields ?? {}),
  // Files macOS routed to us before this webview existed. Launching by
  // double-click delivers the file first, so the frontend has to ASK for it
  // rather than wait to be told — an event sent before boot reaches nobody.
  frontendReady: () => native()?.FrontendReady?.() ?? Promise.resolve([]),
  openExternalURL: (url) => native()?.OpenExternalURL?.(url) ?? missing('OpenExternalURL'),
  revealImageAsset: (documentPath, markdownPath) =>
    native()?.RevealImageAsset?.(documentPath, markdownPath) ?? missing('RevealImageAsset'),
  setDirty: (d) => native()?.SetDirty(d),
  // Explicit rather than an optional call: this is the only binding whose
  // silent absence re-enables a data-loss bug, so it must warn like the rest.
  // A stale generated binding lacking SyncDocuments used to no-op here, leaving
  // Go with no documents at all.
  syncDocuments: (docs) => {
    const app = native()
    if (!app?.SyncDocuments) return missing('SyncDocuments')
    return app.SyncDocuments(docs)
  },
  updateContent: (c) => native()?.UpdateContent(c),
  listFontFamilies: () => native()?.ListFontFamilies() ?? missing('ListFontFamilies'),
  loadPreferences: () => native()?.LoadPreferences?.() ?? null,
  savePreferences: (prefs) => native()?.SavePreferences?.(prefs) ?? null,
  resolveUnsavedChanges: () =>
    native()?.ResolveUnsavedChanges() ?? missing('ResolveUnsavedChanges'),
}
