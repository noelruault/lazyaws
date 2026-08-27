// Journey runner: every file in journeys/ exports run({ term, seed, endpoint }), and a throw is a failure.
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { openTerminal } from './harness.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const url = process.env.TTYD_URL ?? 'http://127.0.0.1:7681'
const screenshotDir = process.env.UI_TEST_SCREENSHOTS ?? join(here, '.screenshots')
const seed = JSON.parse(readFileSync(process.env.UI_TEST_SEED ?? join(here, '.seed.json'), 'utf8'))
// The fake AWS endpoint, for the journeys that have to change what the app will read next. Absent when a runner drives this directly, and a journey that needs it says so itself.
const endpoint = process.env.AWS_ENDPOINT_URL

const only = process.argv.slice(2)
const names = readdirSync(join(here, 'journeys'))
  .filter(f => f.endsWith('.mjs'))
  .filter(f => only.length === 0 || only.includes(f) || only.includes(f.replace(/\.mjs$/, '')))
  .sort()

if (names.length === 0) {
  console.error(only.length ? `no journey matches ${only.join(', ')}` : 'no journeys found')
  process.exit(1)
}

let failed = 0
for (const name of names) {
  const journey = await import(join(here, 'journeys', name))
  // One terminal per journey: a journey that leaves a popup open must not decide what the next one sees.
  const term = await openTerminal({ url, screenshotDir })
  const started = Date.now()
  try {
    await journey.run({ term, seed, endpoint })
    console.log(`ok    ${name} (${Date.now() - started}ms)`)
  } catch (err) {
    failed++
    console.error(`FAIL  ${name} (${Date.now() - started}ms)\n${err.message}`)
    await term.screenshot(`${name.replace(/\.mjs$/, '')}-failure`).catch(() => {})
  } finally {
    await term.close()
  }
}

console.log(`${names.length - failed}/${names.length} journeys passed`)
process.exit(failed === 0 ? 0 : 1)
