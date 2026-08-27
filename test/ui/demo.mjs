// Record the README demo GIF's frames against the seeded harness, so the published media carries fabricated data only.
// Run against a live stack (moto seeded, ttyd on 7681): `bun demo.mjs`, then assemble with ffmpeg — see docs/demo-README.md.
// Each shot() is one GIF frame with its hold in seconds; the concat list ffmpeg needs is written beside the frames.
import { appendFileSync, mkdirSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { openTerminal } from './harness.mjs'

const url = process.env.TTYD_URL ?? 'http://127.0.0.1:7681'
// Beside this script rather than the CWD, so `make ui-demo` from the repo root and a hand run from test/ui write the same place.
const dir = process.env.DEMO_DIR ?? join(dirname(fileURLToPath(import.meta.url)), '.demo-frames')
rmSync(dir, { recursive: true, force: true })
mkdirSync(dir, { recursive: true })

// The README embeds at 900px; 1010x690 is what the previous vhs tape rendered at, kept so the swap is invisible.
const term = await openTerminal({ url, screenshotDir: dir, viewport: { width: 1010, height: 690 } })

let n = 0
async function shot (hold) {
  const name = `f${String(n++).padStart(3, '0')}`
  await term.screenshot(name)
  appendFileSync(`${dir}/frames.txt`, `file '${name}.png'\nduration ${hold}\n`)
}

await term.waitForText('[8]')
await term.settle()
await shot(2)

// ECS: the cluster overview with the deployment badge and the running image requirement.
await term.sendKeys('2')
await term.waitForText('ECS Cluster')
await term.settle()
await shot(2.5)

// EC2: the single-tab overview, sections and console availability in one pane.
await term.sendKeys('3')
await term.waitForText('EC2 Instance')
await term.settle({ timeout: 10000 })
await shot(3)

// Filter: the pane follows the selection the filter lands on.
await term.sendKeys('/')
await term.type('web')
await term.waitForText('filter: web')
await shot(1.5)
await term.sendKeys('Enter')
await term.waitForText('ui-web-1  ● running')
await term.settle({ timeout: 10000 })
await shot(2.5)
await term.sendKeys('Escape')
// Escape returns the selection to the head of the list, and the pane refetches before it repaints; without this wait the popup frame sits on an empty pane.
await term.waitForText('(no name)  ● running')
await term.settle({ timeout: 10000 })

// Copy: the full id in a popup, since the rows truncate identifiers.
await term.sendKeys('y')
await term.waitForText('id / ARN (select to copy)')
await shot(2)
await term.sendKeys('Escape')

// Secrets: rotation badge, versions, policy posture.
await term.sendKeys('7')
await term.waitForText('▣ Secret')
await term.settle({ timeout: 10000 })
await shot(2.5)

// VPC: topology counts on the overview.
await term.sendKeys('8')
await term.waitForText('⇄ VPC')
await term.settle({ timeout: 10000 })
await shot(2.5)

// ffmpeg's concat demuxer ignores the last duration unless the final frame repeats.
await shot(1)

await term.close()
console.log(`${n} frames in ${dir}`)
