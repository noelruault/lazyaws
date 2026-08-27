package presentation

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// BucketWeights sizes the bucket row so the name absorbs the slack; region and creation date are fixed-width.
func BucketWeights() []int {
	return []int{1, 0, 0}
}

// GetBucketDisplayCells leaves region unknown because ListBuckets omits it.
func GetBucketDisplayCells(b *aws.Bucket) []utils.Cell {
	return []utils.Cell{
		{Text: b.Name},
		{Text: b.Region},
		{Text: b.CreationDate},
	}
}

// bucketRulesShown caps the lifecycle and replication listings. A bucket can carry a thousand lifecycle rules; the overview is a glance and the Config tab is the list.
const bucketRulesShown = 5

// FormatBucketOverview re-lays the Config tab's data as the bucket's Overview: what can reach it, what happens to its objects, who is watching it.
// The size is deliberately absent — it is a full object scan — so this pane costs exactly the calls the Config tab already made.
func FormatBucketOverview(b *aws.Bucket, o *aws.BucketOverview, width int, now time.Time) string {
	// Cut to the pane: the header spans the full width rather than a column, so Columns never measures it, and with wrap off a long bucket name plus its badge runs off the edge unmarked.
	header := truncateBlock(ResourceHeader("Bucket", b.Name, bucketExposureBadge(o), "", bucketRegion(o), bucketCreated(b, now)), width)

	left := joinBlocks(bucketSecurityBlock(o), bucketDataBlock(o))
	right := joinBlocks(bucketAccessBlock(o), bucketTagsBlock(o))

	return header + "\n\n" + Columns(width, overviewGap, left, right)
}

// bucketExposureBadge answers the one question a bucket is opened for. The word carries the state and the colour only reinforces it.
// An absent public access block is reported as not blocked rather than as unknown: S3 answers "no configuration" for a bucket that has none, and that genuinely means nothing at the bucket level is stopping a public ACL or policy.
func bucketExposureBadge(o *aws.BucketOverview) string {
	if err := o.Err(aws.SectionPublicAccess); err != nil {
		return utils.ColoredString("Public access unknown", color.FgYellow)
	}
	if o.PublicAccess == nil {
		return utils.ColoredString("Public access not blocked", color.FgRed)
	}

	blocked := 0
	for _, on := range publicAccessFlags(o.PublicAccess) {
		if on {
			blocked++
		}
	}

	switch blocked {
	case len(publicAccessFlags(o.PublicAccess)):
		return utils.ColoredString("Public access blocked", color.FgGreen)
	case 0:
		return utils.ColoredString("Public access not blocked", color.FgRed)
	default:
		return utils.ColoredString(fmt.Sprintf("Public access partly blocked (%d/4)", blocked), color.FgYellow)
	}
}

// publicAccessFlags keeps the badge's count and the security block's rows reading the same four settings, in the order the S3 console lists them.
func publicAccessFlags(p *aws.PublicAccessBlock) []bool {
	return []bool{p.BlockPublicAcls, p.IgnorePublicAcls, p.BlockPublicPolicy, p.RestrictPublicBuckets}
}

// bucketRegion prefers the located region over the list row's placeholder: ListBuckets does not report a region, so the row carries "-" until GetBucketRegion answers.
func bucketRegion(o *aws.BucketOverview) string {
	if err := o.Err(aws.SectionRegion); err != nil {
		return "region unavailable"
	}

	return orNone(o.Region)
}

// bucketCreated pairs the creation date with its age. The list maps it with a space between date and time rather than as RFC3339, so an unparseable value degrades to the string itself instead of to a wrong age.
func bucketCreated(b *aws.Bucket, now time.Time) string {
	if b.CreationDate == "" {
		return ""
	}

	at, err := time.Parse(time.DateTime, b.CreationDate)
	if err != nil {
		return "created " + b.CreationDate
	}

	return "created " + b.CreationDate + " (" + RelTime(at.UTC(), now) + ")"
}

