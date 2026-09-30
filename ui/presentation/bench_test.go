package presentation

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
)

// fatih/color disables itself when stdout is not a terminal, which it never is under `go test`.
// Without this the render benchmarks skip every escape sequence and measure a path no production run takes, because gocui always owns a tty.
func benchForceColor(b *testing.B) {
	b.Helper()

	previous := color.NoColor
	b.Cleanup(func() { color.NoColor = previous })
	color.NoColor = false
}

// benchBlock is a section-shaped block: a heading and rows, which is what the overviews hand the zipper.
func benchBlock(prefix string, lines int) string {
	rows := make([]string, lines)
	for i := range rows {
		rows[i] = prefix + " row " + strconv.Itoa(i) + ": some value worth reading"
	}

	return strings.Join(rows, "\n")
}

// Columns runs once per overview render, and the two-column path is the one that interleaves and cuts line by line.
func BenchmarkColumns(b *testing.B) {
	left, right := benchBlock("left", 40), benchBlock("right", 40)
	b.ReportAllocs()

	for b.Loop() {
		_ = Columns(overviewWidth, 1, left, right)
	}
}

// The stacked path below minTwoColWidth is different code, not a cheaper version of the same code.
func BenchmarkColumnsStacked(b *testing.B) {
	left, right := benchBlock("left", 40), benchBlock("right", 40)
	b.ReportAllocs()

	for b.Loop() {
		_ = Columns(stackedWidth, 1, left, right)
	}
}

// The fixtures are built once: allocating them per iteration would report the fixture's cost as the formatter's.
// They are the same full fixtures the render tests use, so a formatter measured here is measured with every section answered, which is the expensive case and the one on screen.
func BenchmarkFormatInstanceOverview(b *testing.B) {
	benchForceColor(b)
	instance, overview := overviewInstance(), fullOverview()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatInstanceOverview(instance, overview, overviewWidth, overviewNow)
	}
}

// A narrow terminal lays every section out whole instead of cutting it to a column, so it renders more text, not less.
func BenchmarkFormatInstanceOverviewStacked(b *testing.B) {
	benchForceColor(b)
	instance, overview := overviewInstance(), fullOverview()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatInstanceOverview(instance, overview, stackedWidth, overviewNow)
	}
}

func BenchmarkFormatECSClusterOverview(b *testing.B) {
	benchForceColor(b)
	cluster, overview := clusterFixture()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatECSClusterOverview(cluster, overview, overviewWidth)
	}
}

func BenchmarkFormatECSServiceOverview(b *testing.B) {
	benchForceColor(b)
	service, overview, now := serviceFixture()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatECSServiceOverview(service, overview, overviewWidth, now)
	}
}

func BenchmarkFormatBucketOverview(b *testing.B) {
	benchForceColor(b)
	bucket, overview := overviewBucket(), fullBucketOverview()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatBucketOverview(bucket, overview, overviewWidth, overviewNow)
	}
}

func BenchmarkFormatECRRepositoryOverview(b *testing.B) {
	benchForceColor(b)
	repository, images := overviewRepository(), overviewImages()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatECRRepositoryOverview(repository, overviewPolicies(), images, nil, overviewWidth, overviewNow)
	}
}

func BenchmarkFormatVPCOverview(b *testing.B) {
	benchForceColor(b)
	vpc, overview := overviewVPC(), fullVPCOverview()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatVPCOverview(vpc, overview, overviewWidth)
	}
}

func BenchmarkFormatVPCEndpointServiceOverview(b *testing.B) {
	benchForceColor(b)
	service, connections := benchEndpointService(40)
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatVPCEndpointServiceOverview(service, connections, nil, 160)
	}
}

// A narrow terminal lays every section out whole instead of cutting it to a column, so it renders more text, not less.
func BenchmarkFormatVPCEndpointServiceOverviewStacked(b *testing.B) {
	benchForceColor(b)
	service, connections := benchEndpointService(40)
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatVPCEndpointServiceOverview(service, connections, nil, 70)
	}
}

// The cells are built once per visible row on every list render, which is the tightest budget in this package.
func BenchmarkGetVPCEndpointServiceDisplayCells(b *testing.B) {
	benchForceColor(b)
	service, _ := benchEndpointService(0)
	b.ReportAllocs()

	for b.Loop() {
		_ = GetVPCEndpointServiceDisplayCells(service)
	}
}

// benchEndpointService is a service with every block filled, because a pane that renders half its sections measures half the work.
func benchEndpointService(connections int) (*aws.VPCEndpointService, []aws.VPCEndpointConnection) {
	service := &aws.VPCEndpointService{
		ID:                  "vpce-svc-0123456789abcdef0",
		Name:                "com.amazonaws.vpce.eu-west-1.vpce-svc-0123456789abcdef0",
		NameTag:             "payments",
		State:               "Available",
		AcceptanceRequired:  true,
		ManagesEndpoints:    true,
		PrivateDNSName:      "api.internal.example",
		PayerResponsibility: "ServiceOwner",
		AvailabilityZones:   []string{"eu-west-1a", "eu-west-1b", "eu-west-1c"},
		BaseDNSNames:        []string{"vpce-svc-0123456789abcdef0.eu-west-1.vpce.amazonaws.com"},
		LoadBalancerARNs:    []string{"arn:aws:elasticloadbalancing:eu-west-1:111122223333:loadbalancer/net/api-nlb/abc123"},
		SupportedIPTypes:    []string{"ipv4", "dualstack"},
		Tags:                []aws.Tag{{Key: "team", Value: "platform"}, {Key: "env", Value: "prod"}},
		Pending:             connections / 4,
	}

	created := overviewNow.Add(-6 * time.Hour)
	rows := make([]aws.VPCEndpointConnection, connections)
	for i := range rows {
		state := "available"
		if i%4 == 0 {
			state = aws.VPCEndpointStatePendingAcceptance
		}
		rows[i] = aws.VPCEndpointConnection{
			ServiceID:        service.ID,
			EndpointID:       "vpce-0123456789abcde" + strconv.Itoa(i),
			Owner:            "11112222333" + strconv.Itoa(i%10),
			State:            state,
			Region:           "eu-west-1",
			IPAddressType:    "ipv4",
			CreatedAt:        &created,
			DNSNames:         []string{"api.internal.example"},
			LoadBalancerARNs: service.LoadBalancerARNs,
		}
	}

	return service, rows
}

func BenchmarkFormatEKSClusterOverview(b *testing.B) {
	benchForceColor(b)
	cluster, overview := overviewEKSCluster(), fullEKSOverview()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatEKSClusterOverview(cluster, overview, overviewWidth)
	}
}

func BenchmarkFormatSecretOverview(b *testing.B) {
	benchForceColor(b)
	secret := rotatingSecret()
	b.ReportAllocs()

	for b.Loop() {
		_ = FormatSecretOverview(secret, overviewWidth, overviewNow)
	}
}
