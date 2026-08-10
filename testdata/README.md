# Round-trip corpus (testdata/roundtrip/)

Lives one level up from the fixtures on purpose: the driver globs
`testdata/roundtrip/*.md`, so a README inside that directory would be run
through the editor as if it were a fixture.

Fixtures driven by `TestRoundTripCorpus`. Two contracts, distinguished by the
filename:

| filename | contract |
| -------- | -------- |
| `*.canonical.md` | must come back **byte-identical** — already in the spelling the editor emits |
| `*.md` | must be **stable** — the second pass must equal the first, even if the first rewrote something |

Stability is the weaker contract but it is the one that matters most to a user:
a document that keeps changing on every save grows, and that is how the footnote
duplication bug grew a file by one definition per save before anyone noticed.

## 01–22 are deliberately atomic. 23 is deliberately not.

Fixtures 01 through 22 hold one construct each, four to sixteen lines. That
makes a failure trivial to localise, and it is why they were written that way.

It also means they cannot catch anything that happens BETWEEN constructs, and
some defects only exist there. `23-realistic-document.md` is a document shaped
like one a person would actually write — 86 lines, frontmatter, prose, an
ordered list with a continuation line, two fenced blocks back to back, a fenced
block containing tabs, comments and blank lines, a blockquote, a padded table, a
thematic break, a bullet list, and a link reference definition at the end.

It found something on its first run that twenty-two atomic fixtures had not: a
link reference definition preceded by other content comes back with an extra
blank line before it. `17-link-refs.canonical.md` cannot catch this, because it
is nothing BUT definitions — there is no preceding content for the appended
block to be separated from.

That is why 23 is not `.canonical.md`. It is stable, and it is honest about the
one thing it still loses.

## Adding a fixture

Run it through the real editor first and look at what comes back:

```sh
go build -o /tmp/drmd . && /tmp/drmd -doc testdata/roundtrip/<file>.md
```

If the output is byte-identical, name it `.canonical.md`. If it is merely
stable, leave it `.md` and say in this file what it loses and why that is
accepted. Do not adopt the editor's output as the fixture just to make it
canonical: that bakes the defect in and retires the evidence for it.
