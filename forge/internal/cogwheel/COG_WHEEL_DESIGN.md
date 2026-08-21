# Cog Wheel — Shared Visual Design (forge-tui ↔ cogitator-browser)

> **Goal:** 100% visual identity between the Bubble Tea TUI and the
> Electron app.  Any change to the cog wheel here must be mirrored in
> the cogitator-browser CSS.

The cog wheel is the central visual symbol of **Dark Mechanicus**
(Warhammer 40k Adeptus Mechanicus).  The TUI side uses raw
box-drawing characters; the browser side uses the same characters
inside a CSS-styled container.  Same glyphs, same proportions.

---

## Compact Layout (3 lines × 5 cols)

Default for inline use (status bar, header, footer).

```
╓───╖
║ ◉ ║
╙───╜
```

Glyphs:
- `╓ ─── ╖` — top bracket with horizontal teeth
- `║ ◉ ║` — middle row, filled circle (◉) is the hub
- `╙ ─── ╜` — bottom bracket

Color states (Cogitator Gold is the default):
- **Idle / Thinking** → `var(--cogitator-gold)` = `#C8A84B`
- **Working** → `var(--noosphere-cyan)` = `#00BFBF`
- **Error** → `var(--omnissiah-red)` = `#FF0000`
- **Success** → `#39FF14`

---

## Wide Layout (3 lines × 15 cols)

Welcome screen only.  Shows teeth + hub with spokes.

```
╓─╖ ╓─╖ ╓─╖ ╓─╖
║◉║║ ║║◉║║ ║
╙─╜ ╙─╜ ╙─╜ ╙─╜
```

The compact hub glyph (◉) is repeated 3 times across the line for a
wider silhouette.  Same color rules as compact.

---

## Animation (Thinking state)

The hub glyph cycles between `◉` (filled) and `◈` (dotted) every
**200ms**.  Only the **inner glyph** changes — the bracket / teeth
stay static.

```css
/* cogitator-browser side: CSS keyframes */
@keyframes cog-spin {
  0%, 49%   { content: "◉"; }
  50%, 100% { content: "◈"; }
}

.cog-hub::before {
  content: "◉";
  animation: cog-spin 400ms steps(1) infinite;
}
```

```go
// forge-tui side (cogwheel package):
opts.Now = time.Now()  // 200ms cycle derived from UnixMilli()/200 % 2
```

The whole cog never "spins" in 2D — only the central hub ticks.  This
is a deliberate Dark Mechanicus choice: spinning cogs are a 2010s
skeumorphism cliche.  A pulsing hub reads as "the machine is
thinking" without the visual noise.

---

## Cogitator-Browser (TypeScript / React) Implementation Guide

```tsx
// src/renderer/components/CogWheel.tsx
import React from 'react';
import './CogWheel.css';

type CogState = 'idle' | 'thinking' | 'working' | 'error' | 'success';

interface CogWheelProps {
  state?: CogState;
  variant?: 'compact' | 'wide';
}

const HUB_FRAMES = ['◉', '◈']; // alternates every 200ms

export function CogWheel({ state = 'idle', variant = 'compact' }: CogWheelProps) {
  // StyleWide uses a wider silhouette
  return (
    <div className={`cog-wheel cog-${variant} cog-state-${state}`}>
      <span className="cog-bracket cog-bracket-top">╓───╖</span>
      <span className="cog-hub">{HUB_FRAMES[0]}</span>
      <span className="cog-bracket cog-bracket-bottom">╙───╜</span>
    </div>
  );
}
```

```css
/* CogWheel.css */
:root {
  --cogitator-gold: #C8A84B;
  --noosphere-cyan: #00BFBF;
  --omnissiah-red: #FF0000;
  --text-success: #39FF14;
}

.cog-wheel {
  font-family: 'Courier New', monospace;
  display: inline-block;
  text-align: center;
  color: var(--cogitator-gold);
  filter: drop-shadow(0 0 6px var(--cogitator-gold));
  animation: cog-pulse 2s ease-in-out infinite;
}

.cog-state-thinking { animation: cog-pulse 1s ease-in-out infinite; }
.cog-state-working  { color: var(--noosphere-cyan); }
.cog-state-error    { color: var(--omnissiah-red); }
.cog-state-success  { color: var(--text-success); }

.cog-state-thinking .cog-hub::before {
  content: '◉';
  animation: cog-think 400ms steps(1) infinite;
}

@keyframes cog-think {
  0%, 49%   { content: '◉'; }
  50%, 100% { content: '◈'; }
}

@keyframes cog-pulse {
  0%, 100% { opacity: 1.0; }
  50%      { opacity: 0.6; }
}
```

### React + electron-store integration

```tsx
// In renderer/App.tsx — embed the cog in the header
import { CogWheel } from './components/CogWheel';

<header className="dm-header">
  <CogWheel state={cogState} variant="compact" />
  <span className="dm-title">⚒ HereticForge v1.1.0</span>
  <span className="dm-version">MMXXVI</span>
</header>
```

---

## Anti-patterns (DO NOT do these)

- ❌ **Spinning 3D cogs** — 2010s skeuomorphism, looks dated
- ❌ **Animated border glows on the cog itself** — visual noise
- ❌ **Color-changing on every state** — only 4 semantic states allowed:
  idle (gold), thinking (gold, animated), working (cyan), error (red)
- ❌ **Different glyphs than ◉/◈** — keep the hub glyph consistent
- ❌ **Adding teeth/cogs as decoration** — the 3-line compact is the
  canonical minimal form

---

## Source of truth

**`heretic-forge/internal/cogwheel/cogwheel.go`** is the canonical
Go implementation.  All visual decisions originate there:

- Color palette (from cogitator-browser/src/renderer/styles/theme.css)
- Glyph choices (◉ hub, ◈ thinking-alt, ╓╖╙╜ brackets)
- Layout dimensions (compact 3x5, wide 3x15)
- Animation timing (200ms hub cycle)

If a change is needed, edit cogwheel.go FIRST, then mirror to
cogitator-browser/src/renderer/components/CogWheel.tsx + .css.

---

## Testing

```go
// heretic-forge/internal/cogwheel/cogwheel_test.go
func TestRender_Idle(t *testing.T) { ... }      // 3-line, gold
func TestRender_ThinkingCyclesGlyph(t *testing.T) { ... }  // 200ms cycle
func TestRenderCentered(t *testing.T) { ... }  // padding
func TestRender_StateColor_AllStates(t *testing.T) { ... }  // 5 states
```

**Visual regression:** the cogitator-browser side should snapshot
the cog at each state in `tests/components/CogWheel.test.tsx` and
assert the rendered output matches the canonical ASCII art above.

---

*«Veritas in Crypta. Ordo ab Chao.»*