func bucketSecurityBlock(o *aws.BucketOverview) string {
	rows := []kv{{"Public access", bucketPublicAccessLine(o)}}
	if o.Err(aws.SectionPublicAccess) == nil && o.PublicAccess != nil {
		p := o.PublicAccess
		rows = append(rows,
			kv{"  Block ACLs", yesNo(p.BlockPublicAcls)},
			kv{"  Ignore ACLs", yesNo(p.IgnorePublicAcls)},
			kv{"  Block policy", yesNo(p.BlockPublicPolicy)},
			kv{"  Restrict public", yesNo(p.RestrictPublicBuckets)},
		)
	}

	rows = append(rows,
		kv{"Encryption", fieldOr(o.Err(aws.SectionEncryption), bucketEncryptionLine(o.Encryption))},
		kv{"Bucket policy", fieldOr(o.Err(aws.SectionPolicy), bucketPolicyLine(o.PolicyPresent))},
	)

	return SectionTitle("Security") + "\n" + kvBlock(rows)
}

func bucketPublicAccessLine(o *aws.BucketOverview) string {
	if err := o.Err(aws.SectionPublicAccess); err != nil {
		return fieldOr(err, "")
	}
	if o.PublicAccess == nil {
		return utils.ColoredString("no block configuration", color.FgRed)
	}

	return "block configured"
}

// bucketEncryptionLine says which key protects the objects, because "encrypted" is not the question an auditor asks of a bucket: SSE-S3 and SSE-KMS differ in who can decrypt.
// A bucket with no configuration is still encrypted — S3 applies SSE-S3 by default — so the absence is reported as the default rather than as "off".
func bucketEncryptionLine(e *aws.BucketEncryption) string {
	if e == nil {
		return "AES256 (S3 default, not configured on the bucket)"
	}
	if e.KMSKeyID != "" {
		return e.Algorithm + " · " + e.KMSKeyID
	}

	return orNone(e.Algorithm)
}

func bucketPolicyLine(present bool) string {
	if present {
		return "attached, shown on the Policy tab"
	}

	return "none"
}

func bucketDataBlock(o *aws.BucketOverview) string {
	rows := []kv{
		{"Versioning", fieldOr(o.Err(aws.SectionVersioning), orNone(o.Versioning))},
		{"Object lock", fieldOr(o.Err(aws.SectionObjectLock), bucketObjectLockLine(o.ObjectLock))},
	}

	lines := []string{SectionTitle("Data management"), kvBlock(rows)}
	lines = append(lines, bucketRuleLines("Lifecycle", o.Err(aws.SectionLifecycle), lifecycleRuleLines(o.Lifecycle))...)
	lines = append(lines, bucketRuleLines("Replication", o.Err(aws.SectionReplication), replicationRuleLines(o.Replication))...)

	return strings.Join(lines, "\n")
}

// bucketRuleLines renders a rule list as a count plus its rules, capped, so a bucket with fifty lifecycle rules does not push every later section off the pane.
func bucketRuleLines(label string, err error, rules []string) []string {
	if err != nil {
		return []string{utils.ColoredString(label+":", color.FgYellow) + " " + fieldOr(err, "")}
	}
	if len(rules) == 0 {
		return []string{utils.ColoredString(label+":", color.FgYellow) + " none"}
	}

	shown := rules
	if len(shown) > bucketRulesShown {
		shown = shown[:bucketRulesShown]
	}

	lines := []string{utils.ColoredString(label+":", color.FgYellow) + " " + pluralize(len(rules), "rule")}
	for _, rule := range shown {
		lines = append(lines, "  "+rule)
	}
	if hidden := len(rules) - len(shown); hidden > 0 {
		lines = append(lines, utils.ColoredString(fmt.Sprintf("  (%d more on the Config tab)", hidden), color.Faint))
	}

	return lines
}

func bucketObjectLockLine(l *aws.ObjectLockConfiguration) string {
	if l == nil || !l.Enabled {
		return "not configured"
	}
	if l.DefaultRetentionMode == "" {
		return "enabled, no default retention"
	}
	if l.DefaultRetentionDays > 0 {
		return fmt.Sprintf("enabled · %s · %dd default", l.DefaultRetentionMode, l.DefaultRetentionDays)
	}

	return "enabled · " + l.DefaultRetentionMode
}

