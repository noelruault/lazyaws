package ui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/jesseduffield/gocui"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/config"
	"github.com/noelruault/lazyaws/ui/panels"
	"github.com/noelruault/lazyaws/ui/presentation"
	"github.com/noelruault/lazyaws/ui/tasks"
	"github.com/noelruault/lazyaws/ui/utils"
)

type ecsDrillLevel int

const (
	ecsLevelClusters ecsDrillLevel = iota
	ecsLevelServices
	ecsLevelTasks
)

// ecsDrillState lives on Gui because the generic side panel cannot own ECS-specific navigation.
type ecsDrillState struct {
	level   ecsDrillLevel
	cluster string
	service string
}

type ecsRowKind int

const (
	ecsRowKindCluster ecsRowKind = iota
	ecsRowKindService
	ecsRowKindTask
)

// ecsRow requires exactly one payload matching Kind.
type ecsRow struct {
	Kind    ecsRowKind
	Cluster *aws.ECSCluster
	Service *aws.ECSService
	Task    *aws.ECSTask
}

func (r *ecsRow) arn() string {
	switch r.Kind {
	case ecsRowKindService:
		return r.Service.Arn
	case ecsRowKindTask:
		return r.Task.Arn
	default:
		return r.Cluster.Arn
	}
}

func (r *ecsRow) name() string {
	switch r.Kind {
	case ecsRowKindService:
		return r.Service.Name
	case ecsRowKindTask:
		return r.Task.ID
	default:
		return r.Cluster.Name
	}
}

func (r *ecsRow) status() string {
	switch r.Kind {
	case ecsRowKindService:
		return r.Service.Status
	case ecsRowKindTask:
		return r.Task.Status
	default:
		return r.Cluster.Status
	}
}

func (gui *Gui) getECSPanel() *panels.SideListPanel[*ecsRow] {
	return &panels.SideListPanel[*ecsRow]{
		ContextState: &panels.ContextState[*ecsRow]{
			GetMainTabs: func() []panels.MainTab[*ecsRow] {
				// Every row at the current drill level shares one Kind, so the drill level (not the individual item, unavailable here) picks the tab set.
				switch gui.ecsDrill.level {
				case ecsLevelServices:
					return []panels.MainTab[*ecsRow]{
						overviewTab(gui, func(context.Context, *ecsRow, int) string { return overviewUnavailable("service") }),
						{Key: "config", Title: "Config", Render: gui.renderECSServiceConfig},
						{Key: "deployments", Title: "Deployments", Render: gui.renderECSServiceDeployments},
						{Key: "events", Title: "Events", Render: gui.renderECSServiceEvents},
						{Key: "scaling", Title: "Scaling", Render: gui.renderECSServiceScaling},
						{Key: "taskdef", Title: "Task Def", Render: func(row *ecsRow) tasks.TaskFunc {
							return gui.renderECSTaskDefDiff(row.Service.TaskDefinition)
						}},
					}
				case ecsLevelTasks:
					return []panels.MainTab[*ecsRow]{
						{Key: "config", Title: "Config", Render: gui.renderECSTaskConfig},
						{Key: "logs", Title: "Logs", Render: gui.renderECSTaskLogs},
						{Key: "taskdef", Title: "Task Def", Render: func(row *ecsRow) tasks.TaskFunc {
							return gui.renderECSTaskDefDiff(row.Task.Config.TaskDefinition)
						}},
					}
				default:
					return []panels.MainTab[*ecsRow]{
						overviewTab(gui, func(context.Context, *ecsRow, int) string { return overviewUnavailable("cluster") }),
						{Key: "config", Title: "Config", Render: gui.renderECSClusterConfig},
						{Key: "instances", Title: "Instances", Render: gui.renderECSClusterInstances},
						{Key: "tags", Title: "Tags", Render: gui.renderECSClusterTags},
					}
				}
			},
			GetItemContextCacheKey: func(row *ecsRow) string {
				return fmt.Sprintf("ecs-%d-%d-%s", gui.ecsDrill.level, row.Kind, row.arn())
			},
		},

		ListPanel: panels.ListPanel[*ecsRow]{
			List: panels.NewFilteredList[*ecsRow](),
			View: gui.Views.ECS,
		},
		NoItemsMessage: "no ECS resources",
		Gui:            gui.intoInterface(),

		Sort: func(a, b *ecsRow) bool { return a.name() < b.name() },
		GetTableCellsFit: func(row *ecsRow) []utils.Cell {
			switch row.Kind {
			case ecsRowKindService:
				return presentation.GetECSServiceDisplayCells(row.Service)
			case ecsRowKindTask:
				return presentation.GetECSTaskDisplayCells(row.Task)
			default:
				return presentation.GetECSClusterDisplayCells(row.Cluster)
			}
		},
		// The three drill levels are three different tables, so the weights come off the row being rendered rather than off the drill state, which a queued rerender can find already changed.
		Weights: func(row *ecsRow) []int {
			switch row.Kind {
			case ecsRowKindService:
				return presentation.ECSServiceWeights()
			case ecsRowKindTask:
				return presentation.ECSTaskWeights()
			default:
				return presentation.ECSClusterWeights()
			}
		},
	}
}

