import { assertScreen } from '../harness.mjs'

// Sections are asserted at the WIDE viewport: the stacked layout runs the sections down the pane and the ones below the fold are not on screen, while two columns fit the whole overview without any scrolling.
const wide = { width: 1700, height: 900 }
// 1000px lands the pane under presentation.minTwoColWidth (110), which is what makes the layout stack.
const narrow = { width: 1000, height: 900 }

// Per resource: the number key that focuses its panel, the tab list its detail pane must offer with Overview FIRST, the overview's header word, and sections that only the Overview tab renders.
// Section names are deliberately not the tab names: "Configuration" is a section and "Config" is a tab, so a screen still showing the old tab cannot satisfy a section assertion.
// The tab lists are the pruned sets: a tab that only repeated the Overview is gone, and what survives carries content the Overview only summarises.
// Section titles carry the mockups' icons, and the ECS/EC2/Secret header kinds are the mockups' full names.
const resources = [
  { key: '2', name: 'ECS cluster', tabs: ['Overview', 'Instances'], header: '⬡ ECS Cluster', sections: ['♡ Health', '▤ Configuration', '⬡ Capacity', '◒ Metrics', '▦ Service Summary', '≡ Tasks', '◇ Tags'] },
  { key: '3', name: 'EC2 instance', tabs: ['Overview'], header: '◇ EC2 Instance', sections: ['▤ Configuration', '⇄ Network', '◒ Metrics', '♡ Status', '▣ Storage', '⌾ Security', '⌘ Console', '◇ Tags'] },
  { key: '4', name: 'S3 bucket', tabs: ['Overview', 'Config', 'Objects', 'Policy'], header: '▣ Bucket', sections: ['⌾ Access', '▣ Data management', '⌾ Security', '◇ Tags'] },
  { key: '6', name: 'ECR repository', tabs: ['Overview', 'Images', 'Scan', 'Policies'], header: '⬡ Repository', sections: ['▤ Configuration', '▣ Images', '⌾ Policies'] },
  { key: '7', name: 'secret', tabs: ['Overview', 'Value', 'Versions', 'Policy'], header: '▣ Secret', sections: ['▤ Details', '≡ Versions', '⇄ Replication', '⌾ Resource policy', '◇ Tags'] },
  { key: '8', name: 'VPC', tabs: ['Overview', 'Subnets', 'Routes', 'Gateways', 'Endpoints', 'Transit'], header: '⇄ VPC', sections: ['▤ Configuration', '▦ Subnets', '⇄ DNS', '⇄ Gateways', '◇ Endpoints', '◇ Tags'] },
]

// The detail pane's own frame carries the tab list, so this is where a journey reads which tab is open.
// It is anchored on the tab NAMES rather than on the frame's corner: the detail pane shares its terminal row with a side panel's frame, and the side panel's corner comes first.
// Tab names hold no box-drawing runes, so the padding that follows the last tab is where the match ends.
function tabBar (screen) {
  const match = screen.match(/╭─((?:Overview|Credentials)[^─╮\n]*)/)
  if (!match) throw new Error(`no detail-pane tab bar on screen\n--- screen ---\n${screen}`)
  return match[1].trim()
}

// A section title is a whole pane line, so it is read as one: matching a substring would find "Tags" in the tab bar and "Metrics" in half the screen.
// Both layouts are covered by splitting every row on the frame rune, which is also the two-column separator, and trimming each cell.
function paneCells (screen) {
  return screen.split('\n').flatMap(line => line.split('│').slice(1).map(cell => cell.trim()))
}

// The header and the section titles each OPEN a pane line: a title may carry right-aligned content after a run of spaces (the header's stat cards, Service Summary's count note), so the match is "the whole cell, or the cell's start followed by a gap".
// A bare substring check would pass on text the Overview did not draw: "VPC" is also the side panel's own frame title, and "Instance" sits inside the ECS pane's "Instances" tab.
// It WAITS rather than reading once: the tab bar is drawn from the registry the moment the panel takes focus, while the body arrives with the fetches behind it (the bucket overview alone makes eleven calls), so a single read asserts against whichever sections happened to have landed.
function holdsTitle (cells, title) {
  return cells.some(cell => cell === title || cell.startsWith(title + '  '))
}

async function assertSections (term, resource, { timeout = 20000 } = {}) {
  const want = [resource.header, ...resource.sections]
  const deadline = Date.now() + timeout
  let screen = ''
  let missing = want
  while (Date.now() < deadline) {
    screen = await term.readScreen()
    const cells = paneCells(screen)
    missing = want.filter(title => !holdsTitle(cells, title))
    if (missing.length === 0) return screen
    await term.page.waitForTimeout(250)
  }
  throw new Error(`${resource.name} Overview: no ${missing.map(t => JSON.stringify(t)).join(', ')} line in the pane after ${timeout}ms\n--- screen ---\n${screen}`)
}

