import { assertScreen } from '../harness.mjs'

// One case per left panel: its number key focuses it, the arrows walk its rows, and the highlight says which row the app thinks is selected.
// Each panel's `rows` are asserted as whole lines rather than with substring checks, because a substring cannot see a row that was truncated or that grew a field.
function panelCases (seed) {
  return [
    {
      key: '1',
      title: 'Profiles',
      // The connected profile renders its identity triple, and it is the only row, so the arrows have nowhere to go.
      rows: [`ui-harness ▸ ${seed.region} ▸ 123456789012`],
    },
    {
      key: '2',
      title: 'ECS',
      // moto reports the seeded task as pending and the service as not yet steady, so the row's counts are its own fixture, not AWS's.
      rows: [new RegExp(`${seed.ecs.clusterName.slice(0, 7)}.*services.*running / 1 pending`)],
    },
    {
      key: '3',
      title: 'EC2',
      // Sorted by name, so the instance seeded WITHOUT a Name tag leads: the row has to render the fallback rather than a blank first column.
      rows: [/^▶ \(no name\)/, new RegExp(`^▶ ${seed.instanceNames.db}\\b`), new RegExp(`^▶ ${seed.instanceNames.web}\\b`)],
    },
    {
      key: '4',
      title: 'S3',
      rows: seed.buckets.map(bucket => new RegExp(`^${bucket}\\s`)),
    },
    {
      key: '5',
      title: 'EKS',
      // Nothing seeds EKS. The in-panel empty state is the row, and gocui leaves the cursor on it, so it is also what reads as selected.
      rows: ['no EKS clusters'],
      empty: true,
    },
    {
      key: '6',
      title: 'ECR',
      // One repository per mutability setting: the badge is rendered from that field alone, so a swapped mapping shows up here and nowhere else.
      rows: [/^lazyaws\/api\s+immutable scan on$/, /^lazyaws\/worker\s+● mutable scan off$/],
    },
    {
      key: '7',
      title: 'Secrets',
      // The rotation cell is the point of seeding two secrets: 7d comes from the rotation rule, off from its absence.
      rows: [/^lazyaws\/ui\/api-key\s+rotation off/, /^lazyaws\/ui\/db\s+rotation 7d/],
    },
    {
      key: '8',
      title: 'VPC',
      // moto ships a default VPC on top of the two seeded ones, and it has no Name tag, so the third row proves both fallbacks.
      rows: [/^▶ 10\.0\.0\.0\/16\s+ui-core/, /^▶ 10\.1\.0\.0\/16\s+ui-edge/, /^▶ 172\.31\.0\.0\/16\s+\(no name\)/],
    },
  ]
}

// A focused panel is the tall one, so its rows are the lines between its own frame and the next panel's.
function panelRows (screen, title) {
  const lines = screen.split('\n')
  const start = lines.findIndex(line => new RegExp(`\\[\\d\\]\\W${title}\\W`).test(line))
  if (start < 0) throw new Error(`panel ${title} has no frame on screen\n--- screen ---\n${screen}`)
  const rows = []
  for (let i = start + 1; i < lines.length; i++) {
    // The panel's bottom border ends it; a side panel is drawn from column 0, so the border rune is the first cell.
    if (lines[i].startsWith('╰')) break
    const row = lines[i].replace(/^│/, '').split('│')[0].trim()
    if (row !== '') rows.push(row)
  }
  return rows
}

export async function run ({ term, seed }) {
  await term.waitForText(seed.ecs.clusterName)

  for (const panel of panelCases(seed)) {
    await term.sendKeys(panel.key)
    // The panel grows when it takes focus, so the rows are only read once the highlight proves the redraw has landed on THIS panel.
    await term.waitForSelectedRow(panel.rows[0])
    const screen = await term.readScreen()
    const rows = panelRows(screen, panel.title)

    if (rows.length !== panel.rows.length) {
      throw new Error(`panel ${panel.title}: ${rows.length} rows, want ${panel.rows.length}\n${rows.join('\n')}`)
    }
    panel.rows.forEach((want, i) => {
      const found = want instanceof RegExp ? want.test(rows[i]) : rows[i] === want
      if (!found) throw new Error(`panel ${panel.title} row ${i}: ${JSON.stringify(rows[i])} does not match ${want}`)
    })

    // Focus is only proven by the highlight: the frame is drawn whether or not the panel holds it.
    const selected = await term.selectedRow()
    if (selected !== rows[0]) {
      throw new Error(`panel ${panel.title}: highlighted ${JSON.stringify(selected)}, want the first row ${JSON.stringify(rows[0])}`)
    }

    // Arrowing on a panel with one row (or with only an empty state) must not move a selection that does not exist.
    await term.sendKeys('ArrowDown')
    if (panel.empty || rows.length === 1) {
      // Waiting for the row it is already on would pass at once, so this one has to sit out the redraw and then read.
      await term.page.waitForTimeout(500)
      const after = await term.selectedRow()
      if (after !== rows[0]) {
        throw new Error(`panel ${panel.title}: ArrowDown moved the highlight to ${JSON.stringify(after)} with only one row`)
      }
    } else {
      await term.waitForSelectedRow(rows[1])
    }

    // Back to the top, so the next panel's case starts from the same place this one did.
    await term.sendKeys('ArrowUp')
  }

  await term.screenshot('panels')

  // The empty state is a muted message and not a row, so nothing about it may read as data.
  assertScreen(await term.readScreen(), 'no EKS clusters', 'EKS in-panel empty state')
}
