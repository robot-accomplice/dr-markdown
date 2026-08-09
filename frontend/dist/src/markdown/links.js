// Link href safety.
//
// The check must be run on the string the BROWSER will navigate to, not on the
// string as written: the URL parser strips ASCII tab, LF and CR from anywhere
// in a URL before parsing, so `jav<TAB>ascript:` has no scheme by a regex's
// reading and `javascript:` by the parser's. normalizeHref does that stripping
// first so both readings agree.
//
// This matters more here than in a browser tab: a javascript: URL runs in the
// app's own origin, where the Wails bindings expose SaveDocument and
// OpenRecentDocument with no path restriction. This product exists to open
// ARBITRARY markdown, so every document is untrusted input.

// Schemes a document may link to. Anything else — javascript:, data:, file:,
// vbscript:, or a scheme invented later — is refused rather than filtered,
// because a denylist of dangerous schemes is a list you can always add to.
export const SAFE_LINK_SCHEMES = ['http:', 'https:', 'mailto:']

// The URL parser removes every ASCII tab, LF and CR from a URL, and ignores
// leading and trailing C0 controls and spaces, BEFORE it parses the scheme. Any
// check that reads the raw string is therefore checking a different string than
// the one the browser navigates to: `jav<TAB>ascript:` carries no scheme by a
// regex's reading and `javascript:` by the parser's. That is how the previous
// check — which looked for a scheme pattern and treated its absence as "this is
// a relative link" — passed four separate spellings of javascript: through.
export function normalizeHref(href) {
  return String(href).replace(/[\t\n\r]/g, '').replace(/^[\x00-\x20]+|[\x00-\x20]+$/g, '')
}

// safeLinkHref returns the href to assign, or null to refuse. Returning the
// normalized value rather than a boolean is the point: the caller cannot
// validate one string and then assign another, which is the whole defect class
// above rather than one instance of it.
//
// Every candidate is resolved against a base, so there is no "looks relative"
// branch to slip past — a genuine relative link resolves to the base's https:
// and is allowed on the same rule as everything else.
export function safeLinkHref(href) {
  const value = normalizeHref(href)
  try {
    const resolved = new URL(value, 'https://example.invalid')
    return SAFE_LINK_SCHEMES.includes(resolved.protocol) ? value : null
  } catch {
    return null
  }
}