func (gui *Gui) renderECSClusterConfig(row *ecsRow) tasks.TaskFunc {
	c := row.Cluster
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		data, err := gui.Client.LoadECSClusterData(fetchCtx, c.Name)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading cluster data: " + err.Error())
			return
		}
		// Best-effort: Container Insights may be disabled on the cluster, which isn't an error, just zero utilization.
		insights, _ := gui.Client.GetECSContainerInsights(fetchCtx, c.Name, "")
		if gen != gui.Gen {
			return
		}
		gui.RenderStringMain(formatECSClusterConfig(c, data, insights))
	}})
}

func formatECSClusterConfig(c *aws.ECSCluster, data *aws.ECSClusterData, insights *aws.ECSContainerInsights) string {
	out := utils.FormatMap(0, map[string]string{
		"Name":                  c.Name,
		"Status":                c.Status,
		"Running tasks":         strconv.Itoa(int(c.RunningTasksCount)),
		"Pending tasks":         strconv.Itoa(int(c.PendingTasksCount)),
		"Active services":       strconv.Itoa(int(c.ActiveServicesCount)),
		"Registered containers": strconv.Itoa(int(c.RegisteredContainerCount)),
		"CPU utilization":       formatUtilizationPercent(insights, func(i *aws.ECSContainerInsights) float64 { return i.CPUPercent }),
		"Memory utilization":    formatUtilizationPercent(insights, func(i *aws.ECSContainerInsights) float64 { return i.MemPercent }),
		"Console":               c.ConsoleURL,
	})

	if len(c.DefaultCapacityProviderStrat) > 0 {
		out += "\nCapacity Providers:\n"
		for _, s := range c.DefaultCapacityProviderStrat {
			out += fmt.Sprintf("  %s", s.CapacityProvider)
			if s.Base > 0 || s.Weight > 0 {
				out += fmt.Sprintf(" (base: %d, weight: %d)", s.Base, s.Weight)
			}
			out += "\n"
		}
	} else if len(c.CapacityProviders) > 0 {
		out += fmt.Sprintf("\nCapacity Providers: %s\n", strings.Join(c.CapacityProviders, ", "))
	}

	sort.Slice(data.Services, func(i, j int) bool { return data.Services[i].Name < data.Services[j].Name })

	out += "\nServices:\n"
	if len(data.Services) == 0 {
		return out + "none\n"
	}
	rows := make([][]string, len(data.Services))
	for i, s := range data.Services {
		rows[i] = presentation.GetECSServiceDisplayStrings(&s)
	}
	table, err := utils.RenderTable(rows)
	if err != nil {
		return out + err.Error()
	}
	return out + table + "\n"
}

// formatUtilizationPercent renders zero as unavailable because disabled Insights has no reservation denominator.
func formatUtilizationPercent(insights *aws.ECSContainerInsights, get func(*aws.ECSContainerInsights) float64) string {
	if insights == nil {
		return "n/a"
	}
	if pct := get(insights); pct != 0 {
		return fmt.Sprintf("%.1f%%", pct)
	}
	return "n/a"
}

// renderECSClusterTags refetches because cluster details have no shared cache.
func (gui *Gui) renderECSClusterTags(row *ecsRow) tasks.TaskFunc {
	c := row.Cluster
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		data, err := gui.Client.LoadECSClusterData(fetchCtx, c.Name)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading tags: " + err.Error())
			return
		}
		gui.RenderStringMain(utils.FormatMap(0, data.Tags))
	}})
}

