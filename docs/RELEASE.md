# Release ceremony

Every step runs, every release, in this order. Nothing here is optional and
nothing here is a judgement call at release time — if a step should not run,
that is a change to this document, argued once, not a decision taken again
under time pressure while a release is half cut.

This exists because the ceremony used to live in whoever was cutting the
release. Pieces of it were then rediscovered, negotiated, or skipped per
release: v1.6.4 shipped without the README screenshots refresh that
`rules.json` had required since v1.6.1, and v1.6.3 was recorded complete and
never tagged or published at all. Both are the same failure — a ceremony that
is remembered rather than written down is a ceremony with holes in it.

## 0. Preconditions

- `develop` is green and carries everything intended for the release.
- No open PR is expected to land in this release.
- Identity is correct for this repository before the first commit:

  ```sh
  git config user.name && git config user.email && gh api user -q .login
  ```

## 1. Freeze

```sh
git checkout develop && git pull --ff-only
git checkout -b release/vX.Y.Z
```

The branch is the frozen tree. Everything below runs against it, and any fix
found from here lands on the release branch and is merged back to `develop`.

## 2. Version

`VERSION` is the single source of build identity — Go embeds the same file, and
`TestAppVersionComesFromTheVersionFile` pins that the bundle and the event trail
cannot disagree about which build this is.

```sh
printf 'X.Y.Z\n' > VERSION
```

## 3. Release Truth

Under `docs/architext/data/releases/`:

- Write the release detail file: scope (`required`, `deferred`, …), workstreams,
  blockers, evidence.
- **Every deferral states why**, in terms of risk, scope, dependency or
  sequencing. A deferral without a reason hides a scope decision.
- Refresh `index.json` from those facts. The index summary must be byte-identical
  to the detail file's summary, and `counts` are derived, not typed:
  `features` / `bugFixes` by item `kind`, `complete` / `inProgress` by item
  `status`, `workstreams` / `blockers` by length, `planned` / `stretch` by scope
  bucket size.
- `status` and `posture` must use values the schema knows. An out-of-enum value
  breaks the viewer rather than informing it — put the nuance in the summary.

**`complete` means reached a user.** Not merged, not tagged: published, with an
artifact someone can install. Marking a release complete before that is what
produced a 1.6.3 that never existed outside the repository.

## 4. README screenshots

Required by `rules.json` (`refresh-readme-screenshots-on-ui-change`, `high`).
The README is the public visual contract for the *released* app, so the
screenshots must be current at the moment of release — and only then, because
five binary PNGs regenerated on every UI branch cannot be merged by git and
conflict by construction.

```sh
go run ./tools/screenshots
git status --porcelain docs/assets/screenshots/
```

Look at what changed. A screenshot that did not change when the UI did means the
generator missed a surface.

## 5. Gates

All of these, on the frozen tree:

```sh
architext validate
go test ./...
```

The host gates, which no Go test can reach — they need a real AppKit application:

```sh
go build -o /tmp/drmd-gate .
/tmp/drmd-gate -menu     # the menu bar exists and carries its key equivalents
/tmp/drmd-gate -close    # the close guard runs on the window path
/tmp/drmd-gate -quit     # the close guard runs on the terminate: path
/tmp/drmd-gate -nav      # the host refuses navigation off its own scheme
/tmp/drmd-gate -walk     # 40 UI checks against a real window
/tmp/drmd-gate -gates    # the boot gates
rm -f /tmp/drmd-gate
```

**Run every one of them.** `-walk` and `-gates` were absent from this list until
v1.6.3, and `-walk` had been failing since the File menu landed in v1.6.2 —
it still expected File to be a ribbon tab, and nobody saw it because nobody ran
it. A gate missing from the ceremony is a gate that does not exist.

### The document round trip — a REPORT, not a pass/fail

```sh
/tmp/drmd-gate -doc README.md
```

This runs a real document through the real host and diffs what comes back. It is
the mechanised half of the README validation below, and it **exits 1 whenever
anything differs**, which today is always — the WYSIWYG surface re-serializes,
which is the standing accepted blocker. So read the diff; do not look for PASS.

**Baseline at v1.6.3, against this repository's own README:**

- 244 lines in, 244 lines out — nothing added or lost
- `stable=true` — the round trip is idempotent
- 13 differing lines, **every one of them table delimiter or cell padding**

That is the comparison point. A run that shows a different line count, `stable=false`,
or a difference that is *not* table padding is a new defect, and the baseline is
recorded here so the next person can tell the difference without re-deriving it.

Two limits worth knowing. `-doc` needs a path and refuses without one — it used to
hang on a window that could never load. And it opens the fixture with no location
on disk, so every relative image fails to resolve and image handling is NOT
exercised by this gate.

