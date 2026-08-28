import { execFileSync } from 'node:child_process'
import { assertScreen } from '../harness.mjs'

// The footer is the claim under test: it is built per focused view from the keymap, so every keycap it prints has to be bound HERE and do what its label says.
// It is parsed rather than hardcoded, so a rebound key moves the assertion with it instead of failing it.
function footerKeys (footer) {
  // Entries are separated by the renderer's three-space gap (optionsSeparator in ui/view_helpers.go); the wider run before a trailing app status splits away with it.
  return new Map(footer.split(/\s{3,}/).map(entry => {
    const [key, ...label] = entry.trim().split(' ')
    return [key, label.join(' ')]
  }))
}

// The options line shares the bottom row with the app status and the version, and gocui cuts whatever does not fit.
// So a footer read while a load is in flight can arrive with a trailing status ("…   q Quit        loading s3 ⠋") and short of its tail, which silently removes keys from the very list this journey checks. Waiting for a line that still has BOTH ends and nothing after them is what makes the snapshot trustworthy.
async function cleanFooter (term, { timeout = 15000 } = {}) {
  const deadline = Date.now() + timeout
  let footer = ''
  while (Date.now() < deadline) {
    footer = await term.footer()
    if (footer.startsWith('←→↑↓') && footer.endsWith('Quit')) return footer
    await term.page.waitForTimeout(150)
  }
  throw new Error(`the options line never came back clean; last read ${JSON.stringify(footer)}`)
}

function assertAdvertises (footer, key, label) {
  const advertised = footerKeys(footer).get(key)
  if (advertised !== label) {
    throw new Error(`footer advertises ${JSON.stringify(key)} as ${JSON.stringify(advertised)}, want ${JSON.stringify(label)}\n${footer}`)
  }
}

// The lines a popup itself holds, read between ITS borders.
// Scoping matters more than it looks: the detail pane prints the selected instance's full id in its own header, so "the id is somewhere on screen" is true whatever the popup was handed, and a mutant that publishes the wrong copy value survives that assertion.
function popupBody (screen, title) {
  const lines = screen.split('\n')
  const top = lines.findIndex(line => line.includes(`╭─${title}`))
  if (top < 0) throw new Error(`no popup titled ${JSON.stringify(title)} on screen\n--- screen ---\n${screen}`)
  const left = lines[top].indexOf(`╭─${title}`)
  const body = []
  for (let i = top + 1; i < lines.length; i++) {
    const cell = lines[i].slice(left)
    if (cell.startsWith('╰')) break
    body.push(cell.replace(/^│/, '').split('│')[0].trim())
  }
  return body
}

async function assertClosed (term, popup, key) {
  await term.sendKeys('Escape')
  await term.settle()
  if ((await term.readScreen()).includes(popup)) {
    throw new Error(`Escape did not close what ${key} opened (${popup})`)
  }
}

