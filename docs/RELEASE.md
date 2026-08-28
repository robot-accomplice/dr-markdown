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
go build -o /tmp/drmd-menu . && /tmp/drmd-menu -menu && rm -f /tmp/drmd-menu
```

Then the ABORT premortem, pinned to the frozen commit:

```sh
git rev-parse HEAD    # pass this as {sha}
```

`Workflow({ name: "abort-premortem", args: {sha, branch, tag, repo, charter} })`

The premortem is part of the ceremony. It is not authorized per release.

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