func lifecycleRuleLines(c *aws.LifecycleConfiguration) []string {
	if c == nil {
		return nil
	}

	lines := make([]string, 0, len(c.Rules))
	for _, rule := range c.Rules {
		parts := []string{ruleLabel(rule.ID, rule.Status), lifecycleScope(rule)}
		for _, t := range rule.Transitions {
			parts = append(parts, "→ "+t.StorageClass+" "+afterWhen(t.Days, t.Date))
		}
		if when := afterWhen(rule.Expiration.Days, rule.Expiration.Date); when != "" {
			parts = append(parts, "expire "+when)
		}
		lines = append(lines, strings.Join(parts, " · "))
	}

	return lines
}

// lifecycleScope names what the rule matches. A rule with neither a prefix nor a filter applies to the whole bucket, and leaving that blank reads as a rule that does nothing.
func lifecycleScope(rule aws.LifecycleRule) string {
	if rule.Prefix != "" {
		return rule.Prefix
	}
	if rule.Filter != "" {
		return rule.Filter
	}

	return "all objects"
}

// afterWhen renders a lifecycle age as either its day count or its absolute date, which is how S3 stores the two forms; a rule carrying neither returns empty so the caller can drop the clause.
func afterWhen(days int, date string) string {
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}

	return date
}

func replicationRuleLines(r *aws.BucketReplication) []string {
	if r == nil {
		return nil
	}

	lines := make([]string, 0, len(r.Rules))
	for _, rule := range r.Rules {
		line := ruleLabel(rule.ID, rule.Status) + " → " + orNone(rule.DestinationBucket)
		if rule.DestinationRegion != "" {
			line += " (" + rule.DestinationRegion + ")"
		}
		lines = append(lines, line)
	}

	return lines
}

// ruleLabel prefixes a rule with its id and colours the status word, since a Disabled rule listed beside enabled ones is the thing most easily misread.
func ruleLabel(id, status string) string {
	label := "[" + orNone(id) + "]"
	if !strings.EqualFold(status, "Enabled") {
		return label + " " + utils.ColoredString(orNone(status), color.Faint)
	}

	return label + " " + status
}

func bucketAccessBlock(o *aws.BucketOverview) string {
	rows := []kv{
		{"Access logging", fieldOr(o.Err(aws.SectionLogging), bucketLoggingLine(o.Logging))},
		{"Notifications", fieldOr(o.Err(aws.SectionNotifications), bucketNotificationsLine(o.Notifications))},
	}

	return SectionTitle("Access") + "\n" + kvBlock(rows)
}

func bucketLoggingLine(l *aws.BucketLogging) string {
	if l == nil || l.TargetBucket == "" {
		return "disabled"
	}
	if l.TargetPrefix != "" {
		return l.TargetBucket + "/" + l.TargetPrefix
	}

	return l.TargetBucket
}

// bucketNotificationsLine counts the destinations by kind rather than listing them: which Lambda fires is a Config-tab question, whether anything fires at all is an overview one.
func bucketNotificationsLine(n *aws.NotificationConfig) string {
	if n == nil {
		return "none"
	}

	var parts []string
	if count := len(n.LambdaFunctions); count > 0 {
		parts = append(parts, fmt.Sprintf("%d lambda", count))
	}
	if count := len(n.Topics); count > 0 {
		parts = append(parts, fmt.Sprintf("%d topic", count))
	}
	if count := len(n.Queues); count > 0 {
		parts = append(parts, fmt.Sprintf("%d queue", count))
	}
	if len(parts) == 0 {
		return "none"
	}

	return strings.Join(parts, ", ")
}

func bucketTagsBlock(o *aws.BucketOverview) string {
	title := SectionTitle("Tags")
	if err := o.Err(aws.SectionTags); err != nil {
		return title + "\n" + fieldOr(err, "")
	}
	if len(o.Tags) == 0 {
		return title + "\nnone"
	}

	// Go randomizes map iteration, so an unsorted tag block would reorder itself on every re-render of the same bucket.
	keys := make([]string, 0, len(o.Tags))
	for key := range o.Tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := []string{title}
	for _, key := range keys {
		lines = append(lines, key+": "+orNone(o.Tags[key]))
	}

	return strings.Join(lines, "\n")
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}

	return "no"
}