export async function run ({ term, seed }) {
  await term.waitForText(seed.ecs.clusterName)

  // Nothing seeds EKS, so its panel is the empty-panel case, and it goes FIRST while the pane has never held a resource.
  // An empty panel has no resource to inspect: the pane must keep the credentials view rather than render an overview of nothing, which is the nil-row crash the formatters guard against.
  await term.sendKeys('5')
  await term.waitForText('╭─Credentials - Config')
  const eks = tabBar(await term.settle())
  if (eks.startsWith('Overview')) {
    throw new Error(`EKS has no seeded cluster, so the pane must not offer an Overview; tab bar is ${JSON.stringify(eks)}`)
  }
  // The app is still live on an empty panel: the footer no longer names the focused view, so the menu the footer points at is what answers, and it has to open and close over the dashboard.
  await term.sendKeys('?')
  await term.waitForText('╭─Menu')
  await term.sendKeys('Escape')
  await term.settle()
  assertScreen(await term.readScreen(), 'no EKS clusters', 'EKS in-panel empty state')

  await term.resize(wide.width, wide.height)

  for (const resource of resources) {
    await term.sendKeys(resource.key)
    await term.waitForText(`╭─${resource.tabs.join(' - ')}`)

    // Overview leads the tab list: the redesign prepends it, so a registry that appends it renders the same tabs in the wrong order and this is the only assertion that can tell.
    const bar = tabBar(await term.readScreen())
    if (bar !== resource.tabs.join(' - ')) {
      throw new Error(`${resource.name}: tab bar is ${JSON.stringify(bar)}, want ${JSON.stringify(resource.tabs.join(' - '))}`)
    }

    // The header is the overview's first block and it names the resource kind, so it is how the pane says which formatter drew it.
    await assertSections(term, resource)
  }

  await term.screenshot('overview-two-column')

  // Where a section has no answer it must say so rather than render a zero: moto's CloudWatch cannot serve GetMetricData, which is the failure the EC2 pane has to survive section by section.
  await term.sendKeys('3')
  await term.waitForText('╭─Overview')
  const ec2 = await assertSections(term, resources[1])
  assertScreen(ec2, /unavailable/, 'EC2 Metrics unavailable state')
  // A failing metrics fetch must not take the sections beside it down with it.
  assertScreen(ec2, 'Configuration', 'EC2 sections beside the failed one')
  assertScreen(ec2, /Type:\s+t3\.micro/, 'EC2 Configuration survives a failed metrics fetch')

  // Two columns are only two columns if a right-column section shares a line with a left-column one.
  const paired = ec2.split('\n').find(line => /Configuration/.test(line) && /Status/.test(line))
  if (!paired) {
    throw new Error(`Overview is not two-column at ${wide.width}px: no line holds both Configuration and Status\n--- screen ---\n${ec2}`)
  }

  // Narrow it and the same two sections must be stacked instead of wrapped: each on its own line, both still present, nothing soft-wrapped into the panel beside it.
  // POLLED rather than read once: a resize triggers a full re-render through gocui's unordered Update, and a single read can land between the pane's clear and its rewrite.
  await term.resize(narrow.width, narrow.height)
  await term.waitForText('╭─Overview')
  let stacked = ''
  {
    const deadline = Date.now() + 10000
    let missing = []
    while (Date.now() < deadline) {
      stacked = await term.readScreen()
      const cells = paneCells(stacked)
      missing = ['▤ Configuration', '♡ Status'].filter(section => !holdsTitle(cells, section))
      if (missing.length === 0) break
      await term.page.waitForTimeout(200)
    }
    if (missing.length > 0) {
      throw new Error(`Overview lost the ${missing.map(s => JSON.stringify(s)).join(', ')} section(s) when narrow\n--- screen ---\n${stacked}`)
    }
  }
  if (stacked.split('\n').some(line => /Configuration/.test(line) && /Status/.test(line))) {
    throw new Error(`Overview is still two-column at ${narrow.width}px\n--- screen ---\n${stacked}`)
  }
  await term.screenshot('overview-stacked')

  // Tabs cycle from the list itself and not only from the focused main pane, so . must land on the tab after Overview and , must come back to it.
  // EC2 is down to one tab, so ECS is where the cycle is observable; the tab BAR is not the assertion: gocui marks the open tab in the title's attributes and leaves the list of names alone, so only the pane's body says which tab is showing.
  await term.sendKeys('2')
  await term.waitForText('╭─Overview - Instances')
  await assertSections(term, resources[0], { timeout: 20000 })
  await term.sendKeys('.')
  const instances = paneCells(await term.settle())
  if (instances.includes('▤ Configuration')) {
    throw new Error(`. left the Overview open: the pane still holds its Configuration section`)
  }
  // The seeded cluster runs on Fargate, so the Instances tab's whole answer is its empty state.
  if (!instances.some(cell => cell.startsWith('no container instances'))) {
    throw new Error(`. did not open the Instances tab: no container-instance line in the pane\n${instances.filter(Boolean).slice(0, 12).join('\n')}`)
  }

  await term.sendKeys(',')
  const back = paneCells(await term.settle())
  if (!back.includes('▤ Configuration')) {
    throw new Error(`, did not come back to the Overview: no Configuration section in the pane`)
  }
}