func (gui *Gui) renderECSClusterInstances(row *ecsRow) tasks.TaskFunc {
	c := row.Cluster
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		instances, err := gui.Client.ListContainerInstances(fetchCtx, c.Name)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading container instances: " + err.Error())
			return
		}
		gui.RenderStringMain(formatECSContainerInstances(instances))
	}})
}

func formatECSContainerInstances(instances []aws.ECSContainerInstance) string {
	if len(instances) == 0 {
		return "no container instances (Fargate-only cluster)\n"
	}
	sort.Slice(instances, func(i, j int) bool { return instances[i].Ec2InstanceID < instances[j].Ec2InstanceID })

	rows := make([][]string, len(instances))
	for i, ci := range instances {
		agent := "connected"
		if !ci.AgentConnected {
			agent = "disconnected"
		}
		rows[i] = []string{
			ci.Ec2InstanceID,
			ci.Status,
			agent,
			ci.AgentVersion,
			fmt.Sprintf("%d running / %d pending", ci.RunningTasksCount, ci.PendingTasksCount),
		}
	}
	table, err := utils.RenderTable(rows)
	if err != nil {
		return err.Error()
	}
	return table
}

// renderECSServiceConfig keeps target health distinct because it can disagree with ECS task state.
func (gui *Gui) renderECSServiceConfig(row *ecsRow) tasks.TaskFunc {
	s := row.Service
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		insights, _ := gui.Client.GetECSContainerInsights(fetchCtx, s.Cluster, s.Name)

		health := make(map[string][]aws.ECSTargetHealth, len(s.LoadBalancers))
		for _, lb := range s.LoadBalancers {
			if lb.TargetGroupArn == "" {
				continue
			}
			if h, err := gui.Client.DescribeTargetHealth(fetchCtx, lb.TargetGroupArn); err == nil {
				health[lb.TargetGroupArn] = h
			}
		}

		if gen != gui.Gen {
			return
		}
		gui.RenderStringMain(formatECSServiceConfig(s, insights, health))
	}})
}

func formatECSServiceConfig(s *aws.ECSService, insights *aws.ECSContainerInsights, health map[string][]aws.ECSTargetHealth) string {
	out := utils.FormatMap(0, map[string]string{
		"Name":            s.Name,
		"Status":          s.Status,
		"Task definition": s.TaskDefinition,
		"Launch type":     s.LaunchType,
		"Desired":         strconv.Itoa(int(s.DesiredCount)),
		"Running":         strconv.Itoa(int(s.RunningCount)),
		"Pending":         strconv.Itoa(int(s.PendingCount)),
		"CPU utilization": formatUtilizationPercent(insights, func(i *aws.ECSContainerInsights) float64 { return i.CPUPercent }),
		"Mem utilization": formatUtilizationPercent(insights, func(i *aws.ECSContainerInsights) float64 { return i.MemPercent }),
		"Console":         s.ConsoleURL,
	})

	out += "\nLoad balancers:\n"
	if len(s.LoadBalancers) == 0 {
		return out + "none\n"
	}
	for _, lb := range s.LoadBalancers {
		out += fmt.Sprintf("  %s %s (%s -> %s)\n", lb.Type, lb.Name, lb.ContainerMapping, lb.TargetGroup)
		for _, th := range health[lb.TargetGroupArn] {
			out += fmt.Sprintf("    %s %s:%d%s\n", presentation.StatusCell(th.State, presentation.StatusStyleIcon), th.TargetID, th.Port, formatHealthReason(th.Reason))
		}
	}
	return out
}

func formatHealthReason(reason string) string {
	if reason == "" {
		return ""
	}
	return " (" + reason + ")"
}

const ecsCodeDeployControllerType = "CODE_DEPLOY"

func (gui *Gui) renderECSServiceDeployments(row *ecsRow) tasks.TaskFunc {
	s := row.Service
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen

		var cd *aws.ECSCodeDeployStatus
		if s.DeploymentController == ecsCodeDeployControllerType {
			fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			cd, _ = gui.Client.GetECSCodeDeployStatus(fetchCtx, s.Cluster, s.Name)
			cancel()
		}

		if gen != gui.Gen {
			return
		}
		gui.RenderStringMain(formatECSServiceDeployments(s, cd))
	}})
}

