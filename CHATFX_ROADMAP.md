# ChatFX roadmap

ChatFX gives a room atmosphere without turning Zealot into a networked animation
service. The daemon broadcasts *intent* (effect, duration, scope), and each
terminal animates independently. No animation frames, particles, or positions
ever cross the Unix socket.

## Rules

- Background FX is the default. Render only inside the chat viewport, without
  covering message glyphs, selection controls, scrollbars, or click targets.
- One room-wide **primary** effect at a time. Starting another replaces it.
- A **secondary** effect may coexist with one primary effect, but secondary
  effects should remain individually bounded and opt-out friendly.
- A local `self` override affects only that client. When it stops, room FX
  becomes visible again.
- `/disablefx` is a client-side opt-out; it does not change room state.
- Use bounded particle counts, avoid chat reflow, throttle redraws when panels
  are open, and do no rendering work when effects are disabled.
- Always allow an effect to be stopped. Use server-validated durations rather
  than trusting a client's animation clock.

## Delivery stages

### Stage 1 — Rain and the FX transport (in progress)

- `/rain [seconds]` and `/rain self [seconds]`
- `/stop`, `/stopfx self`, `/disablefx`
- Room-wide state broadcast, self-only mode, optional 1–300 second duration,
  expiry, state for new joiners
- A client-rendered rain effect confined to unoccupied chat cells
- Manual testing on multiple local clients and performance benchmarking

### Stage 2 — Weather and primary-effect control

- Add `/snow`, `/blizzard`, and `/embers`
- Define primary-effect replace semantics across room and local scopes
- Add consistent adjustable density and appropriate default motion
- Handle terminal resizing, reconnecting and changing rooms

### Stage 3 — Brief celebrations

- Add `/fireworks` and `/confetti` with short default durations
- Permit optional durations and cap particle and flash rates
- Introduce an accessibility-aware reduced-motion behavior

### Stage 4 — Secondary effects

- `/static`: occasional flicker/interference, stackable with one primary
- `/terminal`: progressive message reveal; must not delay delivery or
  alter message history, selection offsets, search, or hitboxes
- Secondary effects must be non-destructive and individually stoppable
- Decide if `/interrogation` is a primary light treatment or a secondary
  treatment; ensure contrast and readable text first

### Stage 5 — Options menu and rendering quality

- Global settings menu for ChatFX preferences
- `Background FX` (default) / `Foreground FX` (deliberate opt-in)
- Quality options: off, low, normal, high; independent density/fps limits
- Optional high-quality glow for embers, with a firm CPU budget
- Accessibility and reduced-motion preferences; persistent local settings
- Guardrails to ensure FX never intercepts mouse or keyboard input, even
  in foreground mode

## Release gate

Before merging a ChatFX feature branch, run `go test ./...`,
`go test -race ./...`, and `go vet ./...`, plus manual testing with two
clients in the same room. Test that the other client receives intent, not
synchronized frames; verify local opt-out and stop commands, effect expiry,
joining mid-effect, resizing, and handling slow SSH terminals.
