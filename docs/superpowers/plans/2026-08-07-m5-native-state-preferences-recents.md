# M5 Native State, Preferences, And Recents Foundation

GitHub issue: <https://github.com/robot-accomplice/dr-markdown/issues/8>

## Objective

Make the native app state durable and testable before adding more document
features. The Wails-bound API should remain the frontend contract, but Wails
runtime calls must be isolated behind ports so application decisions can be
unit tested without launching the desktop runtime.

## Architecture

1. Extract native ports from `app.go` for dialogs, title updates, document IO,
   font discovery, and preference persistence.
   - Verify with Go unit tests using fakes instead of Wails runtime globals.
2. Add a JSON preference store under the user's config directory.
   - Verify load, missing-file defaults, save, corrupt-file failure, and
     recent-file de-duplication with unit tests.
3. Extend the frontend bridge with load/save preference and recent-open calls.
   - Verify with e2e bridge stubs that boot applies persisted settings and
     Settings Save writes the native store.
4. Render recent files on the start screen and allow opening one without a
   native file picker.
   - Verify with e2e bridge stubs that recent rows render and click through to
     the native bridge.
5. Keep README screenshots current for UI-facing changes.
   - Verify with `go run ./tools/screenshots` and committed images under
     `docs/assets/screenshots/`.

## Test Plan

- Red first: unit tests for native app save/open/title/close behavior with
  injected fakes.
- Red first: unit tests for preference JSON persistence and recents ordering.
- Red first: e2e tests for persisted settings boot/save and recent files.
- Green: implement only the required seams and UI wiring.
- Broader verification:
  - `go test ./... -count=1`
  - non-e2e Go coverage at or above 80% LOC
- `node --check frontend/dist/src/app.js`
- `go run ./tools/screenshots`
- `architext validate .`

## Acceptance

- Settings persist across application launches through native storage.
- Recents are recorded by native open/save flows, rendered on the start screen,
  and open through a specific recent-file bridge method.
- Wails runtime concerns are adapter code; app decisions are covered by unit
  tests.
- Architecture data and Release Truth reflect the new preference/recents
  foundation.
- README screenshots are generated from the current UI and linked from
  `README.md`.