func formatECSServiceDeployments(s *aws.ECSService, cd *aws.ECSCodeDeployStatus) string {
	out := ""
	if s.DeploymentController != "" {
		out += fmt.Sprintf("Controller: %s\n", s.DeploymentController)
	}
	if s.CircuitBreakerEnabled {
		out += fmt.Sprintf("Circuit breaker: enabled (rollback %s)\n", enabledDisabled(s.CircuitBreakerRollback))
	} else {
		out += "Circuit breaker: disabled\n"
	}
	if cd != nil {
		out += fmt.Sprintf("CodeDeploy: %s / %s\n", cd.ApplicationName, cd.DeploymentGroupName)
		if cd.LastAttemptedStatus != "" {
			out += fmt.Sprintf("  Last attempted: %s%s\n", cd.LastAttemptedStatus, formatDeployedAt(cd.LastAttemptedAt))
		}
		if cd.LastSuccessfulStatus != "" {
			out += fmt.Sprintf("  Last successful: %s%s\n", cd.LastSuccessfulStatus, formatDeployedAt(cd.LastSuccessfulAt))
		}
	} else if s.DeploymentController == ecsCodeDeployControllerType {
		out += "CodeDeploy: no matching deployment group found\n"
	}
	out += "\n"

	if len(s.Deployments) == 0 {
		return out + "no deployments\n"
	}
	rows := make([][]string, len(s.Deployments))
	for i, d := range s.Deployments {
		created := "-"
		if d.Created != nil {
			created = d.Created.Format(time.RFC3339)
		}
		rows[i] = []string{
			presentation.StatusCell(d.Status, presentation.StatusStyleIcon),
			fmt.Sprintf("%d/%d desired", d.Running, d.Desired),
			fmt.Sprintf("%d pending", d.Pending),
			created,
		}
	}
	table, err := utils.RenderTable(rows)
	if err != nil {
		return out + err.Error()
	}
	return out + table
}

func enabledDisabled(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

func formatDeployedAt(t *time.Time) string {
	if t == nil {
		return ""
	}
	return " (" + t.Format(time.RFC3339) + ")"
}

func (gui *Gui) renderECSServiceEvents(row *ecsRow) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string {
		s := row.Service
		if len(s.Events) == 0 {
			return "no recent events\n"
		}
		out := ""
		for _, ev := range s.Events {
			when := "-"
			if ev.When != nil {
				when = ev.When.Format(time.RFC3339)
			}
			out += fmt.Sprintf("%s  %s\n", utils.ColoredString(when, color.FgYellow), ev.Message)
		}
		return out
	})
}

func (gui *Gui) renderECSServiceScaling(row *ecsRow) tasks.TaskFunc {
	s := row.Service
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		scaling, err := gui.Client.GetECSServiceAutoScaling(fetchCtx, s.Cluster, s.Name)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error loading auto scaling: " + err.Error())
			return
		}
		gui.RenderStringMain(formatECSServiceScaling(scaling))
	}})
}

func formatECSServiceScaling(scaling *aws.ECSServiceAutoScaling) string {
	if scaling == nil {
		return "no Application Auto Scaling registered for this service\n"
	}
	out := utils.FormatMap(0, map[string]string{
		"Min capacity": strconv.Itoa(int(scaling.MinCapacity)),
		"Max capacity": strconv.Itoa(int(scaling.MaxCapacity)),
	})
	out += "\nPolicies:\n"
	if len(scaling.Policies) == 0 {
		return out + "none\n"
	}
	for _, p := range scaling.Policies {
		out += fmt.Sprintf("  %s (%s)\n", p.Name, p.Type)
		switch p.Type {
		case "TargetTrackingScaling":
			out += fmt.Sprintf("    target %.1f%s, scale-in cooldown %ds, scale-out cooldown %ds\n",
				p.TargetValue, formatScalingMetricSuffix(p.TargetMetric), p.ScaleInCooldownSecs, p.ScaleOutCooldownSecs)
		case "StepScaling":
			out += fmt.Sprintf("    %d step adjustment(s), cooldown %ds\n", p.StepAdjustments, p.ScaleOutCooldownSecs)
		}
	}
	return out
}

func formatScalingMetricSuffix(metric string) string {
	if metric == "" {
		return ""
	}
	return " (" + metric + ")"
}

// renderECSTaskConfig uses row data to avoid an unnecessary AWS call.
func (gui *Gui) renderECSTaskConfig(row *ecsRow) tasks.TaskFunc {
	return gui.NewSimpleRenderStringTask(func() string {
		return formatECSTaskConfig(row.Task)
	})
}

