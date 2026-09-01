import { execFileSync } from 'node:child_process'
import { assertScreen } from '../harness.mjs'

// The menu behind "?" is the claim under test: the footer is one hint now, so this is the only place a panel's keys are advertised, and every keycap it lists has to be bound HERE and do what its description says.
// It is read rather than hardcoded, so a rebound key moves the assertion with it instead of failing it.
async function menuKeys (term) {
  await term.sendKeys('?')
  await term.waitForText('╭─Menu')
  const rows = popupBody(await term.readScreen(), 'Menu')
  await assertClosed(term, '╭─Menu', '?')

  // A row is its keycaps, a run of padding, then the description. Keys that do the same thing share a row ("k / ▲"), so each one is entered separately and the menu reads as the map of the keyboard it is.
  return new Map(rows.flatMap(row => {
    const [, keys, description] = row.match(/^(\S+(?: \/ \S+)*)\s{2,}(\S.*)$/) ?? []
    return keys ? keys.split(' / ').map(key => [key, description.trim()]) : []
  }))
}

// The popup cuts a description at its own width, so the row is asserted as a prefix of the keymap's text rather than as the whole of it: what matters is that this key is listed and says it does this.
function assertBinds (menu, key, description) {
  const bound = menu.get(key)
  if (!bound || !description.startsWith(bound)) {
    throw new Error(`the menu lists ${JSON.stringify(key)} as ${JSON.stringify(bound)}, want ${JSON.stringify(description)}\n${[...menu].map(([k, d]) => `${k}  ${d}`).join('\n')}`)
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
  const menu = await menuKeys(term)

  // --- rebound navigation ---------------------------------------------------------------------
  await term.sendKeys('n')
  await term.waitForSelectedRow(new RegExp(`^▶ ${seed.instanceNames.db}\\b`))
  await term.sendKeys('ArrowUp')
  await term.waitForSelectedRow(/^▶ \(no name\)/)

  // --- y copy popup ---------------------------------------------------------------------------
  assertBinds(menu, 'y', "Show the selected item's full id / ARN, untruncated, to copy by hand")
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
  assertBinds(menu, 'a', 'Open the actions menu for the focused item')
  await term.sendKeys('a')
  await term.waitForText('EC2 Instances actions')
  await assertClosed(term, 'EC2 Instances actions', 'a')

  // --- x opens the same menu as ? ---------------------------------------------------------------
  // Both keys open it and the footer advertises only one, so the other is covered here or nowhere.
  await term.sendKeys('x')
  await term.waitForText('╭─Menu')
  await assertClosed(term, '╭─Menu', 'x')

  // Every key pressed below has to be one this panel's menu offers: with the footer down to a single hint, a working key the menu omits is a key nothing on screen points at.
  for (const key of ['/', 'r', 'q']) {
    if (!menu.has(key)) {
      throw new Error(`the EC2 menu does not bind ${JSON.stringify(key)}, so nothing on screen tells the user it works\n${[...menu.keys()].join(' ')}`)
    }
  }

  // --- / filter -------------------------------------------------------------------------------
  assertBinds(menu, '/', 'Filter the focused list')
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
  assertBinds(menu, 'r', 'Refresh the focused panel')
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
  assertBinds(menu, 'q', 'Quit')
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