export async function run ({ term, seed, endpoint }) {
  if (!endpoint) throw new Error('this journey changes what AWS answers, so it needs AWS_ENDPOINT_URL to point at the fake one')
  // The aws CLI does the signing; run.sh already requires it, and nothing here is asked of anything but the local fake endpoint.
  const env = { ...process.env, AWS_ENDPOINT_URL: endpoint, AWS_REGION: seed.region, AWS_PAGER: '' }
  const aws = args => execFileSync('aws', args, { env, encoding: 'utf8' })

  await term.waitForText(seed.ecs.clusterName)
  await term.sendKeys('3')
  // Sorted by name, so the instance with no Name tag leads and is what every assertion below is about.
  const first = await term.waitForSelectedRow(/^▶ \(no name\)/)
  const footer = await cleanFooter(term)

  // --- y copy popup ---------------------------------------------------------------------------
  assertAdvertises(footer, 'y', 'Copy ARN')
  if (!first.includes('…')) {
    throw new Error(`the copy popup only proves something against a truncated row, and this row is not truncated: ${JSON.stringify(first)}`)
  }
  await term.sendKeys('y')
  await term.waitForText('id / ARN (select to copy)')
  // The point of the popup is the UNTRUNCATED id, and it is read out of the popup's own lines: the row shows an ellipsis at this panel width, so the whole id inside the box proves the popup is not echoing the row.
  const copied = popupBody(await term.readScreen(), 'id / ARN (select to copy)')
  if (!copied.includes(seed.instances.unnamed)) {
    throw new Error(`the copy popup holds ${JSON.stringify(copied)}, want the full id ${seed.instances.unnamed}`)
  }
  await assertClosed(term, 'id / ARN (select to copy)', 'y')

  // --- a actions menu -------------------------------------------------------------------------
  // Opened and closed, never chosen from: every entry in here mutates infrastructure, and this loop reads only.
  assertAdvertises(footer, 'a', 'Actions')
  await term.sendKeys('a')
  await term.waitForText('EC2 Instances actions')
  await assertClosed(term, 'EC2 Instances actions', 'a')

  // --- x options menu -------------------------------------------------------------------------
  await term.sendKeys('x')
  await term.waitForText('╭─Menu')
  // x lists every binding, which makes it the cross-check on the footer: a keycap the footer advertises for this view that the menu does not bind is a label with nothing behind it.
  const menu = await term.readScreen()
  for (const [key, label] of footerKeys(footer)) {
    // Only the single-character keycaps: the arrow and enter keys are literals no config can move, and the menu lists those under their own names rather than as a keycap.
    if (!/^[a-z/]$/.test(key)) continue
    // A menu row is the popup's own border followed by the keycap and its description, which is what keeps this from matching a list row behind the popup.
    if (!new RegExp(`│${key}\\s{2,}\\S`).test(menu)) {
      throw new Error(`the footer advertises "${key} ${label}" but the x menu does not bind ${JSON.stringify(key)} in this view\n--- menu ---\n${menu}`)
    }
  }
  await assertClosed(term, '╭─Menu', 'x')

  // --- / filter -------------------------------------------------------------------------------
  assertAdvertises(footer, '/', 'Filter')
  await term.sendKeys('/')
  await term.type('web')
  // The filter takes over the bottom row from the options line, which is how the app says it is capturing keys.
  await term.waitForText('filter: web')
  await term.sendKeys('Enter')
  // Filtering narrows the list to its one match AND moves the selection onto it, so the detail pane must describe that instance rather than the one selected before.
  await term.waitForSelectedRow(new RegExp(`^▶ ${seed.instanceNames.web}\\b`))
  // The pane fetches before it paints, so the header is WAITED for rather than read: a read racing the fetch sees the previous pane.
  await term.waitForText(`${seed.instanceNames.web}  ● running  ${seed.instances.web}`)
  const filtered = await term.readScreen()
  if (filtered.includes(seed.instanceNames.db)) {
    throw new Error(`the filter left ${seed.instanceNames.db} in the list`)
  }

  // Escape clears it, and the rows it hid come back. The selection returns to the head of the list rather than staying on the match, which is the app's behaviour here, not an accident of this journey.
  await term.sendKeys('Escape')
  await term.waitForSelectedRow(/^▶ \(no name\)/)
  const restored = await term.readScreen()
  assertScreen(restored, seed.instanceNames.db, 'Escape restores the rows the filter hid')
  if (restored.includes('filter: web')) {
    throw new Error(`Escape left the filter on the status line\n${await term.footer()}`)
  }

  // --- r refresh ------------------------------------------------------------------------------
  assertAdvertises(footer, 'r', 'Refresh')
  // r cannot be told from the panel tier's own tick by anything on screen: it TRIGGERS the same throttle the 2s tier triggers (ui/refresh.go, reloadFocusedPanel), deliberately, so a reload arriving after r is not evidence that r caused it.
  // What is assertable is that it is bound here and costs nothing — the panel keeps its rows and its selection.
  const before = await term.selectedRow()
  await term.sendKeys('r')
  await term.settle()
  if (await term.selectedRow() !== before) {
    throw new Error(`r moved the selection from ${JSON.stringify(before)}`)
  }
  assertScreen(await term.readScreen(), seed.instanceNames.web, 'r leaves the panel populated')

  // --- R refresh everything -------------------------------------------------------------------
  // R is separable, and this is the one refresh assertion that can fail: it reloads EVERY list, while the panel tier reloads exactly one per tick — the focused one — so a bucket created now cannot reach the unfocused S3 panel any other way.
  // The name sorts ahead of the seeded buckets so its row lands inside the collapsed panel's visible rows instead of below them.
  // Short as well as first: the collapsed panel's name column truncates around 29 cells, so a long name arrives on screen with an ellipsis through it and matches nothing.
  const bucket = `aaa-keys-${Date.now().toString().slice(-8)}`
  aws(['s3api', 'create-bucket', '--bucket', bucket, '--create-bucket-configuration', `LocationConstraint=${seed.region}`])
  try {
    // A tick's worth of grace: if the unfocused panel were on the tier, it would have picked this up here and R would be proving nothing.
    await term.page.waitForTimeout(2500)
    if ((await term.readScreen()).includes(bucket)) {
      throw new Error(`the unfocused S3 panel refreshed itself, so R cannot be what brings ${bucket} in`)
    }
    await term.sendKeys('Shift+R')
    await term.waitForText(bucket, { timeout: 10000 })
  } finally {
    // Left behind, this bucket would be an extra row in every later journey's S3 panel.
    aws(['s3api', 'delete-bucket', '--bucket', bucket])
  }

  // --- q quit ---------------------------------------------------------------------------------
  // Last, because it takes the app away: ttyd's session ends with the process, and the terminal is dead after this.
  assertAdvertises(footer, 'q', 'Quit')
  await term.screenshot('keys')
  await term.sendKeys('q')
  const deadline = Date.now() + 10000
  let screen = ''
  while (Date.now() < deadline) {
    screen = await term.readScreen()
    if (screen.trim() === '') return
    await term.page.waitForTimeout(200)
  }
  throw new Error(`q did not quit: the app is still drawing\n--- screen ---\n${screen}`)
}