func formatECSTaskConfig(t *aws.ECSTask) string {
	cfg := t.Config
	out := utils.FormatMap(0, map[string]string{
		"ID":                t.ID,
		"Status":            t.Status,
		"Health":            t.Health,
		"Launch type":       cfg.LaunchType,
		"CPU":               cfg.CPU,
		"Memory":            cfg.Memory,
		"OS/Arch":           cfg.OperatingSystem + "/" + cfg.Architecture,
		"Platform version":  cfg.PlatformVersion,
		"Task definition":   cfg.TaskDefinition,
		"Task role":         cfg.TaskRole,
		"Execution role":    cfg.TaskExecutionRole,
		"ECS Exec":          cfg.ECSExec,
		"Capacity provider": cfg.CapacityProvider,
		"Network mode":      cfg.NetworkMode,
		"ENI":               cfg.ENIID,
		"Subnet":            cfg.SubnetID,
		"Private IP":        cfg.PrivateIP,
		"Public IP":         cfg.PublicIP,
		"Console":           t.ConsoleURL,
	})

	out += "\nContainers:\n"
	if len(t.Containers) == 0 {
		out += "none\n"
	}
	for _, ctr := range t.Containers {
		out += fmt.Sprintf("  %s %s (%s) %s\n",
			presentation.StatusCell(ctr.LastStatus, presentation.StatusStyleIcon),
			ctr.Name, ctr.ImageURI, ctr.HealthStatus)
		for _, p := range ctr.Ports {
			out += fmt.Sprintf("    host:%d -> container:%d/%s\n", p.HostPort, p.ContainerPort, p.Protocol)
		}
	}

	out += "\nAttachments:\n"
	if len(t.Attachments) == 0 {
		out += "none\n"
	}
	for _, att := range t.Attachments {
		out += "  " + att.Type + " " + utils.FormatMap(4, att.Details)
	}

	return out
}

// renderECSTaskDefDiff deliberately compares only the current and previous revisions.
func (gui *Gui) renderECSTaskDefDiff(taskDefArn string) tasks.TaskFunc {
	return gui.NewTask(TaskOpts{Func: func(ctx context.Context) {
		gen := gui.Gen
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		family := aws.TaskDefinitionFamily(taskDefArn)
		revisions, err := gui.Client.ListTaskDefinitions(fetchCtx, family)
		if gen != gui.Gen {
			return
		}
		if err != nil {
			gui.RenderStringMain("error listing task definition revisions: " + err.Error())
			return
		}

		var current, previous *aws.ECSTaskDefinitionDetail
		for i, r := range revisions {
			if r.Arn != taskDefArn {
				continue
			}
			current, _ = gui.Client.DescribeTaskDefinitionDetail(fetchCtx, r.Arn)
			if i+1 < len(revisions) {
				previous, _ = gui.Client.DescribeTaskDefinitionDetail(fetchCtx, revisions[i+1].Arn)
			}
			break
		}
		if gen != gui.Gen {
			return
		}
		gui.RenderStringMain(formatECSTaskDefDiff(family, revisions, taskDefArn, current, previous))
	}})
}

func formatECSTaskDefDiff(family string, revisions []aws.ECSTaskDefinitionRevision, currentArn string, current, previous *aws.ECSTaskDefinitionDetail) string {
	out := fmt.Sprintf("Family: %s\n\nRevisions:\n", family)
	if len(revisions) == 0 {
		out += "none\n"
	}
	for _, r := range revisions {
		marker := "  "
		if r.Arn == currentArn {
			marker = "->"
		}
		out += fmt.Sprintf("%s rev %d\n", marker, r.Revision)
	}

	out += "\nDiff (this revision vs previous):\n"
	if current == nil {
		return out + "no revision data\n"
	}
	if previous == nil {
		return out + "no previous revision to diff against\n"
	}
	lines := diffTaskDefinitions(previous, current)
	if len(lines) == 0 {
		return out + "no differences\n"
	}
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}

