// Playwright cannot type into a pty, so every journey drives ttyd's xterm.js page instead.
import { mkdirSync } from 'node:fs'
import { chromium } from 'playwright'

// ttyd sizes the pty from the browser viewport, so the viewport is how a journey chooses its terminal width.
// 1280x720 lands on 160x47, comfortably above the 110-cell threshold where overviews go two-column.
const defaultViewport = { width: 1280, height: 720 }

// The last entry of every options bar this app draws, and the cheapest proof that it has finished its first frame.
const optionsBarMarker = 'q quit'

export async function openTerminal ({ url, screenshotDir, viewport = defaultViewport } = {}) {
  const browser = await chromium.launch()
  const page = await browser.newPage({ viewport })
  await page.goto(url)
  // ttyd exposes the xterm.js instance as window.term; it exists before the pty attaches, so wait for a first frame instead.
  await page.waitForFunction(() => window.term?.buffer?.active?.length > 0)
  mkdirSync(screenshotDir, { recursive: true })

  const readScreen = () => page.evaluate(() => {
    const buf = window.term.buffer.active
    const lines = []
    // viewportY..+rows is what is ON SCREEN; buffer.length includes scrollback, which no journey asserts on.
    for (let i = 0; i < window.term.rows; i++) {
      lines.push(buf.getLine(buf.viewportY + i)?.translateToString(true) ?? '')
    }
    return lines.join('\n')
  })

  const term = {
    page,
    readScreen,
    size: () => page.evaluate(() => ({ cols: window.term.cols, rows: window.term.rows })),

    // The options bar shares the bottom row with the app status and the version, and it is contextual, so it is how a journey sees which view holds focus.
    async footer () {
      const lines = (await readScreen()).split('\n').filter(line => line.trim() !== '')
      return lines[lines.length - 1] ?? ''
    },

    // Presses are real key events through xterm's handler, so a journey exercises the same path a user does.
    async sendKeys (...keys) {
      for (const key of keys) {
        await page.keyboard.press(key)
        await page.waitForTimeout(60)
      }
    },

    async type (text) {
      await page.keyboard.type(text, { delay: 20 })
    },

    async waitForText (needle, { timeout = 15000 } = {}) {
      const deadline = Date.now() + timeout
      let screen = ''
      while (Date.now() < deadline) {
        screen = await readScreen()
        if (screen.includes(needle)) return screen
        await page.waitForTimeout(200)
      }
      throw new Error(`timed out after ${timeout}ms waiting for ${JSON.stringify(needle)}\n--- screen ---\n${screen}`)
    },

    async resize (width, height) {
      await page.setViewportSize({ width, height })
      // The fit addon resizes the pty on its own tick, and the app redraws when the pty tells it to.
      await page.waitForTimeout(500)
    },

    async screenshot (name) {
      await page.screenshot({ path: `${screenshotDir}/${name}.png` })
    },

    close: () => browser.close(),
  }

  // Wait for the app's own chrome before handing the terminal over, so a journey never reads a half-drawn screen and every keystroke reaches a live app.
  await term.waitForText(optionsBarMarker)
  // Focus through xterm's API, never by clicking: gocui turns mouse tracking on, main binds MouseLeft to switchFocus, and a click at the page centre lands there and takes focus off the panel the app started on.
  await page.evaluate(() => window.term.focus())
  return term
}

export function assertScreen (screen, needle, what) {
  const found = needle instanceof RegExp ? needle.test(screen) : screen.includes(needle)
  if (!found) {
    throw new Error(`${what}: expected ${needle} on screen\n--- screen ---\n${screen}`)
  }
}
