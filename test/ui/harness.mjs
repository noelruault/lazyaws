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
      const line = buf.getLine(buf.viewportY + i)
      if (!line) { lines.push(''); continue }
      // Read exactly cols cells rather than translating the whole line: xterm REFLOWS on shrink, so after a resize the buffer line still carries the wide render's cells and translateToString hands back a row hundreds of columns past the terminal's own edge.
      // Reading by cell index is also what makes "wrap off" assertable — output the app pushed past the edge lands on the next row here, exactly as a user would see it, instead of being hidden inside one long logical line.
      let text = ''
      for (let x = 0; x < window.term.cols; x++) {
        const cell = line.getCell(x)
        if (!cell) { text += ' '; continue }
        // A wide rune occupies two cells and reports the second as width 0 with no chars; skipping it keeps every row exactly as many columns as the terminal has.
        if (cell.getWidth() === 0) continue
        const chars = cell.getChars()
        text += chars === '' ? ' ' : chars
      }
      lines.push(text.replace(/\s+$/, ''))
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

    // The selected row is drawn with SelBgColor and nothing else — this gocui's highlight branch never reads SelFgColor — so the highlight lives in the cell attributes and translateToString cannot see it at all.
    // Exactly one row on the dashboard carries a non-default background, which is why this can return the selection rather than a list: a second painted row would mean something else started colouring backgrounds and the assertion should fail rather than pick.
    async selectedRow () {
      const painted = await page.evaluate(() => {
        const buf = window.term.buffer.active
        const rows = []
        for (let i = 0; i < window.term.rows; i++) {
          const line = buf.getLine(buf.viewportY + i)
          if (!line) continue
          // Only the painted cells, never the whole terminal row: the highlight spans the panel's inner width, and the rest of the row is the detail pane beside it.
          let text = ''
          for (let x = 0; x < window.term.cols; x++) {
            const cell = line.getCell(x)
            if (cell && !cell.isBgDefault()) text += cell.getChars()
          }
          // The frame's corner cells can inherit a background, so a row counts as highlighted only once it holds a run of them.
          if (text.trim().length > 3) rows.push(text.trim())
        }
        return rows
      })
      if (painted.length !== 1) {
        throw new Error(`expected exactly one highlighted row, found ${painted.length}: ${JSON.stringify(painted)}`)
      }
      return painted[0]
    },

    // A keypress is answered on the app's next redraw and not on the key event, so an assertion about the selection has to wait for the highlight to arrive rather than read it back immediately (measured: a focus key plus an arrow settles ~200ms later).
    async waitForSelectedRow (want, { timeout = 5000 } = {}) {
      const deadline = Date.now() + timeout
      let last = ''
      while (Date.now() < deadline) {
        try {
          last = await term.selectedRow()
          if (want instanceof RegExp ? want.test(last) : last === want) return last
        } catch (err) {
          last = err.message
        }
        await page.waitForTimeout(100)
      }
      throw new Error(`timed out after ${timeout}ms waiting for the highlight to be ${want}; last saw ${JSON.stringify(last)}`)
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
      const before = await page.evaluate(() => window.term.cols)
      await page.setViewportSize({ width, height })
      // The fit addon resizes the pty on its own tick and the app redraws when the pty tells it to, so a journey that reads straight after a resize reads the OLD width and its old layout.
      // A viewport change that lands on the same column count leaves cols alone and times out here; the caller's own layout assertion is what answers for that, since a resize this ignores would fail it.
      await page.waitForFunction(cols => window.term.cols !== cols, before, { timeout: 5000 }).catch(() => {})
      await term.settle()
    },

    // Waits for the app to stop drawing, for the assertions that have no single string to wait on (a layout that stacked, a pane that emptied).
    async settle ({ timeout = 5000, quiet = 250 } = {}) {
      const deadline = Date.now() + timeout
      let previous = null
      while (Date.now() < deadline) {
        const screen = await readScreen()
        if (screen === previous) return screen
        previous = screen
        await page.waitForTimeout(quiet)
      }
      throw new Error(`the screen was still changing after ${timeout}ms`)
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
