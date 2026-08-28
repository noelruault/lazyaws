import { assertScreen } from '../harness.mjs'

// gocui draws the title prefix and the title joined by the theme's frame rune, so the separator is anything but a letter.
const panelTitles = ['Profiles', 'ECS', 'EC2', 'S3', 'EKS', 'ECR', 'Secrets', 'VPC']
  .map((title, i) => new RegExp(`\\[${i + 1}\\]\\W${title}`))

export async function run ({ term, seed }) {
  await term.waitForText(seed.ecs.clusterName)
  const screen = await term.readScreen()
  await term.screenshot('smoke')

  for (const title of panelTitles) {
    assertScreen(screen, title, 'panel frame')
  }
  // A frame proves the layout, not the data path, so every panel also has to show something it could only have fetched.
  assertScreen(screen, 'ui-harness', 'profiles panel')
  assertScreen(screen, seed.ecs.clusterName, 'ECS panel')
  assertScreen(screen, seed.instanceNames.web, 'EC2 panel')
  assertScreen(screen, 'lazyaws-ui-artifacts', 'S3 panel')
  assertScreen(screen, 'lazyaws/api', 'ECR panel')
  assertScreen(screen, 'lazyaws/ui/db', 'secrets panel')
  assertScreen(screen, 'ui-core', 'VPC panel')
  // Nothing seeds EKS, so the in-panel empty state is the only correct render for it.
  assertScreen(screen, 'no EKS clusters', 'EKS panel')

  // Focus has to be where the app put it (`initiallyFocusedViewName` is the profile panel), not where the harness reached for it.
  // The main pane's own footer offers "Enter Select", so this fails the moment anything in the harness sends a click.
  // WAITED rather than read once: the footer renders through gocui's unordered Update, and a one-shot read at boot can race the first paint.
  await term.waitForText('Enter Switch')
}
