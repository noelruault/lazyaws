// Screenshot every panel's Overview at the wide layout, for eyeballing against the design mockups. Not a journey — run directly: `bun shots.mjs` against a live stack (SHOT_DIR overrides the output dir).
import { openTerminal } from './harness.mjs'

const url = process.env.TTYD_URL ?? 'http://127.0.0.1:7681'
const term = await openTerminal({ url, screenshotDir: process.env.SHOT_DIR ?? '.shots', viewport: { width: 1700, height: 1000 } })

const panels = [
  { key: '2', name: 'ecs', wait: 'Cluster' },
  { key: '3', name: 'ec2', wait: 'Instance' },
  { key: '4', name: 's3', wait: 'Bucket' },
  { key: '6', name: 'ecr', wait: 'Repository' },
  { key: '7', name: 'secret', wait: 'Secret' },
  { key: '8', name: 'vpc', wait: 'VPC' },
]

await term.waitForText('[8]')
await term.screenshot('00-boot')
for (const p of panels) {
  await term.sendKeys(p.key)
  await term.waitForText(p.wait, { timeout: 20000 })
  await term.settle()
  await term.screenshot(`overview-${p.name}`)
}
await term.close()
console.log('shots done')
