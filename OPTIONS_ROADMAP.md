# Options menu

Add [OPTIONS] below [THEMES] on the splash screen. Use a split-pane browser based on Themes, with a category list, scrollable details, theme-aware backgrounds, and a draggable divider where practical. Reuse shared scrollbar and hitbox geometry. Do not add a separate room browser in this feature.

## Settings

- General: username, username color, bell (On / Off / Mention), show splash on startup, random splash logo.
- Chat: show timestamps, 12/24-hour timestamp format, signed hour/minute offset, mention highlighting, confirm mouse-click Quit only.
- Composer: existing input_height (1–8), adaptive_height (boolean), height_max_percent (1–100, numeric stepper, disabled when adaptive height is off).
- ChatFX: enable/disable, FPS (existing supported limits), individual local effect allowlist checkboxes.
- Advanced: mouse support, config path, reload config, restore non-theme defaults with confirmation.

Do not make typing indicators, broadcast typing status or join/leave notices optional. Room selection remains CLI-only. Do not add new ChatFX engine settings beyond local effect filtering and persistence of existing controls.

## Behavior

Bell Mention mode covers direct mentions, received whispers and replies to one's own messages. Preserve legacy boolean TOML bell values. Inspect timestamp source and formatting before applying an offset; never mutate wire timestamps. Keep Ctrl+C unchanged.

Use a staged configuration draft with local visual previews when safe, explicit save and cancel, and confirmation for discarding dirty changes. Do not send username/color changes to the server before save. Use targeted config writes that preserve comments, aliases, unknown settings and theme data. Keep ChatFX command and menu state consistent. Preserve existing defaults unless expressly changed. Provide a way to recover keyboard navigation when mouse support is off.

## Implementation order

1. Inspect existing panel dispatch, focus, sizing, hitboxes, and shared scrollbar helpers. Add Options entry and split-pane shell; test narrow terminals and Escape/back behavior.
2. Implement reusable toggles, numeric steppers, choices and text editing, leveraging existing theme editor where possible. Bind existing settings to config draft and targeted persistence.
3. Add new persistent preferences and runtime behavior, with tests for bell modes, timestamp offsets, splash behavior, mention highlighting, mouse-only quit confirmation and ChatFX filters.
4. Test config compatibility, resize/scrolling, mouse-disabled keyboard recovery, theme isolation and command/menu consistency. Run gofmt, go test ./... and make before release.

Avoid broad refactors and duplicated render/input helpers. Main remains unchanged until reviewed and tested.
