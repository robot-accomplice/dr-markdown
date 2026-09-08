package main

import "context"

// defaultHandlerUseCase backs the application-menu offer. Menu state is a
// rendering of a decision, not the decision — the guards run again here.
type defaultHandlerUseCase struct {
	native nativePort
}

// setAsDefault backs the application-menu offer. The menu item is disabled in
// exactly the two states this refuses, but menu state is a rendering of a
// decision, not the decision — a stale menu must not be able to set a handler
// from a disk image, so the guards run again here.
func (uc *defaultHandlerUseCase) setAsDefault(ctx context.Context) {
	isDefault, err := uc.native.IsDefaultMarkdownHandler(ctx)
	if err != nil {
		uc.native.ShowError(ctx, "Default Application", err.Error())
		return
	}
	switch defaultHandlerMenuState(isDefault, executablePath()) {
	case defaultHandlerIsDefault:
		return
	case defaultHandlerDiskImage:
		uc.native.ShowError(ctx, "Default Application",
			"Dr Markdown is running from a disk image. Drag it to Applications first — otherwise every .md file would open an app on a volume that gets ejected.")
		return
	}
	if err := uc.native.SetDefaultMarkdownHandler(ctx); err != nil {
		uc.native.ShowError(ctx, "Default Application", err.Error())
	}
}
