// Measure the user-perceived first paint of each overview: keypress to header, then to the last section, polled every 25ms.
// Not a journey — run directly against a live stack: `bun paint-timing.mjs`.
import { openTerminal } from './harness.mjs'

const url = process.env.TTYD_URL ?? 'http://127.0.0.1:7681'
const term = await openTerminal({ url, screenshotDir: '.demo-frames', viewport: { width: 1700, height: 1000 } })

async function until (needle, deadlineMs = 30000) {
  const t0 = performance.now()
  while (performance.now() - t0 < deadlineMs) {
    if ((await term.readScreen()).includes(needle)) return Math.round(performance.now() - t0)
    await term.page.waitForTimeout(25)
  }
  throw new Error(`never saw ${JSON.stringify(needle)}`)
}

await term.waitForText('[8]')
await term.settle()

const panes = [
  { key: '3', name: 'EC2', header: '◇ EC2 Instance', last: 'Screenshot:' },
  { key: '7', name: 'secret', header: '▣ Secret', last: 'Resource policy' },
  { key: '2', name: 'ECS cluster', header: '⬡ ECS Cluster', last: '◇ Tags' },
  { key: '8', name: 'VPC', header: '⇄ VPC', last: '◇ Endpoints' },
]

for (const p of panes) {
  await term.sendKeys(p.key)
  const header = await until(p.header)
  const full = await until(p.last)
  console.log(`${p.name}: header ${header}ms, full pane ${full + header}ms`)
}

// Second EC2 selection: the extras memo makes repeat selections of the SAME instance free; a different instance pays the fetch again.
await term.sendKeys('3')
await term.sendKeys('ArrowDown')
const t = await until('ui-db-1  ● running')
console.log(`EC2 next instance: header+pane ${t}ms`)

await term.close()
