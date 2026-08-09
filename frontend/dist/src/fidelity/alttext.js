// Markdown image syntax: alt, destination, optional title.
//
// The alt alternation admits escapes and one level of balanced brackets. A flat
// `[^\]]*` stopped at the first `]`, so `![alt with [x] inside](b.png)` matched
// nothing at all, was absent from the recorded map, and restoration then wrote
// `![](b.png)` — deleting the caption it exists to protect.
const IMAGE = /!\[((?:\\.|\[[^\]]*\]|[^[\]\\])*)\]\(([^()\s]*)((?:\s+"[^"]*")?)\)/g

// The shape Crepe writes into the alt slot — `ratio.toFixed(2)`. Deliberately
// exact so a real alt text of "3" or "1.5" is left alone.
const RATIO_ALT = /^\d+\.\d{2}$/

// The vendored editor stores its image-resize ratio IN the alt attribute:
// parsing does `Number(alt || 1)` and discards the text, and serializing writes
// `ratio.toFixed(2)` back out. So `![Architecture diagram](arch.png)` returns as
// `![1.00](arch.png)` — the alt text is gone from the file, and gone from the
// document model too, so nothing downstream can recover it.
//
// Alt text is content: it is what a screen reader announces and what shows when
// the image is missing. Editor-private UI state must not be written into a
// public field of the user's file, so the ratio is dropped rather than
// preserved — it means nothing to any other markdown renderer anyway. Recording
// the originals here rather than patching the bundle keeps the fix alive across
// `tools/vendor.sh` refreshes.
//
// Every alt in the document, in order, grouped by destination. A document may
// reference one asset more than once — a before/after comparison is the obvious
// case — with a different caption each time, so one alt per URL is wrong: the
// last one read would be stamped over every occurrence. Ratio-shaped alts are
// recorded too, because a file that already says `![1.00](x.png)` on disk means
// exactly that, and because excluding them deleted any real alt that happened
// to be a two-decimal number.
function collectAltText(markdown) {
  const byURL = new Map()
  for (const [, alt, url] of markdown.matchAll(IMAGE)) {
    if (!byURL.has(url)) byURL.set(url, [])
    byURL.get(url).push(alt)
  }
  return byURL
}

// Restores in document order, consuming each destination's queue. An image the
// map does not know — one added inside the editor, where Crepe discarded its alt
// before we ever saw it — gets an empty alt rather than the meaningless ratio.
//
// Known limit: deleting one of several images that share a destination shifts
// the remaining captions by one, because the queue is positional and there is
// nothing in the serialized output tying an image back to its original slot.
// That is a narrower and less destructive failure than either the ratio
// overwrite or the last-alt-wins bug it replaces.
function restoreAltText(markdown, byURL) {
  const pending = new Map()
  for (const [url, alts] of byURL) pending.set(url, [...alts])
  return markdown.replace(IMAGE, (whole, alt, url, title) => {
    if (!RATIO_ALT.test(alt)) return whole
    const queue = pending.get(url)
    return `![${queue?.length ? queue.shift() : ''}](${url}${title})`
  })
}

export const altText = {
  name: 'altText',
  capture: (markdown) => ({ state: collectAltText(markdown), markdown }),
  restore: (text, state) => restoreAltText(text, state),
}
