// Capture the README's still images against the seeded harness, so published media carries fabricated data only.
// Run against a live stack (moto seeded, ttyd on 7681) with `DRIVER=shot.mjs bash test/ui/run.sh`, or `make ui-shots` from the repo root.
// The dashboard is deliberately driven WITHOUT --allow-writes (see run.sh), so the footer badge in the image is the one a first run really shows.
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { openTerminal } from './harness.mjs'

const url = process.env.TTYD_URL ?? 'http://127.0.0.1:7681'
const here = dirname(fileURLToPath(import.meta.url))
const out = process.env.SHOT_DIR ?? join(here, '..', '..', 'docs')
mkdirSync(out, { recursive: true })

// 1010px wide so the terminal geometry matches the recorded demo.
// Captured at 1x on purpose: raising deviceScaleFactor changes devicePixelRatio, xterm.js recomputes how many columns fit, and the glyphs grow enough that a fixed crop cuts through them. The README shows this at its natural size instead of scaling it.
const term = await openTerminal({ url, screenshotDir: out, viewport: { width: 1010, height: 690 } })

await term.waitForText('[8]')
await term.waitForText('read-only')
await term.settle()

// The left of the footer only. The point of the image is the badge, so the empty middle and the version on the right are cropped off: a full-width still shrinks the badge to nothing in a README, and the version there is whatever dev build the harness happened to compile.
await term.screenshotClip('read-only-footer', { x: 0, y: 662, width: 300, height: 26 })

console.log(`wrote ${join(out, 'read-only-footer.png')}`)

process.exit(0)