func diffTaskDefinitions(a, b *aws.ECSTaskDefinitionDetail) []string {
	var lines []string
	if a.CPU != b.CPU {
		lines = append(lines, fmt.Sprintf("task cpu: %s -> %s", a.CPU, b.CPU))
	}
	if a.Memory != b.Memory {
		lines = append(lines, fmt.Sprintf("task memory: %s -> %s", a.Memory, b.Memory))
	}

	aByName := containersByName(a.Containers)
	bByName := containersByName(b.Containers)
	names := make(map[string]struct{}, len(aByName)+len(bByName))
	for n := range aByName {
		names[n] = struct{}{}
	}
	for n := range bByName {
		names[n] = struct{}{}
	}
	sortedNames := make([]string, 0, len(names))
	for n := range names {
		sortedNames = append(sortedNames, n)
	}
	sort.Strings(sortedNames)

	for _, name := range sortedNames {
		ac, aok := aByName[name]
		bc, bok := bByName[name]
		switch {
		case !aok:
			lines = append(lines, fmt.Sprintf("container %s: added (%s)", name, bc.Image))
		case !bok:
			lines = append(lines, fmt.Sprintf("container %s: removed (was %s)", name, ac.Image))
		default:
			if ac.Image != bc.Image {
				lines = append(lines, fmt.Sprintf("container %s image: %s -> %s", name, ac.Image, bc.Image))
			}
			if ac.CPU != bc.CPU {
				lines = append(lines, fmt.Sprintf("container %s cpu: %d -> %d", name, ac.CPU, bc.CPU))
			}
			if ac.Memory != bc.Memory {
				lines = append(lines, fmt.Sprintf("container %s memory: %d -> %d", name, ac.Memory, bc.Memory))
			}
			lines = append(lines, diffEnv(name, ac.Environment, bc.Environment)...)
		}
	}
	return lines
}

func containersByName(containers []aws.ECSTaskDefinitionContainer) map[string]aws.ECSTaskDefinitionContainer {
	m := make(map[string]aws.ECSTaskDefinitionContainer, len(containers))
	for _, c := range containers {
		m[c.Name] = c
	}
	return m
}

func diffEnv(container string, a, b map[string]string) []string {
	var lines []string
	keys := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	sortedKeys := make([]string, 0, len(keys))
	for k := range keys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	for _, k := range sortedKeys {
		av, aok := a[k]
		bv, bok := b[k]
		switch {
		case !aok:
			lines = append(lines, fmt.Sprintf("container %s env %s: added (%s)", container, k, bv))
		case !bok:
			lines = append(lines, fmt.Sprintf("container %s env %s: removed", container, k))
		case av != bv:
			lines = append(lines, fmt.Sprintf("container %s env %s: %s -> %s", container, k, av, bv))
		}
	}
	return lines
}

// renderECSTaskLogs checks Gen on every tick so stale profile tails stop immediately.
func (gui *Gui) renderECSTaskLogs(row *ecsRow) tasks.TaskFunc {
	cluster := gui.ecsDrill.cluster
	taskArn := row.Task.Arn

	return gui.NewTickerTask(TickerTaskOpts{
		Duration:   config.RefreshInterval(gui.Config.User.Refresh.ECSLogsSeconds, 5),
		Before:     func(ctx context.Context) { gui.clearMainView() },
		Autoscroll: true,
		Wrap:       true,
		Func: func(ctx context.Context, notifyStopped chan struct{}) {
			gen := gui.Gen
			fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			streams, err := gui.Client.GetECSTaskLogs(fetchCtx, cluster, taskArn, 200)
			if gen != gui.Gen {
				return
			}
			if err != nil {
				gui.reRenderStringMain("error loading logs: " + err.Error())
				return
			}
			gui.reRenderStringMain(formatECSLogStreams(streams))
		},
	})
}

func formatECSLogStreams(streams []aws.ECSLogStream) string {
	if len(streams) == 0 {
		return "no logs configured for this task (container has no awslogs driver)"
	}

	type line struct {
		when      time.Time
		container string
		message   string
	}
	var lines []line
	for _, s := range streams {
		for _, ev := range s.Events {
			lines = append(lines, line{ev.Timestamp, s.Container, ev.Message})
		}
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].when.Before(lines[j].when) })

	out := ""
	for _, l := range lines {
		out += fmt.Sprintf("%s %s %s\n",
			l.when.Format("15:04:05"),
			utils.ColoredString(l.container, color.FgYellow),
			l.message)
	}
	if out == "" {
		return "no log events yet"
	}
	return out
}

func ecsDrillTitle(state ecsDrillState) string {
	switch state.level {
	case ecsLevelServices:
		return "ECS: " + state.cluster
	case ecsLevelTasks:
		return "ECS: " + state.cluster + "/" + state.service
	default:
		return "ECS"
	}
}

