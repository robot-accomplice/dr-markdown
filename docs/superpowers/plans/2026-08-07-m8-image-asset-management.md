# M8 Image Asset Management

GitHub issue: <https://github.com/robot-accomplice/dr-markdown/issues/9>

## Objective

Replace placeholder image insertion with native image import and a portable
asset-copy policy for saved markdown documents.

## Architecture

1. Add a pure asset manager under `internal/assets`.
   - Verify relative markdown path generation, asset directory naming, collision
     suffixes, and source validation with unit tests.
2. Add a Wails API method for image import.
   - Verify app tests can route selected image paths through an injected native
     image picker and asset manager.
3. Route the existing Image ribbon command through the bridge.
   - Verify e2e stubs insert returned image markdown instead of a placeholder.

## Asset Policy

- Saved document: copy selected image into `<document-name>.assets/` beside the
  markdown file and insert a relative markdown image path.
- Filename collision: append `-1`, `-2`, etc. before the extension.
- Unsaved document: reject import until the document has a saved path. This is
  blunt, but it preserves portability instead of silently inserting absolute
  local paths.

## Test Plan

- Red first: Go unit tests for asset import policy.
- Red first: e2e test for the Image ribbon command using a bridge stub.
- Green: implement asset manager, app bridge method, and frontend command path.
- Refresh README screenshots after visible UI changes.

## Acceptance

- Image ribbon command opens a backed native import path.
- Imported image markdown points at a copied relative asset for saved documents.
- Unsaved documents do not get non-portable absolute image links.
- Collision handling is deterministic and tested.
