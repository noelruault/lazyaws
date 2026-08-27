import { assertScreen } from '../harness.mjs'

// Sections are asserted at the WIDE viewport: the stacked layout runs the sections down the pane and the ones below the fold are not on screen, while two columns fit the whole overview without any scrolling.
const wide = { width: 1700, height: 900 }
// 1000px lands the pane under presentation.minTwoColWidth (110), which is what makes the layout stack.
const narrow = { width: 1000, height: 900 }

// Per resource: the number key that focuses its panel, the tab list its detail pane must offer with Overview FIRST, the overview's header word, and sections that only the Overview tab renders.
// Section names are deliberately not the tab names: "Configuration" is a section and "Config" is a tab, so a screen still showing the old tab cannot satisfy a section assertion.
const resources = [
  { key: '2', name: 'ECS cluster', tabs: ['Overview', 'Config', 'Instances', 'Tags'], header: 'Cluster', sections: ['Configuration', 'Capacity', 'Metrics', 'Services', 'Tasks'] },
  { key: '3', name: 'EC2 instance', tabs: ['Overview', 'Config', 'Status', 'Metrics', 'Storage', 'Security', 'Tags'], header: 'Instance', sections: ['Configuration', 'Network', 'Metrics', 'Status', 'Storage', 'Security', 'Tags'] },
  { key: '4', name: 'S3 bucket', tabs: ['Overview', 'Config', 'Objects', 'Policy'], header: 'Bucket', sections: ['Access', 'Data management', 'Security', 'Tags'] },
  { key: '6', name: 'ECR repository', tabs: ['Overview', 'Config', 'Images', 'Scan'], header: 'Repository', sections: ['Configuration', 'Images', 'Policies'] },
  { key: '7', name: 'secret', tabs: ['Overview', 'Config', 'Value'], header: 'Secret', sections: ['Details', 'Versions', 'Replication', 'Resource policy', 'Tags'] },
  { key: '8', name: 'VPC', tabs: ['Overview', 'Config', 'Subnets', 'Routes', 'Gateways', 'Endpoints', 'Transit'], header: 'VPC', sections: ['Configuration', 'Subnets', 'DNS', 'Gateways', 'Endpoints', 'Tags'] },
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

async function assertSections (term, resource) {
  const screen = await term.readScreen()
  const cells = paneCells(screen)
  for (const section of resource.sections) {
    if (!cells.includes(section)) {
      throw new Error(`${resource.name} Overview: no "${section}" section title\n--- screen ---\n${screen}`)
    }
  }
  return screen
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
  // The app is still live and the focus really moved: the empty panel's own options line is what a list offers, not what main offers.
  assertScreen(await term.footer(), 'enter inspect', 'focus on the empty EKS panel')
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
    const screen = await assertSections(term, resource)
    assertScreen(screen, resource.header, `${resource.name} Overview header`)
  }

  await term.screenshot('overview-two-column')

  // Where a section has no answer it must say so rather than render a zero: moto's CloudWatch cannot serve GetMetricData, which is the failure the EC2 pane has to survive section by section.
  await term.sendKeys('3')
  await term.waitForText('╭─Overview - Config - Status')
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
  await term.resize(narrow.width, narrow.height)
  await term.waitForText('╭─Overview - Config - Status')
  const stacked = await term.readScreen()
  const cells = paneCells(stacked)
  for (const section of ['Configuration', 'Status']) {
    if (!cells.includes(section)) {
      throw new Error(`Overview lost the "${section}" section when narrow\n--- screen ---\n${stacked}`)
    }
  }
  if (stacked.split('\n').some(line => /Configuration/.test(line) && /Status/.test(line))) {
    throw new Error(`Overview is still two-column at ${narrow.width}px\n--- screen ---\n${stacked}`)
  }
  await term.screenshot('overview-stacked')

  // Tabs cycle from the list itself and not only from the focused main pane, so ] must land on the tab after Overview and [ must come back to it.
  // The tab BAR is not the assertion: gocui marks the open tab in the title's attributes and leaves the list of names alone, so only the pane's body says which tab is showing.
  await term.sendKeys(']')
  const config = paneCells(await term.settle())
  if (config.includes('Configuration')) {
    throw new Error(`] left the Overview open: the pane still holds its Configuration section`)
  }
  if (!config.some(cell => cell.startsWith(`ID: ${seed.instances.unnamed}`))) {
    throw new Error(`] did not open the Config tab: no "ID:" line in the pane\n${config.filter(Boolean).slice(0, 12).join('\n')}`)
  }

  await term.sendKeys('[')
  const back = paneCells(await term.settle())
  if (!back.includes('Configuration')) {
    throw new Error(`[ did not come back to the Overview: no Configuration section in the pane`)
  }
}