func ecsDrillDown(state ecsDrillState, clusterName, serviceName string) (ecsDrillState, string) {
	var next ecsDrillState
	switch state.level {
	case ecsLevelClusters:
		next = ecsDrillState{level: ecsLevelServices, cluster: clusterName}
	case ecsLevelServices:
		next = ecsDrillState{level: ecsLevelTasks, cluster: state.cluster, service: serviceName}
	default:
		next = state
	}

	return next, ecsDrillTitle(next)
}

func ecsDrillUp(state ecsDrillState) (ecsDrillState, string) {
	var next ecsDrillState
	switch state.level {
	case ecsLevelTasks:
		next = ecsDrillState{level: ecsLevelServices, cluster: state.cluster}
	case ecsLevelServices:
		next = ecsDrillState{}
	default:
		next = state
	}

	return next, ecsDrillTitle(next)
}

// handleECSDrillDown must remain ahead of generic Enter dispatch so only the deepest level focuses main.
func (gui *Gui) handleECSDrillDown(g *gocui.Gui, v *gocui.View) error {
	if gui.ecsDrill.level == ecsLevelTasks {
		return gui.handleEnterMain(g, v)
	}

	row, err := gui.Panels.ECS.GetSelectedItem()
	if err != nil {
		return nil
	}

	next, title := ecsDrillDown(gui.ecsDrill, row.name(), row.name())
	gui.ecsDrill = next
	gui.Views.ECS.Title = title

	return gui.drillECS()
}

func (gui *Gui) handleECSEscape(g *gocui.Gui, v *gocui.View) error {
	if gui.ecsDrill.level == ecsLevelClusters {
		return gui.escape()
	}

	next, title := ecsDrillUp(gui.ecsDrill)
	gui.ecsDrill = next
	gui.Views.ECS.Title = title

	return gui.drillECS()
}

func (gui *Gui) drillECS() error {
	gui.Panels.ECS.SetSelectedLineIdx(0)
	// The tab set differs per drill level (cluster/service/task); drop any index carried over from the previous level's tabs.
	gui.Panels.ECS.SetMainTabIndex(0)
	gui.Panels.ECS.SetItems(nil)
	if err := gui.Panels.ECS.RerenderList(); err != nil {
		return err
	}
	return gui.loadECSList()
}

func (gui *Gui) loadECSList() error {
	if gui.Client == nil {
		return nil
	}

	gen := gui.Gen
	level, cluster, service := gui.ecsDrill.level, gui.ecsDrill.cluster, gui.ecsDrill.service

	return gui.WithWaitingStatus("loading ecs", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		rows, err := gui.fetchECSRows(ctx, level, cluster, service)
		if err != nil {
			return err
		}
		if gen != gui.Gen {
			return nil
		}

		gui.Panels.ECS.SetItemsKeepSelection(rows, ecsSelectionKey)
		return gui.Panels.ECS.RerenderList()
	})
}

// ecsSelectionKey identifies a cluster, service or task row across reloads. The drill level is not part of it because drillECS empties the list before loading the next level.
func ecsSelectionKey(row *ecsRow) string { return row.arn() }

func (gui *Gui) fetchECSRows(ctx context.Context, level ecsDrillLevel, cluster, service string) ([]*ecsRow, error) {
	switch level {
	case ecsLevelServices:
		services, err := gui.Client.ListECSServices(ctx, cluster)
		if err != nil {
			return nil, err
		}
		rows := make([]*ecsRow, len(services))
		for i := range services {
			rows[i] = &ecsRow{Kind: ecsRowKindService, Service: &services[i]}
		}
		return rows, nil
	case ecsLevelTasks:
		ecsTasks, err := gui.Client.ListECSTasks(ctx, cluster, service)
		if err != nil {
			return nil, err
		}
		rows := make([]*ecsRow, len(ecsTasks))
		for i := range ecsTasks {
			rows[i] = &ecsRow{Kind: ecsRowKindTask, Task: &ecsTasks[i]}
		}
		return rows, nil
	default:
		clusters, err := gui.Client.ListECSClusters(ctx)
		if err != nil {
			return nil, err
		}
		rows := make([]*ecsRow, len(clusters))
		for i := range clusters {
			rows[i] = &ecsRow{Kind: ecsRowKindCluster, Cluster: &clusters[i]}
		}
		return rows, nil
	}
}