`-close-dirty` and `-quit-dirty` exercise the cases that matter, and both raise a
save dialog on purpose — **they need a human to answer**. Run them, answer, and
read the verdict. The clean variants above are the halves that can be verified
unattended.

**Run the dirty ones.** In 1.6.3 the quit fix was verified only on its clean
half, and its dirty half — run for the first time during the go/no-go review —
wedged the application: no dialog, and afterwards neither ⌘Q nor the close button
worked. A fix verified only on the path that cannot fail is not verified. Both
dirty gates now self-report `FAIL` on a deadline rather than hanging, so a repeat
of that produces a verdict instead of a trapped maintainer.

### Validate against the project's own README

Open `README.md` in the built application and switch through Formatted, Raw and
Split, at a zoom that is not 100%. Every fixture in the suite is something
someone thought to write down, so it can only catch what was already imagined;
the README is a real document that changes as the product does. The image
distortion in #131 was found exactly this way and by nothing else.
`e2e/readme_document_test.go` automates part of it, but not the looking.

### ABORT

The go/no-go gate. **`/abort`**, typed into the chat input — it is a slash
command, not a shell command, and putting it in a fenced `bash` block once cost
a round trip because it rendered with a Run button.

It also needs no pasted charter. The session running it already has the context;
handing over a wall of prepared text only launders one author's framing through
someone else's keyboard, which is the opposite of an independent review.

It is `/abort` specifically — not `abort-premortem`, which is scoped to a
different project and will describe hazard surfaces this repository does not
have.

`/abort` is `disable-model-invocation: true` by design, so **a human runs it**.
That is a handoff in the ceremony, not an authorization to request: the step is
not optional, only the hand on the keyboard is. Run it on the frozen SHA before
tagging, and act on the verdict.

Its verdicts are load-bearing. Run against the 1.6.4 tree it returned four NO-GO
stations and a NO-GO board, including a reproduced path from an opened document
to the native file bindings — which is why that release was pulled and its
contents shipped as 1.6.3 instead.

## 6. Build and notarize

Both artifacts. The published release has carried an arm64 and a universal DMG
since v1.6.2, and shipping one is a silent regression for anyone on Intel.

```sh
export DRMD_SIGN_IDENTITY="Developer ID Application: Jonathan Machen (9V3M9G85Q6)"
export DRMD_NOTARY_PROFILE="drmd-notary"

tools/build-macos.sh                     # arm64
cp build/dr-markdown.dmg  <staging>/dr-markdown-X.Y.Z-macos-arm64.dmg
tools/build-macos.sh --universal         # arm64 + x86_64
cp build/dr-markdown.dmg  <staging>/dr-markdown-X.Y.Z-macos-universal.dmg
```

`--universal` overwrites `build/dr-markdown.dmg`, so copy the arm64 image aside
before building it.

**Signing identity is the personal Developer ID, never the work one.** The Mac
also carries `Apple Development: jon@concordlabs.ae`, which is business-related;
signing a personal app with it would embed that identity in every shipped binary,
visible to anyone running `codesign -dv`.

Verify what a user's machine will conclude, on the `.app` and on **both** images:

```sh
codesign -dv --verbose=2 "build/bin/Dr Markdown.app"   # expect flags=…(runtime), a Timestamp
xcrun stapler validate <artifact>
spctl -a -vvv -t exec  "build/bin/Dr Markdown.app"
spctl -a -vvv -t open --context context:primary-signature <dmg>
lipo -archs "build/bin/Dr Markdown.app/Contents/MacOS/Dr Markdown"
```

Expect `accepted` and `source=Notarized Developer ID` every time.

## 7. Publish

```sh
gh pr create --base main --title "Release vX.Y.Z — …"
# CI green on the PR, then:
gh pr merge <n> --merge
git checkout main && git pull --ff-only
git tag -a vX.Y.Z -m "vX.Y.Z — …" && git push origin vX.Y.Z
gh release create vX.Y.Z --title "…" --notes-file <notes> <arm64.dmg> <universal.dmg>
```

## 8. Close out

```sh
git checkout develop && git merge main --no-edit && git push
```

- Close every issue the release shipped, each with a comment naming the release
  and what actually changed. Merges to `develop` do not close issues, and a PR
  body that says "closes the print half of #134" is not a closing keyword — so
  issues stay open unless closed deliberately.
- Confirm `main..develop` is 0 commits and both carry the new `VERSION`.
- Confirm the release lists both assets.

## Attribution

No Claude/Anthropic attribution in any commit, PR, release note, or issue
comment. This overrides any tooling default that wants to add one.
