package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/noelruault/lazyaws/internal/aws"
	"github.com/noelruault/lazyaws/internal/config"
	uiEC2 "github.com/noelruault/lazyaws/internal/ui/ec2"
	uiECR "github.com/noelruault/lazyaws/internal/ui/ecr"
	uiECS "github.com/noelruault/lazyaws/internal/ui/ecs"
	uiEKS "github.com/noelruault/lazyaws/internal/ui/eks"
	uiS3 "github.com/noelruault/lazyaws/internal/ui/s3"
	uiShared "github.com/noelruault/lazyaws/internal/ui/shared"
	"github.com/noelruault/lazyaws/internal/vim"
)

type screen int

const (
	authMethodScreen screen = iota
	authProfileScreen
	ssoConfigScreen
	accountScreen
	regionScreen
	ec2Screen
	ec2DetailsScreen
	s3Screen
	s3BrowseScreen
	s3ObjectDetailsScreen
	eksScreen
	eksDetailsScreen
	ecsScreen
	ecsServicesScreen
	ecsTasksScreen
	ecsTaskDetailsScreen
	ecrScreen
	ecrImagesScreen
	ecrImageDetailsScreen
	secretsScreen
	secretsDetailsScreen
	helpScreen
	serviceScreen
)

// Auth method indices
const (
	authMethodEnvVars = 0
	authMethodProfile = 1
	authMethodSSO     = 2
	maxAuthMethod     = authMethodSSO
)

type model struct {
	currentScreen            screen
	width                    int
	height                   int
	awsClient                *aws.Client
	ec2                      uiEC2.State
	s3                       uiS3.State
	eks                      uiEKS.State
	ecs                      uiECS.State
	ecr                      uiECR.State
	secrets                  []aws.SecretSummary
	secretsFiltered          []aws.SecretSummary
	selectedSecretIndex      int
	currentSecretDetails     *aws.SecretDetails
	deleteConfirmInput       textinput.Model // For typing confirmation
	loading                  bool
	err                      error
	config                   *config.Config
	filterInput              textinput.Model
	ssoURLInput              textinput.Model
	ssoRegionInput           textinput.Model
	profileInput             textinput.Model
	filtering                bool
	configuringSSO           bool
	configuringProfile       bool
	selectedAuthMethod       int // For auth method selection screen
	filter                   string
	statusMessage            string
	autoRefresh              bool
	autoRefreshInterval      int // in seconds
	copyToClipboard          string
	vimState                 *vim.State
	pageSize                 int      // For VIM page navigation
	viewportOffset           int      // Scroll offset for current view
	commandSuggestions       []string // Command suggestions for tab completion
	ssmInstanceID            string   // Store instance ID for SSM session launch
	ssmRegion                string   // Store region for SSM session launch
	ssoAuthenticator         *aws.SSOAuthenticator
	ssoCredentials           *aws.SSOCredentials // Current SSO credentials for passing to CLI
	ssoAccounts              []aws.SSOAccount
	envAccountTarget         string // Optional account name/id from AWS_ACCOUNT env var
	envAccountAutoSwitchDone bool   // Prevent repeated auto-switch attempts
	ssoFilteredAccounts      []aws.SSOAccount
	ssoSelectedIndex         int
	serviceSelectedIndex     int    // Selected service in service selection screen
	regionSelectedIndex      int    // Selected region in region selection screen
	previousScreen           screen // Screen to return to after region selection
	authConfig               *aws.AuthConfig
	currentAccountID         string
	currentAccountName       string
	screenHistory            []screen // navigation history (FIFO-ish with forward/back support)
	historyIndex             int      // current position in history
}

type instancesLoadedMsg struct {
	instances []aws.Instance
	err       error
}

type instanceDetailsLoadedMsg struct {
	details *aws.InstanceDetails
	err     error
}

type instanceStatusLoadedMsg struct {
	status *aws.InstanceStatus
	err    error
}

type instanceMetricsLoadedMsg struct {
	metrics *aws.InstanceMetrics
	err     error
}

type ssmStatusLoadedMsg struct {
	status *aws.SSMConnectionStatus
	err    error
}

type instanceActionCompletedMsg struct {
	action string
	err    error
}

type bucketsLoadedMsg struct {
	buckets []aws.Bucket
	err     error
}

type objectsLoadedMsg struct {
	result *aws.S3ListResult
	err    error
}

type objectDetailsLoadedMsg struct {
	details *aws.S3ObjectDetails
	err     error
}

type fileOperationCompletedMsg struct {
	operation string
	err       error
}

type tickMsg struct{}

type bulkActionCompletedMsg struct {
	action       string
	successCount int
	failureCount int
}

type s3ActionCompletedMsg struct {
	action string
	err    error
}

type presignedURLGeneratedMsg struct {
	url string
	err error
}

type bucketPolicyLoadedMsg struct {
	policy string
	err    error
}

type bucketVersioningLoadedMsg struct {
	versioning string
	err        error
}

type ecsClustersLoadedMsg struct {
	clusters []aws.ECSCluster
	err      error
}

type launchSSMSessionMsg struct {
	instanceID string
	region     string
}

type ssmRestoreInfo struct {
	ssoCredentials *aws.SSOCredentials
	accountID      string
	accountName    string
	region         string
}

type s3RestoreInfo struct {
	bucket         string
	prefix         string
	screen         screen
	ssoCredentials *aws.SSOCredentials
	accountID      string
	accountName    string
}

type eksClustersLoadedMsg struct {
	clusters []aws.EKSCluster
	err      error
}

type eksClusterDetailsLoadedMsg struct {
	details    *aws.EKSClusterDetails
	nodeGroups []aws.EKSNodeGroup
	addons     []aws.EKSAddon
	err        error
}

type ssoAuthCompletedMsg struct {
	authenticator *aws.SSOAuthenticator
	err           error
}

type ssoAccountsLoadedMsg struct {
	accounts []aws.SSOAccount
	err      error
}

type accountSwitchedMsg struct {
	client      *aws.Client
	accountID   string
	accountName string
	credentials *aws.SSOCredentials // SSO credentials for CLI commands
	err         error
}

type kubeconfigUpdatedMsg struct {
	clusterName string
	err         error
}

type ecsServicesLoadedMsg struct {
	cluster  string
	services []aws.ECSService
	err      error
}

type ecsTasksLoadedMsg struct {
	cluster string
	service string
	tasks   []aws.ECSTask
	err     error
}

type ecsTaskLogsLoadedMsg struct {
	taskArn string
	logs    []aws.ECSLogStream
	err     error
}

type ecrReposLoadedMsg struct {
	repos []aws.ECRRepository
	err   error
}

type ecrImagesLoadedMsg struct {
	repo   string
	images []aws.ECRImage
	err    error
}

type ecrScanLoadedMsg struct {
	repo   string
	digest string
	scan   *aws.ECRScanResult
	err    error
}

type secretsLoadedMsg struct {
	secrets []aws.SecretSummary
	err     error
}

type secretDetailsLoadedMsg struct {
	details *aws.SecretDetails
	err     error
}

type ssoConfigSavedMsg struct {
	config *aws.SSOConfig
	err    error
}

func initialModel(cfg *config.Config) model {
	// Filter input
	ti := textinput.New()
	ti.Placeholder = "<name>, <id>, state=<state> or tag:key=value"
	ti.Focus()
	ti.CharLimit = 20
	ti.Width = 20

	// SSO URL input
	ssoInput := textinput.New()
	ssoInput.Placeholder = "https://d-xxxxxxxxxx.awsapps.com/start"
	ssoInput.Focus()
	ssoInput.CharLimit = 256
	ssoInput.Width = 80

	// SSO region input
	ssoRegionInput := textinput.New()
	ssoRegionInput.Placeholder = aws.DefaultSSORegion
	ssoRegionInput.CharLimit = 32
	ssoRegionInput.Width = 20

	// Profile name input
	profileInput := textinput.New()
	profileInput.Placeholder = "default"
	profileInput.Focus()
	profileInput.CharLimit = 64
	profileInput.Width = 40

	// Delete confirmation input
	deleteInput := textinput.New()
	deleteInput.Placeholder = "Type name to confirm"
	deleteInput.CharLimit = 256
	deleteInput.Width = 80

	// Load auth config if available
	authConfig, _ := aws.LoadAuthConfig()
	envAccount := strings.TrimSpace(os.Getenv("AWS_ACCOUNT"))
	if authConfig != nil && authConfig.Method == aws.AuthMethodSSO {
		if authConfig.SSOStartURL != "" {
			ssoInput.SetValue(authConfig.SSOStartURL)
		}
		if authConfig.SSORegion != "" {
			ssoRegionInput.SetValue(authConfig.SSORegion)
		} else {
			ssoRegionInput.SetValue(aws.DefaultSSORegion)
		}
	} else {
		ssoRegionInput.SetValue(aws.DefaultSSORegion)
	}

	// Determine starting screen
	startScreen := serviceScreen
	shouldLoad := false
	if authConfig == nil {
		startScreen = authMethodScreen
	} else if authConfig.Method == aws.AuthMethodSSO {
		// For SSO, start at account selection screen and trigger SSO auth
		startScreen = accountScreen
		shouldLoad = true // Will trigger SSO authentication
	} else {
		// For env vars or profile, start at service selection and load client
		shouldLoad = true
	}

	return model{
		currentScreen:        startScreen,
		loading:              shouldLoad,
		config:               cfg,
		filterInput:          ti,
		ssoURLInput:          ssoInput,
		ssoRegionInput:       ssoRegionInput,
		profileInput:         profileInput,
		deleteConfirmInput:   deleteInput,
		authConfig:           authConfig,
		configuringSSO:       false,
		configuringProfile:   false,
		selectedAuthMethod:   0,
		filtering:            false,
		autoRefresh:          false,
		autoRefreshInterval:  30, // Default 30 seconds
		vimState:             vim.NewState(),
		pageSize:             20, // Default page size for ctrl+d/ctrl+u
		envAccountTarget:     envAccount,
		serviceSelectedIndex: 0,
		screenHistory:        []screen{startScreen},
		historyIndex:         0,
		ec2: uiEC2.State{
			SelectedInstances: make(map[string]bool),
		},
		s3: uiS3.State{
			SelectedBucketIndex: 0,
			SelectedObjectIndex: 0,
		},
		eks:                  uiEKS.State{},
		ecs:                  uiECS.State{},
		ecr:                  uiECR.State{},
		secrets:              []aws.SecretSummary{},
		secretsFiltered:      nil,
		selectedSecretIndex:  0,
		currentSecretDetails: nil,
	}
}

func (m model) Init() tea.Cmd {
	// If we need to restore S3 state after editing, trigger the load
	if m.s3.NeedRestore && m.s3.CurrentBucket != "" && m.awsClient != nil {
		return m.loadS3Objects(m.s3.CurrentBucket, m.s3.CurrentPrefix, nil)
	}

	// If we need to restore EC2 state after SSM session, trigger the load
	if m.ec2.NeedRestore && m.awsClient != nil {
		return m.loadEC2Instances
	}

	// If auth config doesn't exist, stay on auth method selection screen
	if m.authConfig == nil {
		return nil
	}

	// For SSO, start authentication flow immediately
	if m.authConfig.Method == aws.AuthMethodSSO {
		return m.authenticateSSO(m.authConfig.SSOStartURL, m.authConfig.SSORegion)
	}

	// For other methods, initialize the client
	return m.initAWSClient
}

func (m model) initAWSClient() tea.Msg {
	ctx := context.Background()
	var client *aws.Client
	var err error

	if m.authConfig == nil {
		// No auth config, use default (env vars or ~/.aws/config)
		client, err = aws.NewClient(ctx, m.config)
	} else {
		switch m.authConfig.Method {
		case aws.AuthMethodEnv:
			// Use environment variables (AWS SDK handles this automatically)
			client, err = aws.NewClient(ctx, m.config)
		case aws.AuthMethodProfile:
			// Use specific AWS profile
			client, err = aws.NewClientWithProfile(ctx, m.authConfig.ProfileName)
			if client != nil {
				// Override region from profile if configured
				client.Region = m.config.Region
			}
		case aws.AuthMethodSSO:
			// SSO: Don't initialize client yet - wait for SSO authentication and account selection
			// This prevents using environment variables or other credential sources
			// The client will be created in switchToSSOAccount after authentication
			return nil
		default:
			client, err = aws.NewClient(ctx, m.config)
		}
	}

	if err != nil {
		return instancesLoadedMsg{err: err}
	}
	return client
}

// SSO authentication and account switching functions
func (m model) authenticateSSO(startURL, region string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		authenticator := aws.NewSSOAuthenticator(startURL, region)
		err := authenticator.Authenticate(ctx)
		return ssoAuthCompletedMsg{authenticator: authenticator, err: err}
	}
}

func (m model) loadSSOAccounts() tea.Cmd {
	return func() tea.Msg {
		if m.ssoAuthenticator == nil {
			return ssoAccountsLoadedMsg{err: fmt.Errorf("SSO not authenticated")}
		}
		ctx := context.Background()
		accounts, err := m.ssoAuthenticator.ListAccounts(ctx)
		return ssoAccountsLoadedMsg{accounts: accounts, err: err}
	}
}

func (m model) switchToSSOAccount(account aws.SSOAccount, region string) tea.Cmd {
	return func() tea.Msg {
		if m.ssoAuthenticator == nil {
			return accountSwitchedMsg{err: fmt.Errorf("SSO not authenticated")}
		}
		ctx := context.Background()

		// Get credentials for the account/role
		creds, err := m.ssoAuthenticator.GetCredentials(ctx, account.AccountID, account.RoleName)
		if err != nil {
			return accountSwitchedMsg{err: fmt.Errorf("failed to get credentials: %w", err)}
		}

		// Create new AWS client with SSO credentials
		client, err := aws.NewClientWithSSOCredentials(ctx, creds, region, account.AccountName)
		if err != nil {
			return accountSwitchedMsg{err: fmt.Errorf("failed to create client: %w", err)}
		}

		return accountSwitchedMsg{
			client:      client,
			accountID:   account.AccountID,
			accountName: account.AccountName,
			credentials: creds,
		}
	}
}

func (m model) loadEC2Instances() tea.Msg {
	ctx := context.Background()
	instances, err := m.awsClient.ListInstances(ctx)
	return instancesLoadedMsg{instances: instances, err: err}
}

func (m model) loadS3Buckets() tea.Msg {
	ctx := context.Background()
	buckets, err := m.awsClient.ListBuckets(ctx)
	return bucketsLoadedMsg{buckets: buckets, err: err}
}

func (m model) loadEKSClusters() tea.Msg {
	ctx := context.Background()
	clusters, err := m.awsClient.ListEKSClusters(ctx)
	return eksClustersLoadedMsg{clusters: clusters, err: err}
}

func (m model) loadECSClusters() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	clusters, err := m.awsClient.ListECSClusters(ctx)
	return ecsClustersLoadedMsg{clusters: clusters, err: err}
}

func (m model) loadECSServices(clusterName string) tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	services, err := m.awsClient.ListECSServices(ctx, clusterName)
	return ecsServicesLoadedMsg{cluster: clusterName, services: services, err: err}
}

func (m model) loadECSTasks(clusterName, serviceName string) tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tasks, err := m.awsClient.ListECSTasks(ctx, clusterName, serviceName)
	return ecsTasksLoadedMsg{cluster: clusterName, service: serviceName, tasks: tasks, err: err}
}

func (m model) loadECSTaskLogs(clusterName, taskArn string) tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	logs, err := m.awsClient.GetECSTaskLogs(ctx, clusterName, taskArn, 50)
	return ecsTaskLogsLoadedMsg{taskArn: taskArn, logs: logs, err: err}
}

func (m model) loadECRRepositories() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	repos, err := m.awsClient.ListECRRepositoriesDetailed(ctx)
	return ecrReposLoadedMsg{repos: repos, err: err}
}

func (m model) loadECRImages(repoName string) tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	images, err := m.awsClient.ListECRImages(ctx, repoName)
	return ecrImagesLoadedMsg{repo: repoName, images: images, err: err}
}

func (m model) loadECRScan(repoName, digest string) tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	scan, err := m.awsClient.GetECRImageScan(ctx, repoName, digest)
	return ecrScanLoadedMsg{repo: repoName, digest: digest, scan: scan, err: err}
}

func (m model) loadSecrets() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	secrets, err := m.awsClient.ListSecrets(ctx)
	return secretsLoadedMsg{secrets: secrets, err: err}
}

func (m model) loadSecretDetails(name string) tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	details, err := m.awsClient.GetSecretDetails(ctx, name)
	return secretDetailsLoadedMsg{details: details, err: err}
}

func (m model) loadEKSClusterDetails(clusterName string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Load cluster details, node groups, and addons in parallel
		detailsChan := make(chan *aws.EKSClusterDetails)
		nodeGroupsChan := make(chan []aws.EKSNodeGroup)
		addonsChan := make(chan []aws.EKSAddon)
		errChan := make(chan error, 3)

		// Load cluster details
		go func() {
			details, err := m.awsClient.GetEKSClusterDetails(ctx, clusterName)
			if err != nil {
				errChan <- err
				detailsChan <- nil
				return
			}
			detailsChan <- details
			errChan <- nil
		}()

		// Load node groups
		go func() {
			nodeGroups, err := m.awsClient.ListNodeGroups(ctx, clusterName)
			if err != nil {
				errChan <- err
				nodeGroupsChan <- nil
				return
			}
			nodeGroupsChan <- nodeGroups
			errChan <- nil
		}()

		// Load addons
		go func() {
			addons, err := m.awsClient.ListAddons(ctx, clusterName)
			if err != nil {
				errChan <- err
				addonsChan <- nil
				return
			}
			addonsChan <- addons
			errChan <- nil
		}()

		// Wait for all to complete
		details := <-detailsChan
		nodeGroups := <-nodeGroupsChan
		addons := <-addonsChan

		// Check for errors
		var firstErr error
		for i := 0; i < 3; i++ {
			if err := <-errChan; err != nil && firstErr == nil {
				firstErr = err
			}
		}

		return eksClusterDetailsLoadedMsg{
			details:    details,
			nodeGroups: nodeGroups,
			addons:     addons,
			err:        firstErr,
		}
	}
}

func (m model) updateKubeconfig(clusterName string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		err := m.awsClient.UpdateKubeconfig(ctx, clusterName)
		return kubeconfigUpdatedMsg{clusterName: clusterName, err: err}
	}
}

func (m model) loadS3Objects(bucket, prefix string, continuationToken *string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		result, err := m.awsClient.ListObjects(ctx, bucket, prefix, continuationToken)
		return objectsLoadedMsg{result: result, err: err}
	}
}

func (m model) loadS3ObjectDetails(bucket, key string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		details, err := m.awsClient.GetObjectDetails(ctx, bucket, key)
		return objectDetailsLoadedMsg{details: details, err: err}
	}
}

func (m model) downloadS3Object(bucket, key, localPath string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		err := m.awsClient.DownloadObject(ctx, bucket, key, localPath)
		return fileOperationCompletedMsg{operation: "download", err: err}
	}
}

func (m model) uploadS3Object(bucket, key, localPath string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		err := m.awsClient.UploadObject(ctx, bucket, key, localPath)
		return fileOperationCompletedMsg{operation: "upload", err: err}
	}
}

func (m model) deleteS3Object(bucket, key string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		err := m.awsClient.DeleteObject(ctx, bucket, key)
		return s3ActionCompletedMsg{action: "delete object", err: err}
	}
}

func (m model) deleteS3Bucket(bucket string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		err := m.awsClient.DeleteBucket(ctx, bucket)
		return s3ActionCompletedMsg{action: "delete bucket", err: err}
	}
}

func (m model) createS3Bucket(bucket, region string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		err := m.awsClient.CreateBucket(ctx, bucket, region)
		return s3ActionCompletedMsg{action: "create bucket", err: err}
	}
}

func (m model) copyS3Object(sourceBucket, sourceKey, destBucket, destKey string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		err := m.awsClient.CopyObject(ctx, sourceBucket, sourceKey, destBucket, destKey)
		return s3ActionCompletedMsg{action: "copy object", err: err}
	}
}

func (m model) generatePresignedURL(bucket, key string, expiration int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		url, err := m.awsClient.GeneratePresignedURL(ctx, bucket, key, expiration)
		return presignedURLGeneratedMsg{url: url, err: err}
	}
}

func (m model) loadBucketPolicy(bucket string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		policy, err := m.awsClient.GetBucketPolicy(ctx, bucket)
		return bucketPolicyLoadedMsg{policy: policy, err: err}
	}
}

func (m model) loadBucketVersioning(bucket string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		versioning, err := m.awsClient.GetBucketVersioning(ctx, bucket)
		return bucketVersioningLoadedMsg{versioning: versioning, err: err}
	}
}

func (m model) loadEC2InstanceDetails(instanceID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		details, err := m.awsClient.GetInstanceDetails(ctx, instanceID)
		return instanceDetailsLoadedMsg{details: details, err: err}
	}
}

func (m model) loadInstanceStatus(instanceID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		status, err := m.awsClient.GetInstanceStatus(ctx, instanceID)
		return instanceStatusLoadedMsg{status: status, err: err}
	}
}

func (m model) loadInstanceMetrics(instanceID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		metrics, err := m.awsClient.GetInstanceMetrics(ctx, instanceID)
		return instanceMetricsLoadedMsg{metrics: metrics, err: err}
	}
}

func (m model) loadSSMStatus(instanceID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		status, err := m.awsClient.CheckSSMConnectivity(ctx, instanceID)
		return ssmStatusLoadedMsg{status: status, err: err}
	}
}

func (m model) performInstanceAction(action string, instanceID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var err error

		switch action {
		case "start":
			err = m.awsClient.StartInstance(ctx, instanceID)
		case "stop":
			err = m.awsClient.StopInstance(ctx, instanceID)
		case "reboot":
			err = m.awsClient.RebootInstance(ctx, instanceID)
		case "terminate":
			err = m.awsClient.TerminateInstance(ctx, instanceID)
		}

		return instanceActionCompletedMsg{action: action, err: err}
	}
}

func (m model) performBulkAction(action string, instanceIDs []string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		successCount := 0
		failureCount := 0

		for _, instanceID := range instanceIDs {
			var err error
			switch action {
			case "start":
				err = m.awsClient.StartInstance(ctx, instanceID)
			case "stop":
				err = m.awsClient.StopInstance(ctx, instanceID)
			case "reboot":
				err = m.awsClient.RebootInstance(ctx, instanceID)
			case "terminate":
				err = m.awsClient.TerminateInstance(ctx, instanceID)
			}

			if err != nil {
				failureCount++
			} else {
				successCount++
			}
		}

		return bulkActionCompletedMsg{
			action:       action,
			successCount: successCount,
			failureCount: failureCount,
		}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle auth method selection screen
	if m.currentScreen == authMethodScreen {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "up", "k":
				if m.selectedAuthMethod > 0 {
					m.selectedAuthMethod--
				}
			case "down", "j":
				if m.selectedAuthMethod < maxAuthMethod {
					m.selectedAuthMethod++
				}
			case "enter":
				// Save selected auth method
				switch m.selectedAuthMethod {
				case 0: // Environment Variables
					authConfig := &aws.AuthConfig{
						Method: aws.AuthMethodEnv,
					}
					if err := aws.SaveAuthConfig(authConfig); err != nil {
						m.statusMessage = fmt.Sprintf("Failed to save config: %v", err)
						return m, nil
					}
					m.authConfig = authConfig
					m.currentScreen = ec2Screen
					m.loading = true
					return m, m.initAWSClient
				case 1: // AWS Profile
					m.currentScreen = authProfileScreen
					m.configuringProfile = true
					return m, nil
				case 2: // SSO
					m.currentScreen = ssoConfigScreen
					// Pre-fill defaults if not already set
					if m.ssoURLInput.Value() == "" && m.authConfig != nil {
						m.ssoURLInput.SetValue(m.authConfig.SSOStartURL)
					}
					if m.ssoRegionInput.Value() == "" {
						if m.authConfig != nil && m.authConfig.SSORegion != "" {
							m.ssoRegionInput.SetValue(m.authConfig.SSORegion)
						} else {
							m.ssoRegionInput.SetValue(aws.DefaultSSORegion)
						}
					}
					m.configuringSSO = true
					return m, nil
				}
			case "esc":
				return m, tea.Quit
			}
		}
		return m, nil
	}

	// Handle AWS Profile configuration
	if m.configuringProfile && m.currentScreen == authProfileScreen {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				profileName := m.profileInput.Value()
				if profileName == "" {
					profileName = "default"
				}
				if err := aws.ValidateProfileName(profileName); err != nil {
					m.statusMessage = fmt.Sprintf("Invalid profile name: %v", err)
					return m, nil
				}
				// Save profile config
				authConfig := &aws.AuthConfig{
					Method:      aws.AuthMethodProfile,
					ProfileName: profileName,
				}
				if err := aws.SaveAuthConfig(authConfig); err != nil {
					m.statusMessage = fmt.Sprintf("Failed to save config: %v", err)
					return m, nil
				}
				m.authConfig = authConfig
				m.configuringProfile = false
				m.currentScreen = ec2Screen
				m.loading = true
				m.statusMessage = fmt.Sprintf("Using AWS profile: %s", profileName)
				return m, m.initAWSClient
			case "esc":
				// Go back to auth method selection
				m.currentScreen = authMethodScreen
				m.configuringProfile = false
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.profileInput, cmd = m.profileInput.Update(msg)
		return m, cmd
	}

	// Handle SSO URL configuration
	if m.configuringSSO && m.currentScreen == ssoConfigScreen {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "tab":
				// Toggle focus between URL and region inputs
				if m.ssoURLInput.Focused() {
					m.ssoURLInput.Blur()
					m.ssoRegionInput.Focus()
				} else {
					m.ssoRegionInput.Blur()
					m.ssoURLInput.Focus()
				}
				return m, nil
			case "enter":
				ssoURL := strings.TrimSpace(m.ssoURLInput.Value())
				if err := aws.ValidateSSOStartURL(ssoURL); err != nil {
					m.statusMessage = fmt.Sprintf("Invalid SSO URL: %v", err)
					return m, nil
				}
				ssoRegion := strings.TrimSpace(m.ssoRegionInput.Value())
				if ssoRegion == "" {
					ssoRegion = aws.DefaultSSORegion
				}
				if err := aws.ValidateSSORegion(ssoRegion); err != nil {
					m.statusMessage = fmt.Sprintf("Invalid SSO region: %v", err)
					return m, nil
				}
				// Save SSO config
				authConfig := &aws.AuthConfig{
					Method:      aws.AuthMethodSSO,
					SSOStartURL: ssoURL,
					SSORegion:   ssoRegion,
				}
				if err := aws.SaveAuthConfig(authConfig); err != nil {
					m.statusMessage = fmt.Sprintf("Failed to save config: %v", err)
					return m, nil
				}
				m.authConfig = authConfig
				m.configuringSSO = false
				m.currentScreen = accountScreen
				m.loading = true
				m.statusMessage = "Starting SSO authentication - opening browser..."
				return m, m.authenticateSSO(authConfig.SSOStartURL, authConfig.SSORegion)
			case "esc":
				// Go back to auth method selection
				m.currentScreen = authMethodScreen
				m.configuringSSO = false
				return m, nil
			}
		}
		var cmds []tea.Cmd
		var cmd tea.Cmd
		m.ssoURLInput, cmd = m.ssoURLInput.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		var cmd2 tea.Cmd
		m.ssoRegionInput, cmd2 = m.ssoRegionInput.Update(msg)
		if cmd2 != nil {
			cmds = append(cmds, cmd2)
		}
		return m, tea.Batch(cmds...)
	}

	// Handle S3 delete confirmation dialog
	if m.s3.ConfirmDelete {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				// Check if typed value matches the item to delete
				if m.deleteConfirmInput.Value() == m.s3.DeleteKey {
					m.loading = true
					m.s3.ConfirmDelete = false
					m.deleteConfirmInput.SetValue("")
					if m.s3.DeleteTarget == "object" {
						return m, m.deleteS3Object(m.s3.CurrentBucket, m.s3.DeleteKey)
					} else if m.s3.DeleteTarget == "bucket" {
						return m, m.deleteS3Bucket(m.s3.DeleteKey)
					}
				} else {
					m.statusMessage = "Name doesn't match - delete cancelled"
					m.s3.ConfirmDelete = false
					m.s3.DeleteTarget = ""
					m.s3.DeleteKey = ""
					m.deleteConfirmInput.SetValue("")
					return m, nil
				}
			case "esc":
				m.s3.ConfirmDelete = false
				m.s3.DeleteTarget = ""
				m.s3.DeleteKey = ""
				m.deleteConfirmInput.SetValue("")
				m.statusMessage = "Delete cancelled"
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.deleteConfirmInput, cmd = m.deleteConfirmInput.Update(msg)
		return m, cmd
	}

	// Handle EC2 confirmation dialog
	if m.ec2.ShowingConfirmation {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "y", "Y":
				m.loading = true
				// Check if this is a bulk action
				if len(m.ec2.SelectedInstances) > 0 && m.currentScreen == ec2Screen {
					var instanceIDs []string
					for id := range m.ec2.SelectedInstances {
						instanceIDs = append(instanceIDs, id)
					}
					return m, m.performBulkAction(m.ec2.ConfirmAction, instanceIDs)
				}
				return m, m.performInstanceAction(m.ec2.ConfirmAction, m.ec2.ConfirmInstanceID)
			case "n", "N", "esc":
				m.ec2.ShowingConfirmation = false
				m.ec2.ConfirmAction = ""
				m.ec2.ConfirmInstanceID = ""
				return m, nil
			}
		}
		return m, nil
	}

	// Handle VIM modes (search/command)
	if m.vimState.Mode == vim.SearchMode || m.vimState.Mode == vim.CommandMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			// Handle tab completion in command mode
			if m.vimState.Mode == vim.CommandMode && msg.String() == "tab" {
				completed, isComplete := vim.CompleteCommand(m.vimState.CommandBuffer)
				m.vimState.CommandBuffer = completed
				if !isComplete {
					// Show suggestions
					m.commandSuggestions = vim.GetCommandSuggestions(completed)
				} else {
					m.commandSuggestions = nil
				}
				return m, nil
			}

			if m.vimState.HandleKey(msg) {
				// Update suggestions as user types in command mode
				if m.vimState.Mode == vim.CommandMode {
					m.commandSuggestions = vim.GetCommandSuggestions(m.vimState.CommandBuffer)
				}

				// Apply search on every keypress in search mode (instant search)
				if m.vimState.Mode == vim.SearchMode {
					if m.vimState.SearchQuery != "" {
						// Update the search query and apply immediately
						m.vimState.LastSearch = m.vimState.SearchQuery
						m.applyVimSearch()
					} else {
						// Empty search query - clear the search
						m.vimState.LastSearch = ""
						m.vimState.SearchResults = []int{}
						m.ec2.FilteredInstances = nil
						m.s3.FilteredBuckets = nil
						m.s3.FilteredObjects = nil
					}
				}

				// If search mode was just completed, apply the search
				if m.vimState.Mode == vim.NormalMode && m.vimState.LastSearch != "" {
					m.applyVimSearch()
				}
				// If command mode was just completed, execute the command
				if m.vimState.Mode == vim.NormalMode && m.vimState.CommandBuffer != "" {
					cmd := m.executeVimCommand(m.vimState.CommandBuffer)
					m.vimState.CommandBuffer = ""
					m.commandSuggestions = nil
					return m, cmd
				}
				return m, nil
			}
		}
		return m, nil
	}

	// Handle filtering (legacy filter mode)
	if m.filtering {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				m.filter = m.filterInput.Value()
				m.filtering = false
				return m, nil
			case "esc":
				m.filtering = false
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case *aws.Client:
		m.awsClient = msg
		// Clear stale data when switching accounts/regions
		m.ec2.Instances = nil
		m.s3.Buckets = nil
		m.eks.Clusters = nil
		m.clearSearch()
		// If we're already on a specific service screen, load that service; otherwise wait for user selection
		if m.currentScreen == ec2Screen {
			if m.autoRefresh {
				return m, tea.Batch(m.loadEC2Instances, tickCmd())
			}
			return m, m.loadEC2Instances
		}
		if m.currentScreen == s3Screen {
			return m, m.loadS3Buckets
		}
		if m.currentScreen == eksScreen {
			return m, m.loadEKSClusters
		}
		m.statusMessage = "Authenticated - choose a service"
		return m, nil

	case tickMsg:
		// Auto-refresh EC2 instances if enabled and on EC2 screen
		if m.autoRefresh && m.currentScreen == ec2Screen {
			return m, tea.Batch(m.loadEC2Instances, tickCmd())
		}
		return m, tickCmd()

	case bulkActionCompletedMsg:
		m.loading = false
		m.ec2.ShowingConfirmation = false
		if msg.failureCount > 0 {
			m.statusMessage = fmt.Sprintf("Bulk %s: %d succeeded, %d failed", msg.action, msg.successCount, msg.failureCount)
		} else {
			m.statusMessage = fmt.Sprintf("Bulk %s: %d instances succeeded", msg.action, msg.successCount)
		}
		// Clear selections
		m.ec2.SelectedInstances = make(map[string]bool)
		// Refresh instances list
		return m, m.loadEC2Instances

	case instancesLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.ec2.Instances = msg.instances
			m.ec2.SelectedIndex = 0 // Reset selection
		}
		return m, nil

	case instanceDetailsLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.ec2.InstanceDetails = msg.details
			m.currentScreen = ec2DetailsScreen
			// Load additional information for details view
			instanceID := msg.details.ID
			return m, tea.Batch(
				m.loadInstanceStatus(instanceID),
				m.loadInstanceMetrics(instanceID),
				m.loadSSMStatus(instanceID),
			)
		}
		return m, nil

	case instanceStatusLoadedMsg:
		if msg.err == nil {
			m.ec2.InstanceStatus = msg.status
		}
		return m, nil

	case instanceMetricsLoadedMsg:
		if msg.err == nil {
			m.ec2.InstanceMetrics = msg.metrics
		}
		return m, nil

	case ssmStatusLoadedMsg:
		if msg.err == nil {
			m.ec2.SSMStatus = msg.status
		}
		return m, nil

	case instanceActionCompletedMsg:
		m.loading = false
		m.ec2.ShowingConfirmation = false
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.statusMessage = fmt.Sprintf("Successfully %sed instance", msg.action)
		}
		// Refresh instances list
		return m, m.loadEC2Instances

	case bucketsLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.s3.Buckets = msg.buckets
			m.s3.SelectedBucketIndex = 0
		}
		return m, nil

	case eksClustersLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.eks.Clusters = msg.clusters
			m.eks.SelectedIndex = 0
		}
		return m, nil

	case ecsClustersLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.ecs.Clusters = msg.clusters
			m.ecs.SelectedIndex = 0
			// Clear downstream state
			m.ecs.CurrentCluster = ""
			m.ecs.Services = nil
			m.ecs.FilteredSvcs = nil
			m.ecs.Tasks = nil
			m.ecs.FilteredTasks = nil
			m.ecs.TaskLogs = nil
			m.statusMessage = fmt.Sprintf("Loaded %d ECS clusters", len(msg.clusters))
		}
		return m, nil

	case ecsServicesLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.ecs.CurrentCluster = msg.cluster
			m.ecs.Services = msg.services
			m.ecs.FilteredSvcs = nil
			m.ecs.SelectedSvc = 0
			// Reset task state when cluster changes
			m.ecs.CurrentService = ""
			m.ecs.Tasks = nil
			m.ecs.FilteredTasks = nil
			m.ecs.TaskLogs = nil
			m.navigateToScreen(ecsServicesScreen)
			m.viewportOffset = 0
			m.statusMessage = fmt.Sprintf("Loaded %d ECS services", len(msg.services))
		}
		return m, nil

	case ecsTasksLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.ecs.CurrentCluster = msg.cluster
			m.ecs.CurrentService = msg.service
			m.ecs.Tasks = msg.tasks
			m.ecs.FilteredTasks = nil
			m.ecs.SelectedTask = 0
			m.ecs.TaskLogs = nil
			m.navigateToScreen(ecsTasksScreen)
			m.viewportOffset = 0
			m.statusMessage = fmt.Sprintf("Loaded %d ECS tasks", len(msg.tasks))
		}
		return m, nil

	case ecsTaskLogsLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.ecs.TaskLogs = msg.logs
			m.statusMessage = fmt.Sprintf("Loaded logs for task %s", msg.taskArn)
		}
		return m, nil

	case ecrReposLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.ecr.Repositories = msg.repos
			m.ecr.FilteredRepos = nil
			m.ecr.SelectedRepoIndex = 0
			m.ecr.CurrentRepo = ""
			m.ecr.Images = nil
			m.ecr.FilteredImages = nil
			m.ecr.ImageScanResult = nil
			m.statusMessage = fmt.Sprintf("Loaded %d ECR repositories", len(msg.repos))
		}
		return m, nil

	case ecrImagesLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.ecr.CurrentRepo = msg.repo
			m.ecr.Images = msg.images
			m.ecr.FilteredImages = nil
			m.ecr.SelectedImage = 0
			m.ecr.ImageScanResult = nil
			m.currentScreen = ecrImagesScreen
			m.viewportOffset = 0
			m.statusMessage = fmt.Sprintf("Loaded %d images", len(msg.images))
		}
		return m, nil

	case ecrScanLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.ecr.ImageScanResult = msg.scan
			m.statusMessage = fmt.Sprintf("Loaded scan for %s", msg.digest)
		}
		return m, nil

	case secretsLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.secrets = msg.secrets
			m.secretsFiltered = nil
			m.selectedSecretIndex = 0
			m.currentSecretDetails = nil
			m.statusMessage = fmt.Sprintf("Loaded %d secrets", len(msg.secrets))
		}
		return m, nil

	case secretDetailsLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.currentSecretDetails = msg.details
			m.navigateToScreen(secretsDetailsScreen)
			m.viewportOffset = 0
			m.statusMessage = fmt.Sprintf("Loaded details for %s", msg.details.Name)
		}
		return m, nil

	case eksClusterDetailsLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.eks.Details = msg.details
			m.eks.NodeGroups = msg.nodeGroups
			m.eks.Addons = msg.addons
			m.currentScreen = eksDetailsScreen
			m.viewportOffset = 0
		}
		return m, nil

	case kubeconfigUpdatedMsg:
		m.loading = false
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error updating kubeconfig: %v", msg.err)
		} else {
			m.statusMessage = fmt.Sprintf("Updated kubeconfig for cluster: %s", msg.clusterName)
		}
		return m, nil

	case ssoAuthCompletedMsg:
		m.loading = false
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("SSO authentication failed: %v", msg.err)
			m.err = msg.err
			return m, nil
		}
		m.ssoAuthenticator = msg.authenticator
		m.statusMessage = "SSO authentication successful - loading accounts..."
		// Load available accounts
		return m, m.loadSSOAccounts()

	case ssoAccountsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Failed to load accounts: %v", msg.err)
			m.err = msg.err
			return m, nil
		}
		m.ssoAccounts = msg.accounts
		m.ssoSelectedIndex = 0
		m.currentScreen = accountScreen
		m.statusMessage = fmt.Sprintf("Loaded %d accounts - select one to continue", len(msg.accounts))

		// If AWS_ACCOUNT env var is set, auto-select matching account by name or ID once
		if m.envAccountTarget != "" && !m.envAccountAutoSwitchDone {
			target := strings.ToLower(m.envAccountTarget)
			matchIdx := -1
			for i, acc := range m.ssoAccounts {
				if strings.ToLower(acc.AccountName) == target || strings.ToLower(acc.AccountID) == target {
					matchIdx = i
					break
				}
			}
			m.envAccountAutoSwitchDone = true
			if matchIdx >= 0 {
				m.ssoSelectedIndex = matchIdx
				m.statusMessage = fmt.Sprintf("Auto-selecting account via AWS_ACCOUNT=%s", m.envAccountTarget)
				m.loading = true
				return m, m.switchToSSOAccount(m.ssoAccounts[matchIdx], m.config.Region)
			}
			m.statusMessage = fmt.Sprintf("AWS_ACCOUNT=%s not found; choose manually", m.envAccountTarget)
		}

		return m, nil

	case accountSwitchedMsg:
		m.loading = false
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Failed to switch account: %v", msg.err)
			m.err = msg.err
			return m, nil
		}
		// Update client and account info
		m.awsClient = msg.client
		m.currentAccountID = msg.accountID
		m.currentAccountName = msg.accountName
		m.ssoCredentials = msg.credentials // Store SSO credentials for CLI commands
		// Clear stale data when switching accounts
		m.ec2.Instances = nil
		m.s3.Buckets = nil
		m.eks.Clusters = nil
		m.clearSearch()
		// Clear any previous errors
		m.err = nil
		// Switch to service selection and wait for user choice
		m.navigateToScreen(serviceScreen)
		m.viewportOffset = 0
		m.loading = false
		m.statusMessage = fmt.Sprintf("Switched to account: %s - choose a service", msg.accountName)
		return m, nil

	case objectsLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.s3.Objects = msg.result.Objects
			m.s3.NextContinuationToken = msg.result.NextContinuationToken
			m.s3.IsTruncated = msg.result.IsTruncated
			m.s3.SelectedObjectIndex = 0
			m.currentScreen = s3BrowseScreen
		}
		return m, nil

	case objectDetailsLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.s3.ObjectDetails = msg.details
			m.currentScreen = s3ObjectDetailsScreen
		}
		return m, nil

	case fileOperationCompletedMsg:
		m.loading = false
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error %sing: %v", msg.operation, msg.err)
		} else {
			m.statusMessage = fmt.Sprintf("Successfully %sed file", msg.operation)
		}
		return m, nil

	case s3ActionCompletedMsg:
		m.loading = false
		m.s3.ConfirmDelete = false
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.statusMessage = fmt.Sprintf("Successfully completed: %s", msg.action)
		}
		// Refresh appropriate view
		if m.s3.DeleteTarget == "bucket" {
			return m, m.loadS3Buckets
		} else if m.s3.DeleteTarget == "object" {
			return m, m.loadS3Objects(m.s3.CurrentBucket, m.s3.CurrentPrefix, nil)
		}
		return m, nil

	case presignedURLGeneratedMsg:
		m.loading = false
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error generating URL: %v", msg.err)
		} else {
			m.s3.PresignedURL = msg.url
			// Copy to clipboard on macOS
			if runtime.GOOS == "darwin" {
				cmd := exec.Command("pbcopy")
				cmd.Stdin = strings.NewReader(msg.url)
				if err := cmd.Run(); err == nil {
					m.statusMessage = "Presigned URL copied to clipboard"
				} else {
					m.statusMessage = "Presigned URL generated (displayed below)"
				}
			} else {
				m.statusMessage = "Presigned URL generated (displayed below)"
			}
		}
		return m, nil

	case bucketPolicyLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.s3.BucketPolicy = fmt.Sprintf("Error: %v", msg.err)
		} else if msg.policy == "" {
			m.s3.BucketPolicy = "No bucket policy set"
		} else {
			m.s3.BucketPolicy = msg.policy
		}
		m.s3.ShowingInfo = true
		m.s3.InfoType = "policy"
		return m, nil

	case bucketVersioningLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.s3.BucketVersioning = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.s3.BucketVersioning = msg.versioning
		}
		m.s3.ShowingInfo = true
		m.s3.InfoType = "versioning"
		return m, nil

	case launchSSMSessionMsg:
		// Store the SSM session info in the model so we can access it after quit
		m.ssmInstanceID = msg.instanceID
		m.ssmRegion = msg.region
		m.statusMessage = fmt.Sprintf("Launching SSM session for %s...", msg.instanceID)
		// Quit the program to launch SSM in current terminal
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			// Don't quit if we're in help screen, go back instead
			if m.currentScreen == helpScreen {
				m.currentScreen = m.previousScreen
				m.viewportOffset = 0
				return m, nil
			} else if m.currentScreen == ec2DetailsScreen {
				m.currentScreen = ec2Screen
				m.ec2.InstanceDetails = nil
				m.viewportOffset = 0
				return m, nil
			} else if m.currentScreen == s3BrowseScreen {
				m.currentScreen = s3Screen
				m.s3.Objects = nil
				m.s3.CurrentBucket = ""
				m.s3.CurrentPrefix = ""
				m.viewportOffset = 0
				return m, nil
			} else if m.currentScreen == s3ObjectDetailsScreen {
				m.currentScreen = s3BrowseScreen
				m.s3.ObjectDetails = nil
				m.viewportOffset = 0
				return m, nil
			}
			return m, tea.Quit
		case "esc":
			// ESC key to dismiss help, S3 info popup, clear presigned URL, clear search, or go back from details view
			if m.currentScreen == helpScreen {
				// Close help modal and return to previous screen
				m.currentScreen = m.previousScreen
				m.viewportOffset = 0
				return m, nil
			}
			if m.currentScreen == regionScreen {
				// Go back to previous screen without changing region
				m.currentScreen = m.previousScreen
				m.viewportOffset = 0
				m.statusMessage = "Region selection cancelled"
				return m, nil
			}
			if m.s3.ShowingInfo {
				m.s3.ShowingInfo = false
				m.s3.InfoType = ""
				m.s3.BucketPolicy = ""
				m.s3.BucketVersioning = ""
				m.s3.PresignedURL = ""
				return m, nil
			}
			// Clear presigned URL if showing
			if m.s3.PresignedURL != "" {
				m.s3.PresignedURL = ""
				m.statusMessage = "Presigned URL cleared"
				return m, nil
			}
			// Clear active search filter
			if m.vimState.LastSearch != "" {
				m.vimState.LastSearch = ""
				m.vimState.SearchResults = []int{}
				m.ec2.FilteredInstances = nil
				m.s3.FilteredBuckets = nil
				m.s3.FilteredObjects = nil
				m.eks.Filtered = nil
				m.ecs.Filtered = nil
				m.ecs.FilteredSvcs = nil
				m.ecs.FilteredTasks = nil
				m.statusMessage = "Search cleared"
				return m, nil
			}
			if m.currentScreen == ec2DetailsScreen {
				m.currentScreen = ec2Screen
				m.ec2.InstanceDetails = nil
				return m, nil
			} else if m.currentScreen == ecsTaskDetailsScreen {
				m.currentScreen = ecsTasksScreen
				return m, nil
			} else if m.currentScreen == s3BrowseScreen {
				// If we're in a subfolder, go to parent folder
				// Otherwise go back to bucket list
				if m.s3.CurrentPrefix != "" {
					// Calculate parent prefix
					// Remove trailing slash if present
					prefix := strings.TrimSuffix(m.s3.CurrentPrefix, "/")
					// Find last slash to get parent
					lastSlash := strings.LastIndex(prefix, "/")
					if lastSlash >= 0 {
						// Go to parent folder
						m.s3.CurrentPrefix = prefix[:lastSlash+1]
					} else {
						// We're at root level, go to root
						m.s3.CurrentPrefix = ""
					}
					m.loading = true
					m.viewportOffset = 0
					return m, m.loadS3Objects(m.s3.CurrentBucket, m.s3.CurrentPrefix, nil)
				} else {
					// We're at bucket root, go back to bucket list
					m.currentScreen = s3Screen
					m.s3.Objects = nil
					m.s3.CurrentBucket = ""
					m.s3.CurrentPrefix = ""
					return m, nil
				}
			} else if m.currentScreen == s3ObjectDetailsScreen {
				m.currentScreen = s3BrowseScreen
				m.s3.ObjectDetails = nil
				return m, nil
			} else if m.currentScreen == eksDetailsScreen {
				m.currentScreen = eksScreen
				m.eks.Details = nil
				m.eks.NodeGroups = nil
				m.eks.Addons = nil
				return m, nil
			} else if m.currentScreen == ecsTasksScreen {
				// Back to services list
				m.currentScreen = ecsServicesScreen
				m.ecs.SelectedTask = 0
				m.ecs.TaskLogs = nil
				return m, nil
			} else if m.currentScreen == ecsServicesScreen {
				// Back to clusters list
				m.currentScreen = ecsScreen
				m.ecs.SelectedSvc = 0
				return m, nil
			} else if m.currentScreen == ecrImagesScreen {
				m.currentScreen = ecrScreen
				m.ecr.Images = nil
				m.ecr.FilteredImages = nil
				m.ecr.SelectedImage = 0
				m.ecr.ImageScanResult = nil
				return m, nil
			} else if m.currentScreen == ecrImageDetailsScreen {
				m.currentScreen = ecrImagesScreen
				return m, nil
			} else if m.currentScreen == secretsDetailsScreen {
				m.currentScreen = secretsScreen
				m.currentSecretDetails = nil
				return m, nil
			} else if m.currentScreen == ecsScreen || m.currentScreen == ec2Screen || m.currentScreen == s3Screen || m.currentScreen == eksScreen || m.currentScreen == ecrScreen {
				// Return to service selector from any main service screen
				m.goToServiceSelection()
				return m, nil
			}
		case "ctrl+o":
			// Back in history
			if m.goBack() {
				return m, nil
			}
		case "ctrl+i":
			// Forward in history
			if m.goForward() {
				return m, nil
			}
		case "k", "up", "j", "down", "g", "G", "ctrl+g", "ctrl+u", "ctrl+d", "ctrl+b", "ctrl+f", "pgup", "pgdown":
			// VIM-style navigation
			action := vim.ParseNavigation(msg)
			m.handleVimNavigation(action)
		case "/":
			// Enter VIM search mode
			m.vimState.EnterSearchMode()
			return m, nil
		case "n":
			// Next search result (VIM-style)
			if m.vimState.LastSearch != "" {
				if idx := m.vimState.NextMatch(); idx >= 0 {
					m.setSelectedIndex(idx)
				}
			} else if m.currentScreen == s3BrowseScreen && m.s3.IsTruncated && m.s3.NextContinuationToken != nil {
				// Load next page in S3 browser (if no active search)
				m.loading = true
				return m, m.loadS3Objects(m.s3.CurrentBucket, m.s3.CurrentPrefix, m.s3.NextContinuationToken)
			}
		case "N":
			// Previous search result
			if m.vimState.LastSearch != "" {
				if idx := m.vimState.PrevMatch(); idx >= 0 {
					m.setSelectedIndex(idx)
				}
			}
		case ":":
			// Enter VIM command mode
			m.vimState.EnterCommandMode()
			return m, nil
		case "enter", "i":
			// Enter key to view instance details or browse S3 bucket, or view object details
			if m.currentScreen == serviceScreen {
				switch getServices()[m.serviceSelectedIndex] {
				case "EC2":
					m.navigateToScreen(ec2Screen)
					m.viewportOffset = 0
					m.statusMessage = "Switched to EC2"
					if len(m.ec2.Instances) == 0 && m.awsClient != nil {
						m.loading = true
						return m, m.loadEC2Instances
					}
					return m, nil
				case "S3":
					m.navigateToScreen(s3Screen)
					m.viewportOffset = 0
					m.statusMessage = "Switched to S3"
					if len(m.s3.Buckets) == 0 && m.awsClient != nil {
						m.loading = true
						return m, m.loadS3Buckets
					}
					return m, nil
				case "EKS":
					m.navigateToScreen(eksScreen)
					m.viewportOffset = 0
					m.statusMessage = "Switched to EKS"
					if len(m.eks.Clusters) == 0 && m.awsClient != nil {
						m.loading = true
						return m, m.loadEKSClusters
					}
					return m, nil
				case "ECS":
					m.navigateToScreen(ecsScreen)
					m.viewportOffset = 0
					m.statusMessage = "Switched to ECS"
					if len(m.ecs.Clusters) == 0 && m.awsClient != nil {
						m.loading = true
						return m, m.loadECSClusters
					}
					return m, nil
				case "ECR":
					m.navigateToScreen(ecrScreen)
					m.viewportOffset = 0
					m.statusMessage = "Switched to ECR"
					if len(m.ecr.Repositories) == 0 && m.awsClient != nil {
						m.loading = true
						return m, m.loadECRRepositories
					}
					return m, nil
				case "Secrets":
					m.navigateToScreen(secretsScreen)
					m.viewportOffset = 0
					m.statusMessage = "Switched to Secrets Manager"
					if len(m.secrets) == 0 && m.awsClient != nil {
						m.loading = true
						return m, m.loadSecrets
					}
					return m, nil
				default:
					return m, nil
				}
			} else if m.currentScreen == regionScreen {
				// Select region and switch back to previous screen
				if m.config != nil && m.regionSelectedIndex < len(m.config.Regions) {
					selectedRegion := m.config.Regions[m.regionSelectedIndex]
					m.config.Region = selectedRegion
					m.currentScreen = m.previousScreen
					m.viewportOffset = 0
					m.loading = true
					m.statusMessage = fmt.Sprintf("Switching to region: %s", selectedRegion)
					return m, m.initAWSClient
				}
			} else if m.currentScreen == ec2Screen {
				// Use filtered list if active
				instances := m.ec2.Instances
				if len(m.ec2.FilteredInstances) > 0 {
					instances = m.ec2.FilteredInstances
				}
				if len(instances) > 0 && m.ec2.SelectedIndex < len(instances) {
					selectedInstance := instances[m.ec2.SelectedIndex]
					m.loading = true
					m.viewportOffset = 0
					return m, m.loadEC2InstanceDetails(selectedInstance.ID)
				}
			} else if m.currentScreen == s3Screen {
				// Browse bucket contents - use filtered list if active
				buckets := m.s3.Buckets
				if len(m.s3.FilteredBuckets) > 0 {
					buckets = m.s3.FilteredBuckets
				}
				if len(buckets) > 0 && m.s3.SelectedBucketIndex < len(buckets) {
					selectedBucket := buckets[m.s3.SelectedBucketIndex]
					m.s3.CurrentBucket = selectedBucket.Name
					m.s3.CurrentPrefix = ""
					// Clear search when entering a bucket
					m.vimState.LastSearch = ""
					m.vimState.SearchResults = []int{}
					m.s3.FilteredBuckets = nil
					m.loading = true
					m.viewportOffset = 0
					return m, m.loadS3Objects(m.s3.CurrentBucket, m.s3.CurrentPrefix, nil)
				}
			} else if m.currentScreen == s3BrowseScreen {
				// Use filtered list if active
				objects := m.s3.Objects
				if len(m.s3.FilteredObjects) > 0 {
					objects = m.s3.FilteredObjects
				}
				if len(objects) > 0 && m.s3.SelectedObjectIndex < len(objects) {
					selectedObject := objects[m.s3.SelectedObjectIndex]
					if selectedObject.IsFolder {
						// Navigate into folder
						m.s3.CurrentPrefix = selectedObject.Key
						// Clear search when entering a folder
						m.vimState.LastSearch = ""
						m.vimState.SearchResults = []int{}
						m.s3.FilteredObjects = nil
						m.loading = true
						m.viewportOffset = 0
						return m, m.loadS3Objects(m.s3.CurrentBucket, m.s3.CurrentPrefix, nil)
					} else {
						// View file details
						m.loading = true
						m.viewportOffset = 0
						return m, m.loadS3ObjectDetails(m.s3.CurrentBucket, selectedObject.Key)
					}
				}
			} else if m.currentScreen == eksScreen {
				// View EKS cluster details - use filtered list if active
				clusters := m.eks.Clusters
				if len(m.eks.Filtered) > 0 {
					clusters = m.eks.Filtered
				}
				if len(clusters) > 0 && m.eks.SelectedIndex < len(clusters) {
					selectedCluster := clusters[m.eks.SelectedIndex]
					m.loading = true
					m.viewportOffset = 0
					return m, m.loadEKSClusterDetails(selectedCluster.Name)
				}
			} else if m.currentScreen == ecsScreen {
				clusters := m.ecs.Clusters
				if len(m.ecs.Filtered) > 0 {
					clusters = m.ecs.Filtered
				}
				if len(clusters) > 0 && m.ecs.SelectedIndex < len(clusters) {
					selectedCluster := clusters[m.ecs.SelectedIndex]
					m.loading = true
					m.viewportOffset = 0
					return m, func() tea.Msg { return m.loadECSServices(selectedCluster.Name) }
				}
			} else if m.currentScreen == ecsServicesScreen {
				services := m.ecs.Services
				if len(m.ecs.FilteredSvcs) > 0 {
					services = m.ecs.FilteredSvcs
				}
				if len(services) > 0 && m.ecs.SelectedSvc < len(services) && m.ecs.CurrentCluster != "" {
					selectedService := services[m.ecs.SelectedSvc]
					m.loading = true
					m.viewportOffset = 0
					return m, func() tea.Msg { return m.loadECSTasks(m.ecs.CurrentCluster, selectedService.Name) }
				}
			} else if m.currentScreen == ecsTasksScreen {
				tasks := m.ecs.Tasks
				if len(m.ecs.FilteredTasks) > 0 {
					tasks = m.ecs.FilteredTasks
				}
				if len(tasks) > 0 && m.ecs.SelectedTask < len(tasks) && m.ecs.CurrentCluster != "" {
					selectedTask := tasks[m.ecs.SelectedTask]
					m.navigateToScreen(ecsTaskDetailsScreen)
					m.viewportOffset = 0
					m.loading = true
					return m, func() tea.Msg { return m.loadECSTaskLogs(m.ecs.CurrentCluster, selectedTask.Arn) }
				}
			} else if m.currentScreen == ecrScreen {
				repos := m.ecr.Repositories
				if len(m.ecr.FilteredRepos) > 0 {
					repos = m.ecr.FilteredRepos
				}
				if len(repos) > 0 && m.ecr.SelectedRepoIndex < len(repos) {
					selectedRepo := repos[m.ecr.SelectedRepoIndex]
					m.loading = true
					m.viewportOffset = 0
					return m, func() tea.Msg { return m.loadECRImages(selectedRepo.Name) }
				}
			} else if m.currentScreen == ecrImagesScreen {
				images := m.ecr.Images
				if len(m.ecr.FilteredImages) > 0 {
					images = m.ecr.FilteredImages
				}
				if len(images) > 0 && m.ecr.SelectedImage < len(images) {
					selected := images[m.ecr.SelectedImage]
					m.navigateToScreen(ecrImageDetailsScreen)
					m.viewportOffset = 0
					m.loading = true
					return m, func() tea.Msg { return m.loadECRScan(m.ecr.CurrentRepo, selected.Digest) }
				}
			} else if m.currentScreen == secretsScreen {
				list := m.secrets
				if len(m.secretsFiltered) > 0 {
					list = m.secretsFiltered
				}
				if len(list) > 0 && m.selectedSecretIndex < len(list) {
					sec := list[m.selectedSecretIndex]
					m.loading = true
					m.viewportOffset = 0
					return m, func() tea.Msg { return m.loadSecretDetails(sec.Name) }
				}
			} else if m.currentScreen == accountScreen {
				// Switch to selected AWS account
				accounts := m.ssoAccounts
				if len(m.ssoFilteredAccounts) > 0 {
					accounts = m.ssoFilteredAccounts
				}
				if len(accounts) > 0 && m.ssoSelectedIndex < len(accounts) {
					selectedAccount := accounts[m.ssoSelectedIndex]
					// Clear search when switching accounts
					m.clearSearch()
					m.loading = true
					m.viewportOffset = 0
					m.statusMessage = fmt.Sprintf("Switching to account: %s (%s)", selectedAccount.AccountName, selectedAccount.AccountID)
					return m, m.switchToSSOAccount(selectedAccount, m.config.Region)
				}
			}
		case "c":
			// Find the index of the current region
			currentIndex := -1
			for i, r := range m.config.Regions {
				if r == m.config.Region {
					currentIndex = i
					break
				}
			}
			// Cycle to the next region
			if currentIndex != -1 {
				nextIndex := (currentIndex + 1) % len(m.config.Regions)
				m.config.Region = m.config.Regions[nextIndex]
				m.loading = true
				return m, m.initAWSClient
			}

		case "tab":
			// Tab cycles through main screens (not details)
			m.clearSearch() // Clear search when switching screens
			if m.currentScreen == ec2Screen {
				m.currentScreen = s3Screen
			} else if m.currentScreen == s3Screen {
				m.currentScreen = eksScreen
				// Load EKS clusters when switching to EKS screen
				if len(m.eks.Clusters) == 0 {
					m.loading = true
					return m, m.loadEKSClusters
				}
			} else if m.currentScreen == eksScreen {
				m.currentScreen = ec2Screen
			}
		case "r":
			// Refresh current view
			if m.currentScreen == ec2Screen {
				m.loading = true
				return m, m.loadEC2Instances
			} else if m.currentScreen == s3Screen {
				m.loading = true
				return m, m.loadS3Buckets
			} else if m.currentScreen == s3BrowseScreen {
				m.loading = true
				return m, m.loadS3Objects(m.s3.CurrentBucket, m.s3.CurrentPrefix, nil)
			} else if m.currentScreen == eksScreen {
				m.loading = true
				return m, m.loadEKSClusters
			} else if m.currentScreen == ecsScreen {
				m.loading = true
				return m, m.loadECSClusters
			} else if m.currentScreen == ecsServicesScreen {
				target := m.ecs.CurrentCluster
				if target == "" && len(m.ecs.Clusters) > 0 && m.ecs.SelectedIndex < len(m.ecs.Clusters) {
					target = m.ecs.Clusters[m.ecs.SelectedIndex].Name
				}
				if target != "" {
					m.loading = true
					return m, func() tea.Msg { return m.loadECSServices(target) }
				}
			} else if m.currentScreen == ecsTasksScreen {
				cluster := m.ecs.CurrentCluster
				service := m.ecs.CurrentService
				if cluster != "" && service != "" {
					m.loading = true
					return m, func() tea.Msg { return m.loadECSTasks(cluster, service) }
				}
			} else if m.currentScreen == ecsTaskDetailsScreen {
				cluster := m.ecs.CurrentCluster
				tasks := m.ecs.Tasks
				if len(m.ecs.FilteredTasks) > 0 {
					tasks = m.ecs.FilteredTasks
				}
				if cluster != "" && len(tasks) > 0 && m.ecs.SelectedTask < len(tasks) {
					task := tasks[m.ecs.SelectedTask]
					m.loading = true
					return m, func() tea.Msg { return m.loadECSTaskLogs(cluster, task.Arn) }
				}
			} else if m.currentScreen == ecrScreen {
				m.loading = true
				return m, m.loadECRRepositories
			} else if m.currentScreen == ecrImagesScreen {
				if m.ecr.CurrentRepo != "" {
					m.loading = true
					return m, func() tea.Msg { return m.loadECRImages(m.ecr.CurrentRepo) }
				}
			} else if m.currentScreen == ecrImageDetailsScreen {
				if m.ecr.CurrentRepo != "" {
					images := m.ecr.Images
					if len(m.ecr.FilteredImages) > 0 {
						images = m.ecr.FilteredImages
					}
					if len(images) > 0 && m.ecr.SelectedImage < len(images) {
						img := images[m.ecr.SelectedImage]
						m.loading = true
						return m, func() tea.Msg { return m.loadECRScan(m.ecr.CurrentRepo, img.Digest) }
					}
				}
			} else if m.currentScreen == secretsScreen {
				m.loading = true
				return m, m.loadSecrets
			} else if m.currentScreen == secretsDetailsScreen {
				if m.currentSecretDetails != nil {
					m.loading = true
					name := m.currentSecretDetails.Name
					return m, func() tea.Msg { return m.loadSecretDetails(name) }
				}
			}
		case "K":
			// Update kubeconfig for selected EKS cluster
			if m.currentScreen == eksScreen {
				// Use filtered list if active
				clusters := m.eks.Clusters
				if len(m.eks.Filtered) > 0 {
					clusters = m.eks.Filtered
				}
				if len(clusters) > 0 && m.eks.SelectedIndex < len(clusters) {
					selectedCluster := clusters[m.eks.SelectedIndex]
					m.loading = true
					m.statusMessage = fmt.Sprintf("Updating kubeconfig for %s...", selectedCluster.Name)
					return m, m.updateKubeconfig(selectedCluster.Name)
				}
			} else if m.currentScreen == eksDetailsScreen && m.eks.Details != nil {
				// Update kubeconfig from details screen
				m.loading = true
				m.statusMessage = fmt.Sprintf("Updating kubeconfig for %s...", m.eks.Details.Name)
				return m, m.updateKubeconfig(m.eks.Details.Name)
			}
		case "backspace", "h":
			// Go up one level in S3 browser
			if m.currentScreen == s3BrowseScreen {
				if m.s3.CurrentPrefix == "" {
					// At root, go back to bucket list
					m.currentScreen = s3Screen
					m.s3.Objects = nil
					m.s3.CurrentBucket = ""
					m.viewportOffset = 0
					return m, nil
				}
				// Remove last directory from prefix
				parts := strings.Split(strings.TrimSuffix(m.s3.CurrentPrefix, "/"), "/")
				if len(parts) > 1 {
					m.s3.CurrentPrefix = strings.Join(parts[:len(parts)-1], "/") + "/"
				} else {
					m.s3.CurrentPrefix = ""
				}
				m.loading = true
				m.viewportOffset = 0
				return m, m.loadS3Objects(m.s3.CurrentBucket, m.s3.CurrentPrefix, nil)
			}

		case "e":
			// Edit selected S3 object in $EDITOR
			if m.currentScreen == s3BrowseScreen {
				// Use filtered list if active
				objects := m.s3.Objects
				if len(m.s3.FilteredObjects) > 0 {
					objects = m.s3.FilteredObjects
				}
				if len(objects) > 0 && m.s3.SelectedObjectIndex < len(objects) {
					selectedObject := objects[m.s3.SelectedObjectIndex]
					if !selectedObject.IsFolder {
						m.s3.EditBucket = m.s3.CurrentBucket
						m.s3.EditKey = selectedObject.Key
						m.statusMessage = fmt.Sprintf("Opening %s in editor...", selectedObject.Key)
						return m, tea.Quit
					}
				}
			} else if m.currentScreen == s3ObjectDetailsScreen && m.s3.ObjectDetails != nil {
				m.s3.EditBucket = m.s3.CurrentBucket
				m.s3.EditKey = m.s3.ObjectDetails.Key
				m.statusMessage = fmt.Sprintf("Opening %s in editor...", m.s3.ObjectDetails.Key)
				return m, tea.Quit
			}
		case "d":
			// Download selected S3 object
			if m.currentScreen == s3BrowseScreen {
				// Use filtered list if active
				objects := m.s3.Objects
				if len(m.s3.FilteredObjects) > 0 {
					objects = m.s3.FilteredObjects
				}
				if len(objects) > 0 && m.s3.SelectedObjectIndex < len(objects) {
					selectedObject := objects[m.s3.SelectedObjectIndex]
					if !selectedObject.IsFolder {
						// Extract just the filename from the key
						fileName := selectedObject.Key
						if strings.Contains(fileName, "/") {
							parts := strings.Split(fileName, "/")
							fileName = parts[len(parts)-1]
						}
						m.loading = true
						m.statusMessage = fmt.Sprintf("Downloading %s...", fileName)
						return m, m.downloadS3Object(m.s3.CurrentBucket, selectedObject.Key, fileName)
					}
				}
			} else if m.currentScreen == s3ObjectDetailsScreen && m.s3.ObjectDetails != nil {
				// Download from object details view
				fileName := m.s3.ObjectDetails.Key
				if strings.Contains(fileName, "/") {
					parts := strings.Split(fileName, "/")
					fileName = parts[len(parts)-1]
				}
				m.loading = true
				m.statusMessage = fmt.Sprintf("Downloading %s...", fileName)
				return m, m.downloadS3Object(m.s3.CurrentBucket, m.s3.ObjectDetails.Key, fileName)
			}
		case "u":
			// Upload file to S3 (prompt for file path)
			// For now, we'll just show a message that upload requires file path
			// In a full implementation, we'd add a text input for the file path
			if m.currentScreen == s3BrowseScreen {
				m.statusMessage = "Upload: Feature requires interactive file picker (coming soon)"
			}
		case "D":
			// Delete S3 object or bucket
			if m.currentScreen == s3BrowseScreen {
				// Use filtered list if active
				objects := m.s3.Objects
				if len(m.s3.FilteredObjects) > 0 {
					objects = m.s3.FilteredObjects
				}
				if len(objects) > 0 && m.s3.SelectedObjectIndex < len(objects) {
					selectedObject := objects[m.s3.SelectedObjectIndex]
					if !selectedObject.IsFolder {
						m.s3.ConfirmDelete = true
						m.s3.DeleteTarget = "object"
						m.s3.DeleteKey = selectedObject.Key
						m.deleteConfirmInput.SetValue("")
						m.deleteConfirmInput.Focus()
						m.statusMessage = fmt.Sprintf("Type the object name to confirm deletion: %s", selectedObject.Key)
						return m, nil
					}
				}
			} else if m.currentScreen == s3Screen {
				// Use filtered list if active
				buckets := m.s3.Buckets
				if len(m.s3.FilteredBuckets) > 0 {
					buckets = m.s3.FilteredBuckets
				}
				if len(buckets) > 0 && m.s3.SelectedBucketIndex < len(buckets) {
					selectedBucket := buckets[m.s3.SelectedBucketIndex]
					m.s3.ConfirmDelete = true
					m.s3.DeleteTarget = "bucket"
					m.s3.DeleteKey = selectedBucket.Name
					m.deleteConfirmInput.SetValue("")
					m.deleteConfirmInput.Focus()
					m.statusMessage = fmt.Sprintf("Type the bucket name to confirm deletion (bucket must be empty!): %s", selectedBucket.Name)
					return m, nil
				}
			}
		case "p":
			// Generate presigned URL or view bucket policy
			if m.currentScreen == s3BrowseScreen {
				// Use filtered list if active
				objects := m.s3.Objects
				if len(m.s3.FilteredObjects) > 0 {
					objects = m.s3.FilteredObjects
				}
				if len(objects) > 0 && m.s3.SelectedObjectIndex < len(objects) {
					selectedObject := objects[m.s3.SelectedObjectIndex]
					if !selectedObject.IsFolder {
						m.loading = true
						// Generate presigned URL with 1 hour expiration
						return m, m.generatePresignedURL(m.s3.CurrentBucket, selectedObject.Key, 3600)
					}
				}
			} else if m.currentScreen == s3Screen {
				// Use filtered list if active
				buckets := m.s3.Buckets
				if len(m.s3.FilteredBuckets) > 0 {
					buckets = m.s3.FilteredBuckets
				}
				if len(buckets) > 0 && m.s3.SelectedBucketIndex < len(buckets) {
					selectedBucket := buckets[m.s3.SelectedBucketIndex]
					m.loading = true
					return m, m.loadBucketPolicy(selectedBucket.Name)
				}
			} else if m.currentScreen == s3ObjectDetailsScreen && m.s3.ObjectDetails != nil {
				m.loading = true
				return m, m.generatePresignedURL(m.s3.CurrentBucket, m.s3.ObjectDetails.Key, 3600)
			}
		case "v":
			// View bucket versioning
			if m.currentScreen == s3Screen {
				// Use filtered list if active
				buckets := m.s3.Buckets
				if len(m.s3.FilteredBuckets) > 0 {
					buckets = m.s3.FilteredBuckets
				}
				if len(buckets) > 0 && m.s3.SelectedBucketIndex < len(buckets) {
					selectedBucket := buckets[m.s3.SelectedBucketIndex]
					m.loading = true
					return m, m.loadBucketVersioning(selectedBucket.Name)
				}
			}
		case "f":
			// Only filter on EC2 list screen
			if m.currentScreen == ec2Screen {
				m.filtering = true
				m.filterInput.Focus()
				return m, nil
			}
		case " ":
			// Toggle instance selection (space bar)
			if m.currentScreen == ec2Screen {
				// Use filtered list if active
				instances := m.ec2.Instances
				if len(m.ec2.FilteredInstances) > 0 {
					instances = m.ec2.FilteredInstances
				}
				if len(instances) > 0 && m.ec2.SelectedIndex < len(instances) {
					instanceID := instances[m.ec2.SelectedIndex].ID
					if m.ec2.SelectedInstances[instanceID] {
						delete(m.ec2.SelectedInstances, instanceID)
					} else {
						m.ec2.SelectedInstances[instanceID] = true
					}
					return m, nil
				}
			}
		case "a":
			// Toggle auto-refresh
			if m.currentScreen == ec2Screen {
				m.autoRefresh = !m.autoRefresh
				if m.autoRefresh {
					m.statusMessage = "Auto-refresh enabled (30s)"
					return m, tickCmd()
				} else {
					m.statusMessage = "Auto-refresh disabled"
				}
				return m, nil
			}
		case "x":
			// Clear all selections
			if m.currentScreen == ec2Screen {
				m.ec2.SelectedInstances = make(map[string]bool)
				m.statusMessage = "Cleared all selections"
				return m, nil
			}
		case "y":
			// Copy instance ID or IP to clipboard
			if m.currentScreen == ec2Screen && len(m.ec2.Instances) > 0 {
				instance := m.ec2.Instances[m.ec2.SelectedIndex]
				// Try to copy public IP, fallback to private IP, then instance ID
				toCopy := instance.PublicIP
				if toCopy == "" {
					toCopy = instance.PrivateIP
				}
				if toCopy == "" {
					toCopy = instance.ID
				}
				m.copyToClipboard = toCopy
				m.statusMessage = fmt.Sprintf("Copied to clipboard: %s", toCopy)
				return m, func() tea.Msg {
					// Try to copy to clipboard using xclip or pbcopy
					return nil
				}
			}
		case "s":
			// Start instance (single or bulk)
			if m.currentScreen == ec2Screen && len(m.ec2.SelectedInstances) > 0 {
				// Bulk action
				var instanceIDs []string
				for id := range m.ec2.SelectedInstances {
					instanceIDs = append(instanceIDs, id)
				}
				m.loading = true
				return m, m.performBulkAction("start", instanceIDs)
			}
			var instanceID string
			if m.currentScreen == ec2Screen && len(m.ec2.Instances) > 0 {
				instanceID = m.ec2.Instances[m.ec2.SelectedIndex].ID
			} else if m.currentScreen == ec2DetailsScreen && m.ec2.InstanceDetails != nil {
				instanceID = m.ec2.InstanceDetails.ID
			}
			if instanceID != "" {
				m.ec2.ShowingConfirmation = true
				m.ec2.ConfirmAction = "start"
				m.ec2.ConfirmInstanceID = instanceID
				return m, nil
			}
		case "S":
			// Stop instance (single or bulk)
			if m.currentScreen == ec2Screen && len(m.ec2.SelectedInstances) > 0 {
				// Bulk action
				var instanceIDs []string
				for id := range m.ec2.SelectedInstances {
					instanceIDs = append(instanceIDs, id)
				}
				m.ec2.ShowingConfirmation = true
				m.ec2.ConfirmAction = "stop"
				m.ec2.ConfirmInstanceID = fmt.Sprintf("%d instances", len(instanceIDs))
				return m, nil
			}
			var instanceID string
			if m.currentScreen == ec2Screen && len(m.ec2.Instances) > 0 {
				instanceID = m.ec2.Instances[m.ec2.SelectedIndex].ID
			} else if m.currentScreen == ec2DetailsScreen && m.ec2.InstanceDetails != nil {
				instanceID = m.ec2.InstanceDetails.ID
			}
			if instanceID != "" {
				m.ec2.ShowingConfirmation = true
				m.ec2.ConfirmAction = "stop"
				m.ec2.ConfirmInstanceID = instanceID
				return m, nil
			}
		case "R":
			// Reboot instance (single or bulk)
			if m.currentScreen == ec2Screen && len(m.ec2.SelectedInstances) > 0 {
				// Bulk action
				var instanceIDs []string
				for id := range m.ec2.SelectedInstances {
					instanceIDs = append(instanceIDs, id)
				}
				m.ec2.ShowingConfirmation = true
				m.ec2.ConfirmAction = "reboot"
				m.ec2.ConfirmInstanceID = fmt.Sprintf("%d instances", len(instanceIDs))
				return m, nil
			}
			var instanceID string
			if m.currentScreen == ec2Screen && len(m.ec2.Instances) > 0 {
				instanceID = m.ec2.Instances[m.ec2.SelectedIndex].ID
			} else if m.currentScreen == ec2DetailsScreen && m.ec2.InstanceDetails != nil {
				instanceID = m.ec2.InstanceDetails.ID
			}
			if instanceID != "" {
				m.ec2.ShowingConfirmation = true
				m.ec2.ConfirmAction = "reboot"
				m.ec2.ConfirmInstanceID = instanceID
				return m, nil
			}
		case "t":
			// Terminate instance (single or bulk)
			if m.currentScreen == ec2Screen && len(m.ec2.SelectedInstances) > 0 {
				// Bulk action
				var instanceIDs []string
				for id := range m.ec2.SelectedInstances {
					instanceIDs = append(instanceIDs, id)
				}
				m.ec2.ShowingConfirmation = true
				m.ec2.ConfirmAction = "terminate"
				m.ec2.ConfirmInstanceID = fmt.Sprintf("%d instances", len(instanceIDs))
				return m, nil
			}
			var instanceID string
			if m.currentScreen == ec2Screen && len(m.ec2.Instances) > 0 {
				instanceID = m.ec2.Instances[m.ec2.SelectedIndex].ID
			} else if m.currentScreen == ec2DetailsScreen && m.ec2.InstanceDetails != nil {
				instanceID = m.ec2.InstanceDetails.ID
			}
			if instanceID != "" {
				m.ec2.ShowingConfirmation = true
				m.ec2.ConfirmAction = "terminate"
				m.ec2.ConfirmInstanceID = instanceID
				return m, nil
			}
		case "C":
			// Launch SSM session (only in details view with SSM connected)
			if m.currentScreen == ec2DetailsScreen && m.ec2.InstanceDetails != nil && m.ec2.SSMStatus != nil && m.ec2.SSMStatus.Connected {
				// Return a message that will trigger SSM session launch
				return m, func() tea.Msg {
					return launchSSMSessionMsg{
						instanceID: m.ec2.InstanceDetails.ID,
						region:     m.awsClient.GetRegion(),
					}
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

// Helper function to clear all search state
func (m *model) clearSearch() {
	m.vimState.LastSearch = ""
	m.vimState.SearchResults = []int{}
	m.ec2.FilteredInstances = nil
	m.s3.FilteredBuckets = nil
	m.s3.FilteredObjects = nil
	m.ssoFilteredAccounts = nil
	m.eks.Filtered = nil
	m.ecs.Filtered = nil
	m.ecs.FilteredSvcs = nil
	m.ecs.FilteredTasks = nil
	m.ecr.FilteredRepos = nil
	m.ecr.FilteredImages = nil
	m.secretsFiltered = nil
}

func (m *model) initHistory(start screen) {
	m.screenHistory = []screen{start}
	m.historyIndex = 0
}

// navigateToScreen records history and moves to a new screen, trimming forward history.
func (m *model) navigateToScreen(s screen) {
	if len(m.screenHistory) == 0 {
		m.initHistory(s)
		m.currentScreen = s
		return
	}

	// If already at this screen, no-op
	if m.screenHistory[m.historyIndex] == s {
		m.currentScreen = s
		return
	}

	// Drop any forward history
	m.screenHistory = m.screenHistory[:m.historyIndex+1]
	m.screenHistory = append(m.screenHistory, s)
	m.historyIndex = len(m.screenHistory) - 1
	m.currentScreen = s

	// Keep history from growing unbounded (simple FIFO cap)
	const maxHistory = 50
	if len(m.screenHistory) > maxHistory {
		overflow := len(m.screenHistory) - maxHistory
		m.screenHistory = m.screenHistory[overflow:]
		if m.historyIndex >= overflow {
			m.historyIndex -= overflow
		} else {
			m.historyIndex = 0
		}
	}
}

// goBack navigates to the previous screen in history, returns true if moved.
func (m *model) goBack() bool {
	if m.historyIndex > 0 {
		m.historyIndex--
		m.currentScreen = m.screenHistory[m.historyIndex]
		return true
	}
	return false
}

// goForward navigates forward in history, returns true if moved.
func (m *model) goForward() bool {
	if m.historyIndex < len(m.screenHistory)-1 {
		m.historyIndex++
		m.currentScreen = m.screenHistory[m.historyIndex]
		return true
	}
	return false
}

// goToServiceSelection returns to the top-level service picker
func (m *model) goToServiceSelection() {
	m.clearSearch()
	m.navigateToScreen(serviceScreen)
	m.viewportOffset = 0
	m.statusMessage = "Select a service"
}

// Helper functions for VIM navigation
func (m *model) handleVimNavigation(action vim.NavigationAction) {
	// For detail screens, handle viewport scrolling instead of item navigation
	if m.currentScreen == ec2DetailsScreen || m.currentScreen == s3ObjectDetailsScreen || m.currentScreen == eksDetailsScreen || m.currentScreen == ecsTaskDetailsScreen || m.currentScreen == ecrImageDetailsScreen || m.currentScreen == secretsDetailsScreen {
		m.handleDetailViewScroll(action)
		return
	}

	var listLength int
	var currentIndex int

	// Determine current list and index (use filtered list if active)
	switch m.currentScreen {
	case regionScreen:
		if m.config != nil {
			listLength = len(m.config.Regions)
		}
		currentIndex = m.regionSelectedIndex
	case serviceScreen:
		listLength = len(getServices())
		currentIndex = m.serviceSelectedIndex
	case ecsScreen:
		if len(m.ecs.Filtered) > 0 {
			listLength = len(m.ecs.Filtered)
		} else if m.vimState.LastSearch != "" {
			return
		} else {
			listLength = len(m.ecs.Clusters)
		}
		currentIndex = m.ecs.SelectedIndex
	case ecsServicesScreen:
		if len(m.ecs.FilteredSvcs) > 0 {
			listLength = len(m.ecs.FilteredSvcs)
		} else if m.vimState.LastSearch != "" && len(m.ecs.Services) == 0 {
			return
		} else {
			listLength = len(m.ecs.Services)
		}
		currentIndex = m.ecs.SelectedSvc
	case ecsTasksScreen:
		if len(m.ecs.FilteredTasks) > 0 {
			listLength = len(m.ecs.FilteredTasks)
		} else if m.vimState.LastSearch != "" && len(m.ecs.Tasks) == 0 {
			return
		} else {
			listLength = len(m.ecs.Tasks)
		}
		currentIndex = m.ecs.SelectedTask
	case ecrScreen:
		if len(m.ecr.FilteredRepos) > 0 {
			listLength = len(m.ecr.FilteredRepos)
		} else if m.vimState.LastSearch != "" {
			return
		} else {
			listLength = len(m.ecr.Repositories)
		}
		currentIndex = m.ecr.SelectedRepoIndex
	case ecrImagesScreen:
		if len(m.ecr.FilteredImages) > 0 {
			listLength = len(m.ecr.FilteredImages)
		} else if m.vimState.LastSearch != "" && len(m.ecr.Images) == 0 {
			return
		} else {
			listLength = len(m.ecr.Images)
		}
		currentIndex = m.ecr.SelectedImage
	case secretsScreen:
		if len(m.secretsFiltered) > 0 {
			listLength = len(m.secretsFiltered)
		} else if m.vimState.LastSearch != "" && len(m.secrets) == 0 {
			return
		} else {
			listLength = len(m.secrets)
		}
		currentIndex = m.selectedSecretIndex
	case accountScreen:
		if len(m.ssoFilteredAccounts) > 0 {
			listLength = len(m.ssoFilteredAccounts)
		} else if m.vimState.LastSearch != "" {
			// Search active but no results
			return
		} else {
			listLength = len(m.ssoAccounts)
		}
		currentIndex = m.ssoSelectedIndex
	case ec2Screen:
		if len(m.ec2.FilteredInstances) > 0 {
			listLength = len(m.ec2.FilteredInstances)
		} else if m.vimState.LastSearch != "" {
			// Search active but no results
			return
		} else {
			listLength = len(m.ec2.Instances)
		}
		currentIndex = m.ec2.SelectedIndex
	case s3Screen:
		if len(m.s3.FilteredBuckets) > 0 {
			listLength = len(m.s3.FilteredBuckets)
		} else if m.vimState.LastSearch != "" {
			// Search active but no results
			return
		} else {
			listLength = len(m.s3.Buckets)
		}
		currentIndex = m.s3.SelectedBucketIndex
	case s3BrowseScreen:
		if len(m.s3.FilteredObjects) > 0 {
			listLength = len(m.s3.FilteredObjects)
		} else if m.vimState.LastSearch != "" {
			// Search active but no results
			return
		} else {
			listLength = len(m.s3.Objects)
		}
		currentIndex = m.s3.SelectedObjectIndex
	case eksScreen:
		if len(m.eks.Filtered) > 0 {
			listLength = len(m.eks.Filtered)
		} else if m.vimState.LastSearch != "" {
			// Search active but no results
			return
		} else {
			listLength = len(m.eks.Clusters)
		}
		currentIndex = m.eks.SelectedIndex
	default:
		return // No navigation for other screens
	}

	if listLength == 0 {
		return
	}

	// Calculate new index
	newIndex := vim.CalculateNewIndex(action, currentIndex, listLength, m.pageSize)

	// Set the new index
	m.setSelectedIndex(newIndex)

	// Ensure the selected item is visible in the viewport
	m.ensureVisible(newIndex, listLength)
}

// handleDetailViewScroll handles scrolling in detail views
func (m *model) handleDetailViewScroll(action vim.NavigationAction) {
	// For detail views, we scroll the viewport by lines
	switch action {
	case vim.MoveUp:
		if m.viewportOffset > 0 {
			m.viewportOffset--
		}
	case vim.MoveDown:
		m.viewportOffset++
		// Max will be clamped by renderWithViewport
	case vim.MoveTop:
		m.viewportOffset = 0
	case vim.MoveBottom:
		// Set to a large number, renderWithViewport will clamp it
		m.viewportOffset = 10000
	case vim.MoveHalfPageUp:
		m.viewportOffset -= m.pageSize / 2
		if m.viewportOffset < 0 {
			m.viewportOffset = 0
		}
	case vim.MoveHalfPageDown:
		m.viewportOffset += m.pageSize / 2
	case vim.MovePageUp:
		m.viewportOffset -= m.pageSize
		if m.viewportOffset < 0 {
			m.viewportOffset = 0
		}
	case vim.MovePageDown:
		m.viewportOffset += m.pageSize
	}
}

func (m *model) setSelectedIndex(index int) {
	switch m.currentScreen {
	case regionScreen:
		if m.config != nil && index >= 0 && index < len(m.config.Regions) {
			m.regionSelectedIndex = index
		}
	case serviceScreen:
		services := getServices()
		if index >= 0 && index < len(services) {
			m.serviceSelectedIndex = index
		}
	case accountScreen:
		if index >= 0 && index < len(m.ssoAccounts) {
			m.ssoSelectedIndex = index
		}
	case ec2Screen:
		if index >= 0 && index < len(m.ec2.Instances) {
			m.ec2.SelectedIndex = index
		}
	case s3Screen:
		if index >= 0 && index < len(m.s3.Buckets) {
			m.s3.SelectedBucketIndex = index
		}
	case s3BrowseScreen:
		if index >= 0 && index < len(m.s3.Objects) {
			m.s3.SelectedObjectIndex = index
		}
	case eksScreen:
		if index >= 0 && index < len(m.eks.Clusters) {
			m.eks.SelectedIndex = index
		}
	case ecsScreen:
		if len(m.ecs.Filtered) > 0 && index >= 0 && index < len(m.ecs.Filtered) {
			m.ecs.SelectedIndex = index
		} else if index >= 0 && index < len(m.ecs.Clusters) {
			m.ecs.SelectedIndex = index
		}
	case ecsServicesScreen:
		if len(m.ecs.FilteredSvcs) > 0 && index >= 0 && index < len(m.ecs.FilteredSvcs) {
			m.ecs.SelectedSvc = index
		} else if index >= 0 && index < len(m.ecs.Services) {
			m.ecs.SelectedSvc = index
		}
	case ecsTasksScreen:
		if len(m.ecs.FilteredTasks) > 0 && index >= 0 && index < len(m.ecs.FilteredTasks) {
			m.ecs.SelectedTask = index
		} else if index >= 0 && index < len(m.ecs.Tasks) {
			m.ecs.SelectedTask = index
		}
	case ecrScreen:
		if len(m.ecr.FilteredRepos) > 0 && index >= 0 && index < len(m.ecr.FilteredRepos) {
			m.ecr.SelectedRepoIndex = index
		} else if index >= 0 && index < len(m.ecr.Repositories) {
			m.ecr.SelectedRepoIndex = index
		}
	case ecrImagesScreen:
		if len(m.ecr.FilteredImages) > 0 && index >= 0 && index < len(m.ecr.FilteredImages) {
			m.ecr.SelectedImage = index
		} else if index >= 0 && index < len(m.ecr.Images) {
			m.ecr.SelectedImage = index
		}
	case secretsScreen:
		if len(m.secretsFiltered) > 0 && index >= 0 && index < len(m.secretsFiltered) {
			m.selectedSecretIndex = index
		} else if index >= 0 && index < len(m.secrets) {
			m.selectedSecretIndex = index
		}
	}
}

func (m *model) applyVimSearch() {
	// Build searchable strings for current view
	var searchItems []string

	switch m.currentScreen {
	case accountScreen:
		for _, acc := range m.ssoAccounts {
			var sb strings.Builder
			sb.WriteString(acc.AccountID)
			sb.WriteString(" ")
			sb.WriteString(acc.AccountName)
			sb.WriteString(" ")
			sb.WriteString(acc.RoleName)
			sb.WriteString(" ")
			sb.WriteString(acc.EmailAddress)
			searchItems = append(searchItems, strings.ToLower(sb.String()))
		}
	case ec2Screen:
		for _, inst := range m.ec2.Instances {
			searchItems = append(searchItems,
				strings.ToLower(inst.ID+" "+inst.Name+" "+inst.State+" "+inst.InstanceType+" "+inst.PublicIP+" "+inst.PrivateIP))
		}
	case s3Screen:
		for _, bucket := range m.s3.Buckets {
			searchItems = append(searchItems, strings.ToLower(bucket.Name+" "+bucket.Region))
		}
	case s3BrowseScreen:
		for _, obj := range m.s3.Objects {
			searchItems = append(searchItems, strings.ToLower(obj.Key))
		}
	case eksScreen:
		for _, cluster := range m.eks.Clusters {
			searchItems = append(searchItems, strings.ToLower(cluster.Name+" "+cluster.Version+" "+cluster.Status+" "+cluster.Region))
		}
	case ecsScreen:
		for _, cl := range m.ecs.Clusters {
			searchItems = append(searchItems, strings.ToLower(cl.Name+" "+cl.Status))
		}
	case ecsServicesScreen:
		for _, svc := range m.ecs.Services {
			line := fmt.Sprintf("%s %s %s %s", svc.Name, svc.Status, svc.TaskDefinition, svc.LaunchType)
			searchItems = append(searchItems, strings.ToLower(line))
		}
	case ecsTasksScreen:
		for _, task := range m.ecs.Tasks {
			line := fmt.Sprintf("%s %s %s %s %s", task.ID, task.Status, task.Health, task.CPU, task.Memory)
			searchItems = append(searchItems, strings.ToLower(line))
		}
	case ecrScreen:
		for _, repo := range m.ecr.Repositories {
			line := fmt.Sprintf("%s %s %s", repo.Name, repo.TagMutability, repo.EncryptionType)
			searchItems = append(searchItems, strings.ToLower(line))
		}
	case ecrImagesScreen:
		for _, img := range m.ecr.Images {
			line := fmt.Sprintf("%s %s %s", img.Digest, strings.Join(img.Tags, ","), img.ManifestType)
			searchItems = append(searchItems, strings.ToLower(line))
		}
	case secretsScreen:
		for _, sec := range m.secrets {
			line := fmt.Sprintf("%s %s %t", sec.Name, sec.Description, sec.RotationEnabled)
			searchItems = append(searchItems, strings.ToLower(line))
		}
	default:
		return
	}

	// Perform search
	m.vimState.SearchItems(searchItems)

	// Filter the view to only show matching items
	if len(m.vimState.SearchResults) > 0 {
		switch m.currentScreen {
		case accountScreen:
			m.ssoFilteredAccounts = make([]aws.SSOAccount, 0, len(m.vimState.SearchResults))
			for _, idx := range m.vimState.SearchResults {
				m.ssoFilteredAccounts = append(m.ssoFilteredAccounts, m.ssoAccounts[idx])
			}
		case ec2Screen:
			m.ec2.FilteredInstances = make([]aws.Instance, 0, len(m.vimState.SearchResults))
			for _, idx := range m.vimState.SearchResults {
				m.ec2.FilteredInstances = append(m.ec2.FilteredInstances, m.ec2.Instances[idx])
			}
		case s3Screen:
			m.s3.FilteredBuckets = make([]aws.Bucket, 0, len(m.vimState.SearchResults))
			for _, idx := range m.vimState.SearchResults {
				m.s3.FilteredBuckets = append(m.s3.FilteredBuckets, m.s3.Buckets[idx])
			}
		case s3BrowseScreen:
			m.s3.FilteredObjects = make([]aws.S3Object, 0, len(m.vimState.SearchResults))
			for _, idx := range m.vimState.SearchResults {
				m.s3.FilteredObjects = append(m.s3.FilteredObjects, m.s3.Objects[idx])
			}
		case eksScreen:
			m.eks.Filtered = make([]aws.EKSCluster, 0, len(m.vimState.SearchResults))
			for _, idx := range m.vimState.SearchResults {
				m.eks.Filtered = append(m.eks.Filtered, m.eks.Clusters[idx])
			}
		case ecsScreen:
			m.ecs.Filtered = make([]aws.ECSCluster, 0, len(m.vimState.SearchResults))
			for _, idx := range m.vimState.SearchResults {
				m.ecs.Filtered = append(m.ecs.Filtered, m.ecs.Clusters[idx])
			}
		case ecsServicesScreen:
			m.ecs.FilteredSvcs = make([]aws.ECSService, 0, len(m.vimState.SearchResults))
			for _, idx := range m.vimState.SearchResults {
				m.ecs.FilteredSvcs = append(m.ecs.FilteredSvcs, m.ecs.Services[idx])
			}
		case ecsTasksScreen:
			m.ecs.FilteredTasks = make([]aws.ECSTask, 0, len(m.vimState.SearchResults))
			for _, idx := range m.vimState.SearchResults {
				m.ecs.FilteredTasks = append(m.ecs.FilteredTasks, m.ecs.Tasks[idx])
			}
		case ecrScreen:
			m.ecr.FilteredRepos = make([]aws.ECRRepository, 0, len(m.vimState.SearchResults))
			for _, idx := range m.vimState.SearchResults {
				m.ecr.FilteredRepos = append(m.ecr.FilteredRepos, m.ecr.Repositories[idx])
			}
		case ecrImagesScreen:
			m.ecr.FilteredImages = make([]aws.ECRImage, 0, len(m.vimState.SearchResults))
			for _, idx := range m.vimState.SearchResults {
				m.ecr.FilteredImages = append(m.ecr.FilteredImages, m.ecr.Images[idx])
			}
		case secretsScreen:
			m.secretsFiltered = make([]aws.SecretSummary, 0, len(m.vimState.SearchResults))
			for _, idx := range m.vimState.SearchResults {
				m.secretsFiltered = append(m.secretsFiltered, m.secrets[idx])
			}
		}

		// Reset selection to first filtered result
		m.setSelectedIndex(0)
		m.statusMessage = fmt.Sprintf("Showing %d matching results (ESC or :cf to clear)", len(m.vimState.SearchResults))
	} else {
		m.statusMessage = "No matches found"
		// Clear filtered lists to show "no matches"
		switch m.currentScreen {
		case accountScreen:
			m.ssoFilteredAccounts = []aws.SSOAccount{}
		case ec2Screen:
			m.ec2.FilteredInstances = []aws.Instance{}
		case s3Screen:
			m.s3.FilteredBuckets = []aws.Bucket{}
		case s3BrowseScreen:
			m.s3.FilteredObjects = []aws.S3Object{}
		case eksScreen:
			m.eks.Filtered = []aws.EKSCluster{}
		case ecsScreen:
			m.ecs.Filtered = []aws.ECSCluster{}
		case ecsServicesScreen:
			m.ecs.FilteredSvcs = []aws.ECSService{}
		case ecsTasksScreen:
			m.ecs.FilteredTasks = []aws.ECSTask{}
		case ecrScreen:
			m.ecr.FilteredRepos = []aws.ECRRepository{}
		case ecrImagesScreen:
			m.ecr.FilteredImages = []aws.ECRImage{}
		case secretsScreen:
			m.secretsFiltered = []aws.SecretSummary{}
		}
	}
}

func (m *model) executeVimCommand(commandStr string) tea.Cmd {
	cmd := vim.ParseCommand(commandStr)
	cmdName := cmd.Name
	if cmdName == "acc" {
		cmdName = vim.CmdAccount
	}

	switch cmdName {
	case vim.CmdQuit, "quit":
		// Quit current view or app
		if m.currentScreen == ec2DetailsScreen {
			m.currentScreen = ec2Screen
			m.ec2.InstanceDetails = nil
		} else if m.currentScreen == s3BrowseScreen {
			m.currentScreen = s3Screen
			m.s3.Objects = nil
			m.s3.CurrentBucket = ""
			m.s3.CurrentPrefix = ""
		} else if m.currentScreen == s3ObjectDetailsScreen {
			m.currentScreen = s3BrowseScreen
			m.s3.ObjectDetails = nil
		} else if m.currentScreen == ecrImagesScreen {
			m.currentScreen = ecrScreen
		} else if m.currentScreen == ecrImageDetailsScreen {
			m.currentScreen = ecrImagesScreen
		} else if m.currentScreen == secretsDetailsScreen {
			m.currentScreen = secretsScreen
			m.currentSecretDetails = nil
		} else {
			return tea.Quit
		}
		return nil

	case vim.CmdRefresh, "refresh":
		// Refresh current view
		m.loading = true
		if m.currentScreen == ec2Screen {
			return m.loadEC2Instances
		} else if m.currentScreen == s3Screen {
			return m.loadS3Buckets
		} else if m.currentScreen == s3BrowseScreen {
			return m.loadS3Objects(m.s3.CurrentBucket, m.s3.CurrentPrefix, nil)
		}

	case vim.CmdSelectAll, "selectall":
		// Select all instances (EC2 only)
		if m.currentScreen == ec2Screen {
			for _, inst := range m.ec2.Instances {
				m.ec2.SelectedInstances[inst.ID] = true
			}
			m.statusMessage = fmt.Sprintf("Selected all %d instances", len(m.ec2.Instances))
		}

	case vim.CmdDeselectAll, "deselectall":
		// Deselect all instances
		if m.currentScreen == ec2Screen {
			m.ec2.SelectedInstances = make(map[string]bool)
			m.statusMessage = "Cleared all selections"
		}

	case vim.CmdClearFilter, "clearfilter":
		// Clear filter and reset filtered lists
		m.filter = ""
		m.vimState.LastSearch = ""
		m.vimState.SearchResults = []int{}
		m.ec2.FilteredInstances = nil
		m.s3.FilteredBuckets = nil
		m.s3.FilteredObjects = nil
		m.eks.Filtered = nil
		m.ecs.Filtered = nil
		m.ecs.FilteredSvcs = nil
		m.ecs.FilteredTasks = nil
		m.ecr.FilteredRepos = nil
		m.ecr.FilteredImages = nil
		m.secretsFiltered = nil
		m.statusMessage = "Filter cleared"

	case vim.CmdHelp, "h", "?":
		// Show help modal
		m.previousScreen = m.currentScreen
		m.currentScreen = helpScreen

	case vim.CmdEC2:
		// Switch to EC2 service
		m.clearSearch() // Clear search when switching screens
		m.navigateToScreen(ec2Screen)
		m.viewportOffset = 0
		if len(m.ec2.Instances) == 0 && m.awsClient != nil {
			m.loading = true
			return m.loadEC2Instances
		}
		m.statusMessage = "Switched to EC2"

	case vim.CmdS3:
		// Switch to S3 service
		m.clearSearch() // Clear search when switching screens
		m.navigateToScreen(s3Screen)
		m.viewportOffset = 0
		if len(m.s3.Buckets) == 0 && m.awsClient != nil {
			m.loading = true
			return m.loadS3Buckets
		}
		m.statusMessage = "Switched to S3"

	case vim.CmdEKS:
		// Switch to EKS service
		m.clearSearch() // Clear search when switching screens
		m.navigateToScreen(eksScreen)
		m.viewportOffset = 0
		if len(m.eks.Clusters) == 0 && m.awsClient != nil {
			m.loading = true
			return m.loadEKSClusters
		}
		m.statusMessage = "Switched to EKS"

	case vim.CmdECS:
		// Switch to ECS service
		m.clearSearch()
		m.navigateToScreen(ecsScreen)
		m.viewportOffset = 0
		if len(m.ecs.Clusters) == 0 && m.awsClient != nil {
			m.loading = true
			return m.loadECSClusters
		}
		m.statusMessage = "Switched to ECS"

	case vim.CmdECR:
		// Switch to ECR service
		m.clearSearch()
		m.navigateToScreen(ecrScreen)
		m.viewportOffset = 0
		if len(m.ecr.Repositories) == 0 && m.awsClient != nil {
			m.loading = true
			return m.loadECRRepositories
		}
		m.statusMessage = "Switched to ECR"

	case vim.CmdServices:
		// Show service selection screen
		m.clearSearch()
		m.navigateToScreen(serviceScreen)
		m.viewportOffset = 0
		m.statusMessage = "Select a service"

	case vim.CmdAccount:
		// Switch to account selection screen
		// Only works with SSO auth method
		if m.authConfig == nil || m.authConfig.Method != aws.AuthMethodSSO {
			// Send user back to the welcome/auth selection screen
			m.currentScreen = authMethodScreen
			m.configuringSSO = false
			m.configuringProfile = false
			m.statusMessage = "Choose an auth method to enable account switching"
			return nil
		}

		// Clear any stale search so navigation works in the account list
		m.clearSearch()

		if m.ssoAuthenticator == nil {
			// Start SSO authentication flow
			m.loading = true
			m.statusMessage = "Starting SSO authentication - opening browser..."
			return m.authenticateSSO(m.authConfig.SSOStartURL, m.authConfig.SSORegion)
		}
		// Show account selection screen
		m.currentScreen = accountScreen
		m.viewportOffset = 0
		if len(m.ssoAccounts) == 0 {
			m.loading = true
			return m.loadSSOAccounts()
		}
		m.statusMessage = "Account selection"

	case vim.CmdRegion:
		// Show region selection screen
		if m.config == nil || len(m.config.Regions) == 0 {
			m.statusMessage = "No regions configured"
			return nil
		}
		// Find current region index to pre-select it
		currentIndex := 0
		for i, r := range m.config.Regions {
			if r == m.config.Region {
				currentIndex = i
				break
			}
		}
		m.regionSelectedIndex = currentIndex
		m.previousScreen = m.currentScreen
		m.currentScreen = regionScreen
		m.viewportOffset = 0
		m.statusMessage = "Select a region"

	default:
		m.statusMessage = fmt.Sprintf("Unknown command: %s", cmd.Name)
	}

	return nil
}

// ensureVisible adjusts viewport offset to keep the selected item visible
func (m *model) ensureVisible(selectedIndex, listLength int) {
	if listLength == 0 {
		m.viewportOffset = 0
		return
	}

	// Calculate available height for the list data rows only
	// Account for: k9s header (9 lines), content border/padding (4 lines),
	//              table title+header (2 lines), footer info (2 lines), breadcrumb (3 lines),
	//              vim command line (2 lines)
	availableHeight := m.height - 22
	if availableHeight < 5 {
		availableHeight = 5 // Minimum viewport size
	}

	// Ensure selected item is visible
	if selectedIndex < m.viewportOffset {
		// Selected item is above viewport, scroll up
		m.viewportOffset = selectedIndex
	} else if selectedIndex >= m.viewportOffset+availableHeight {
		// Selected item is below viewport, scroll down
		m.viewportOffset = selectedIndex - availableHeight + 1
	}

	// Clamp viewport offset
	maxOffset := listLength - availableHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.viewportOffset > maxOffset {
		m.viewportOffset = maxOffset
	}
	if m.viewportOffset < 0 {
		m.viewportOffset = 0
	}
}

// getVisibleRange returns the start and end indices for items to display
func (m *model) getVisibleRange(listLength int) (int, int) {
	if listLength == 0 {
		return 0, 0
	}

	// Match the calculation in ensureVisible - MUST BE THE SAME
	availableHeight := m.height - 22
	if availableHeight < 5 {
		availableHeight = 5
	}

	start := m.viewportOffset
	end := m.viewportOffset + availableHeight
	if end > listLength {
		end = listLength
	}

	return start, end
}

// renderWithViewport takes a multi-line string and returns only visible lines
func (m *model) renderWithViewport(content string) string {
	lines := strings.Split(content, "\n")

	// Calculate available height for content
	// Must match the calculations in ensureVisible and getVisibleRange
	// Account for: k9s header (9 lines), content border/padding (4 lines),
	//              breadcrumb (3 lines), vim command line (2 lines), scroll indicator (1 line)
	availableHeight := m.height - 19
	if availableHeight < 10 {
		availableHeight = 10 // Minimum viewport
	}

	totalLines := len(lines)
	if totalLines <= availableHeight {
		// Content fits, no need to scroll
		return content
	}

	// Apply viewport
	start := m.viewportOffset
	end := start + availableHeight
	if end > totalLines {
		end = totalLines
	}
	if start >= totalLines {
		start = totalLines - availableHeight
		if start < 0 {
			start = 0
		}
	}

	visibleLines := lines[start:end]
	result := strings.Join(visibleLines, "\n")

	// Add scroll indicator
	scrollInfo := fmt.Sprintf("\n[Lines %d-%d of %d] (j/k to scroll)", start+1, end, totalLines)
	result += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(scrollInfo)

	return result
}

func (m model) View() string {
	var s string

	// K9s-style header: left sidebar with context info, center/right with key hints
	s += m.renderK9sHeader() + "\n"

	// Content area
	contentStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(1, 2).
		Width(m.width - 2) // Full width minus small margin

	// Only apply MaxHeight for main screens that need viewport control
	// Auth screens should show full content
	if m.currentScreen >= ec2Screen {
		// Set max height to prevent overflow and top clipping
		// Leave room for header (~7 lines) + border (2) + padding (2) + breadcrumb (1) + status (1) = ~13 total
		// But be less aggressive - use height - 8 to give more content space
		maxContentHeight := m.height - 8
		if maxContentHeight < 15 {
			maxContentHeight = 15
		}
		contentStyle = contentStyle.MaxHeight(maxContentHeight)
	}

	var content string
	switch m.currentScreen {
	case authMethodScreen:
		content = m.renderAuthMethodSelection()
	case authProfileScreen:
		content = m.renderProfileConfig()
	case ssoConfigScreen:
		content = m.renderSSOConfig()
	case accountScreen:
		content = m.renderAccountSelection()
	case serviceScreen:
		content = m.renderServiceSelection()
	case regionScreen:
		content = m.renderRegionSelection()
	case ec2Screen:
		content = m.renderEC2()
	case ec2DetailsScreen:
		content = m.renderEC2Details()
	case s3Screen:
		content = m.renderS3()
	case s3BrowseScreen:
		content = m.renderS3Browse()
	case s3ObjectDetailsScreen:
		content = m.renderS3ObjectDetails()
	case eksScreen:
		content = m.renderEKS()
	case eksDetailsScreen:
		content = m.renderEKSDetails()
	case ecsScreen:
		content = m.renderECS()
	case ecsServicesScreen:
		content = m.renderECSServices()
	case ecsTasksScreen:
		content = m.renderECSTasks()
	case ecsTaskDetailsScreen:
		content = m.renderECSTaskDetails()
	case ecrScreen:
		content = m.renderECRRepositories()
	case ecrImagesScreen:
		content = m.renderECRImages()
	case ecrImageDetailsScreen:
		content = m.renderECRImageDetails()
	case secretsScreen:
		content = m.renderSecrets()
	case secretsDetailsScreen:
		content = m.renderSecretDetails()
	case helpScreen:
		content = m.renderHelp()
	}

	if m.filtering {
		s += "\n" + m.filterInput.View()
	}

	s += contentStyle.Render(content) + "\n"

	// Show delete confirmation input
	if m.s3.ConfirmDelete {
		confirmStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")).
			Bold(true)
		s += "\n" + confirmStyle.Render("DELETE CONFIRMATION")
		s += "\n" + m.deleteConfirmInput.View()
		s += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Press ESC to cancel")
	}

	// Show VIM mode indicator
	if m.vimState.Mode == vim.SearchMode {
		searchStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("3")).
			Background(lipgloss.Color("0"))
		s += "\n" + searchStyle.Render("/"+m.vimState.SearchQuery)
	} else if m.vimState.Mode == vim.CommandMode {
		commandStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("6")).
			Background(lipgloss.Color("0"))
		s += "\n" + commandStyle.Render(":"+m.vimState.CommandBuffer)

		// Show command suggestions
		if len(m.commandSuggestions) > 0 {
			suggestionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
			suggestions := strings.Join(m.commandSuggestions, ", ")
			s += "\n" + suggestionStyle.Render("  suggestions: "+suggestions)
		}
	}

	// Show confirmation dialog
	if m.ec2.ShowingConfirmation {
		confirmStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("3")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("3")).
			Padding(1, 2)

		actionText := m.ec2.ConfirmAction
		if m.ec2.ConfirmAction == "terminate" {
			actionText = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render("TERMINATE")
		}

		confirmMsg := fmt.Sprintf("Are you sure you want to %s instance %s?\n\n(y)es / (n)o",
			actionText, m.ec2.ConfirmInstanceID)
		s += "\n" + confirmStyle.Render(confirmMsg)
	}

	// Show S3 info popup (bucket policy, versioning, presigned URL)
	if m.s3.ShowingInfo {
		infoStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("6")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("6")).
			Padding(1, 2).
			Width(80)

		var infoContent string
		if m.s3.InfoType == "policy" {
			infoContent = "Bucket Policy:\n\n" + m.s3.BucketPolicy
		} else if m.s3.InfoType == "versioning" {
			infoContent = "Bucket Versioning:\n\n" + m.s3.BucketVersioning
		}
		s += "\n" + infoStyle.Render(infoContent)
		s += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Press ESC to close")
	}

	// Show presigned URL (only on non-macOS platforms, since macOS auto-copies)
	if m.s3.PresignedURL != "" && runtime.GOOS != "darwin" {
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
		urlStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
		s += "\n" + labelStyle.Render("Presigned URL (1 hour): ") + urlStyle.Render(m.s3.PresignedURL)
	}

	// Show status message
	if m.statusMessage != "" {
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
		s += "\n" + statusStyle.Render(m.statusMessage)
	}

	// K9s-style breadcrumb navigation at bottom
	s += "\n" + m.renderK9sBreadcrumb()

	return s
}

// renderK9sHeader creates a k9s-style header with context info and key hints
func (m model) renderK9sHeader() string {
	// K9s color scheme
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))                 // Yellow/orange
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))                 // White
	keyHintKeyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Bold(true) // Magenta
	keyHintActionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))           // Gray

	// Left side: Context information
	var leftSide strings.Builder

	// Service name (like "Context" in k9s)
	var serviceName, viewName string
	switch m.currentScreen {
	case authMethodScreen:
		serviceName = "Setup"
		viewName = "Authentication"
	case authProfileScreen:
		serviceName = "Setup"
		viewName = "AWS Profile"
	case ssoConfigScreen:
		serviceName = "Setup"
		viewName = "SSO Configuration"
	case accountScreen:
		serviceName = "AWS"
		viewName = "Account Selection"
	case serviceScreen:
		serviceName = "AWS"
		viewName = "Service Selection"
	case ec2Screen:
		serviceName = "EC2"
		viewName = "Instances"
	case ec2DetailsScreen:
		serviceName = "EC2"
		viewName = "Details"
	case s3Screen:
		serviceName = "S3"
		viewName = "Buckets"
	case s3BrowseScreen:
		serviceName = "S3"
		viewName = "Objects"
	case s3ObjectDetailsScreen:
		serviceName = "S3"
		viewName = "Object Details"
	case eksScreen:
		serviceName = "EKS"
		viewName = "Clusters"
	case eksDetailsScreen:
		serviceName = "EKS"
		viewName = "Cluster Details"
	case ecsScreen:
		serviceName = "ECS"
		viewName = "Clusters"
	case ecsServicesScreen:
		serviceName = "ECS"
		viewName = "Services"
	case ecsTasksScreen:
		serviceName = "ECS"
		viewName = "Tasks"
	case ecsTaskDetailsScreen:
		serviceName = "ECS"
		viewName = "Task Details"
	case ecrScreen:
		serviceName = "ECR"
		viewName = "Repositories"
	case ecrImagesScreen:
		serviceName = "ECR"
		viewName = "Images"
	case ecrImageDetailsScreen:
		serviceName = "ECR"
		viewName = "Image Details"
	case secretsScreen:
		serviceName = "Secrets"
		viewName = "List"
	case secretsDetailsScreen:
		serviceName = "Secrets"
		viewName = "Details"
	}

	leftSide.WriteString(labelStyle.Render("Service: ") + valueStyle.Render(serviceName) + "\n")
	leftSide.WriteString(labelStyle.Render("View:    ") + valueStyle.Render(viewName) + "\n")

	if m.awsClient != nil {
		leftSide.WriteString(labelStyle.Render("Region:  ") + valueStyle.Render(m.awsClient.GetRegion()) + "\n")

		// Display account information if available
		if m.currentAccountName != "" {
			leftSide.WriteString(labelStyle.Render("Account: ") + valueStyle.Render(m.currentAccountName))
			if m.currentAccountID != "" {
				leftSide.WriteString(valueStyle.Render(fmt.Sprintf(" (%s)", m.currentAccountID)))
			}
			leftSide.WriteString("\n")
		} else if m.awsClient.GetAccountID() != "" {
			leftSide.WriteString(labelStyle.Render("Account: ") + valueStyle.Render(m.awsClient.GetAccountID()) + "\n")
		}
	}

	// Add version info (like K9s Rev)
	leftSide.WriteString(labelStyle.Render("lazyaws: ") + valueStyle.Render("v0.2.0") + "\n")

	// Right side: Key hints based on current screen
	var keyHints []string
	switch m.currentScreen {
	case ec2Screen:
		keyHints = []string{
			keyHintKeyStyle.Render("<enter>") + " " + keyHintActionStyle.Render("Details"),
			keyHintKeyStyle.Render("<space>") + " " + keyHintActionStyle.Render("Select"),
			keyHintKeyStyle.Render("<s>") + " " + keyHintActionStyle.Render("Start"),
			keyHintKeyStyle.Render("<S>") + " " + keyHintActionStyle.Render("Stop"),
			keyHintKeyStyle.Render("<R>") + " " + keyHintActionStyle.Render("Reboot"),
			keyHintKeyStyle.Render("<t>") + " " + keyHintActionStyle.Render("Terminate"),
			keyHintKeyStyle.Render("<:>") + " " + keyHintActionStyle.Render("Command"),
			keyHintKeyStyle.Render("</>") + " " + keyHintActionStyle.Render("Search"),
		}
	case ec2DetailsScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<s>") + " " + keyHintActionStyle.Render("Start"),
			keyHintKeyStyle.Render("<S>") + " " + keyHintActionStyle.Render("Stop"),
			keyHintKeyStyle.Render("<R>") + " " + keyHintActionStyle.Render("Reboot"),
			keyHintKeyStyle.Render("<t>") + " " + keyHintActionStyle.Render("Terminate"),
			keyHintKeyStyle.Render("<esc>") + " " + keyHintActionStyle.Render("Back"),
		}
		if m.ec2.SSMStatus != nil && m.ec2.SSMStatus.Connected {
			keyHints = append(keyHints, keyHintKeyStyle.Render("<C>")+" "+keyHintActionStyle.Render("SSM Connect"))
		}
	case s3Screen:
		keyHints = []string{
			keyHintKeyStyle.Render("<enter>") + " " + keyHintActionStyle.Render("Browse"),
			keyHintKeyStyle.Render("<D>") + " " + keyHintActionStyle.Render("Delete"),
			keyHintKeyStyle.Render("<p>") + " " + keyHintActionStyle.Render("Policy"),
			keyHintKeyStyle.Render("<v>") + " " + keyHintActionStyle.Render("Versioning"),
			keyHintKeyStyle.Render("<:>") + " " + keyHintActionStyle.Render("Command"),
			keyHintKeyStyle.Render("</>") + " " + keyHintActionStyle.Render("Search"),
		}
	case s3BrowseScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<enter>") + " " + keyHintActionStyle.Render("Open"),
			keyHintKeyStyle.Render("<e>") + " " + keyHintActionStyle.Render("Edit"),
			keyHintKeyStyle.Render("<d>") + " " + keyHintActionStyle.Render("Download"),
			keyHintKeyStyle.Render("<D>") + " " + keyHintActionStyle.Render("Delete"),
			keyHintKeyStyle.Render("<h>") + " " + keyHintActionStyle.Render("Up"),
			keyHintKeyStyle.Render("<:>") + " " + keyHintActionStyle.Render("Command"),
			keyHintKeyStyle.Render("</>") + " " + keyHintActionStyle.Render("Search"),
		}
	case s3ObjectDetailsScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<e>") + " " + keyHintActionStyle.Render("Edit"),
			keyHintKeyStyle.Render("<d>") + " " + keyHintActionStyle.Render("Download"),
			keyHintKeyStyle.Render("<p>") + " " + keyHintActionStyle.Render("Presigned URL"),
			keyHintKeyStyle.Render("<esc>") + " " + keyHintActionStyle.Render("Back"),
		}
	case eksScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<enter>") + " " + keyHintActionStyle.Render("Details"),
			keyHintKeyStyle.Render("<K>") + " " + keyHintActionStyle.Render("Update Kubeconfig"),
			keyHintKeyStyle.Render("<:>") + " " + keyHintActionStyle.Render("Command"),
		}
	case eksDetailsScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<K>") + " " + keyHintActionStyle.Render("Update Kubeconfig"),
			keyHintKeyStyle.Render("<esc>") + " " + keyHintActionStyle.Render("Back"),
		}
	case ecsScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<enter>") + " " + keyHintActionStyle.Render("Services"),
			keyHintKeyStyle.Render("</>") + " " + keyHintActionStyle.Render("Search"),
			keyHintKeyStyle.Render("<:>") + " " + keyHintActionStyle.Render("Command"),
		}
	case ecsServicesScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<enter>") + " " + keyHintActionStyle.Render("Tasks"),
			keyHintKeyStyle.Render("</>") + " " + keyHintActionStyle.Render("Search"),
			keyHintKeyStyle.Render("<r>") + " " + keyHintActionStyle.Render("Refresh"),
			keyHintKeyStyle.Render("<esc>") + " " + keyHintActionStyle.Render("Clusters"),
		}
	case ecsTasksScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<enter>") + " " + keyHintActionStyle.Render("Details/Logs"),
			keyHintKeyStyle.Render("</>") + " " + keyHintActionStyle.Render("Search"),
			keyHintKeyStyle.Render("<r>") + " " + keyHintActionStyle.Render("Refresh"),
			keyHintKeyStyle.Render("<esc>") + " " + keyHintActionStyle.Render("Services"),
		}
	case ecsTaskDetailsScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<esc>") + " " + keyHintActionStyle.Render("Back"),
			keyHintKeyStyle.Render("<r>") + " " + keyHintActionStyle.Render("Reload Logs"),
		}
	case ecrScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<enter>") + " " + keyHintActionStyle.Render("Images"),
			keyHintKeyStyle.Render("</>") + " " + keyHintActionStyle.Render("Search"),
			keyHintKeyStyle.Render("<r>") + " " + keyHintActionStyle.Render("Refresh"),
		}
	case ecrImagesScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<enter>") + " " + keyHintActionStyle.Render("Details/Scan"),
			keyHintKeyStyle.Render("</>") + " " + keyHintActionStyle.Render("Search"),
			keyHintKeyStyle.Render("<r>") + " " + keyHintActionStyle.Render("Refresh"),
			keyHintKeyStyle.Render("<esc>") + " " + keyHintActionStyle.Render("Repositories"),
		}
	case ecrImageDetailsScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<esc>") + " " + keyHintActionStyle.Render("Images"),
			keyHintKeyStyle.Render("<r>") + " " + keyHintActionStyle.Render("Reload Scan"),
		}
	case secretsScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<enter>") + " " + keyHintActionStyle.Render("Details"),
			keyHintKeyStyle.Render("</>") + " " + keyHintActionStyle.Render("Search"),
			keyHintKeyStyle.Render("<r>") + " " + keyHintActionStyle.Render("Refresh"),
		}
	case secretsDetailsScreen:
		keyHints = []string{
			keyHintKeyStyle.Render("<esc>") + " " + keyHintActionStyle.Render("Back"),
			keyHintKeyStyle.Render("<r>") + " " + keyHintActionStyle.Render("Reload"),
		}
	}

	// ASCII art logo (simplified version for lazyaws)
	logo := `  _
 | | __ _ ____ _   _
 | |/ _  |_  /| | | |
 | | (_| |/ / | |_| |
 |_|__,_/___| ___,_|
   __ ___      _____
  / _  |-|-|/| / __|
 | (_| | V  V ||__ |
  __,_| |/|/| |___/`

	// Layout: left side, center spacing, key hints (2 columns), right side logo
	leftContent := leftSide.String()

	// Format key hints in columns
	var rightSide strings.Builder
	for i := 0; i < len(keyHints); i += 2 {
		if i < len(keyHints) {
			rightSide.WriteString(keyHints[i])
			if i+1 < len(keyHints) {
				rightSide.WriteString("  " + keyHints[i+1])
			}
			rightSide.WriteString("\n")
		}
	}

	// Combine left and right with proper spacing
	leftLines := strings.Split(leftContent, "\n")
	rightLines := strings.Split(rightSide.String(), "\n")
	logoLines := strings.Split(logo, "\n")

	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}
	if len(logoLines) > maxLines {
		maxLines = len(logoLines)
	}

	var header strings.Builder
	for i := 0; i < maxLines; i++ {
		// Build the line in a temporary buffer to measure and truncate if needed
		var line strings.Builder

		// Left side (fixed width ~30 chars)
		left := ""
		if i < len(leftLines) {
			left = leftLines[i]
		}
		line.WriteString(left)

		// Padding to align
		leftWidth := lipgloss.Width(left)
		padding := 30 - leftWidth
		if padding > 0 {
			line.WriteString(strings.Repeat(" ", padding))
		}

		// Right side key hints (middle section)
		right := ""
		if i < len(rightLines) {
			right = rightLines[i]
		}
		line.WriteString(right)

		// Logo on far right - but ensure we don't overflow terminal width
		if i < len(logoLines) && m.width > 0 {
			rightWidth := lipgloss.Width(right)
			// Calculate how much space we've used: left (30) + right
			usedWidth := 30 + rightWidth
			// Only add logo if we have room (need at least 30 chars for logo)
			if usedWidth+30 < m.width {
				logoPadding := 90 - rightWidth
				if logoPadding > 0 && logoPadding < 100 { // Sanity check
					line.WriteString(strings.Repeat(" ", logoPadding))
				}
				line.WriteString(labelStyle.Render(logoLines[i]))
			}
		}

		// Truncate line to terminal width if needed
		lineStr := line.String()
		lineWidth := lipgloss.Width(lineStr)
		if m.width > 0 && lineWidth > m.width {
			// Line is too long, truncate it
			// This is tricky with ANSI codes, so just skip the logo for this line
			var safeLine strings.Builder
			safeLine.WriteString(left)
			if padding > 0 {
				safeLine.WriteString(strings.Repeat(" ", padding))
			}
			safeLine.WriteString(right)
			lineStr = safeLine.String()
		}

		header.WriteString(lineStr)
		// Add ANSI clear-to-end-of-line to prevent any artifacts
		header.WriteString("\x1b[K")
		header.WriteString("\n")
	}

	return header.String()
}

// renderK9sBreadcrumb creates a k9s-style breadcrumb navigation at the bottom
func (m model) renderK9sBreadcrumb() string {
	breadcrumbStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("220")).
		Bold(true).
		Padding(0, 1)

	var breadcrumbs []string

	switch m.currentScreen {
	case ec2Screen:
		breadcrumbs = []string{"<ec2>", "<instances>"}
	case ec2DetailsScreen:
		breadcrumbs = []string{"<ec2>", "<instances>", "<details>"}
	case s3Screen:
		breadcrumbs = []string{"<s3>", "<buckets>"}
	case s3BrowseScreen:
		if m.s3.CurrentBucket != "" {
			breadcrumbs = []string{"<s3>", "<" + m.s3.CurrentBucket + ">"}
			if m.s3.CurrentPrefix != "" {
				breadcrumbs = append(breadcrumbs, "<"+m.s3.CurrentPrefix+">")
			}
		}
	case s3ObjectDetailsScreen:
		breadcrumbs = []string{"<s3>", "<object>", "<details>"}
	case eksScreen:
		breadcrumbs = []string{"<eks>", "<clusters>"}
	case eksDetailsScreen:
		if m.eks.Details != nil {
			breadcrumbs = []string{"<eks>", "<clusters>", "<" + m.eks.Details.Name + ">"}
		} else {
			breadcrumbs = []string{"<eks>", "<clusters>", "<details>"}
		}
	case ecsScreen:
		breadcrumbs = []string{"<ecs>", "<clusters>"}
	case ecsServicesScreen:
		label := m.ecs.CurrentCluster
		if label == "" {
			label = "<cluster>"
		} else {
			label = "<" + label + ">"
		}
		breadcrumbs = []string{"<ecs>", "<clusters>", label, "<services>"}
	case ecsTasksScreen:
		cluster := m.ecs.CurrentCluster
		service := m.ecs.CurrentService
		if cluster == "" {
			cluster = "<cluster>"
		} else {
			cluster = "<" + cluster + ">"
		}
		if service == "" {
			service = "<service>"
		} else {
			service = "<" + service + ">"
		}
		breadcrumbs = []string{"<ecs>", "<clusters>", cluster, "<services>", service, "<tasks>"}
	case ecsTaskDetailsScreen:
		cluster := m.ecs.CurrentCluster
		service := m.ecs.CurrentService
		if cluster == "" {
			cluster = "<cluster>"
		} else {
			cluster = "<" + cluster + ">"
		}
		if service == "" {
			service = "<service>"
		} else {
			service = "<" + service + ">"
		}
		taskLabel := "<task>"
		if len(m.ecs.Tasks) > 0 && m.ecs.SelectedTask < len(m.ecs.Tasks) {
			taskLabel = "<" + m.ecs.Tasks[m.ecs.SelectedTask].ID + ">"
		}
		breadcrumbs = []string{"<ecs>", "<clusters>", cluster, "<services>", service, "<tasks>", taskLabel, "<details>"}
	case ecrScreen:
		breadcrumbs = []string{"<ecr>", "<repositories>"}
	case ecrImagesScreen:
		repo := m.ecr.CurrentRepo
		if repo == "" {
			repo = "<repo>"
		} else {
			repo = "<" + repo + ">"
		}
		breadcrumbs = []string{"<ecr>", "<repositories>", repo, "<images>"}
	case ecrImageDetailsScreen:
		repo := m.ecr.CurrentRepo
		if repo == "" {
			repo = "<repo>"
		} else {
			repo = "<" + repo + ">"
		}
		breadcrumbs = []string{"<ecr>", "<repositories>", repo, "<images>", "<details>"}
	case secretsScreen:
		breadcrumbs = []string{"<secrets>", "<list>"}
	case secretsDetailsScreen:
		name := "<secret>"
		if m.currentSecretDetails != nil {
			name = "<" + m.currentSecretDetails.Name + ">"
		}
		breadcrumbs = []string{"<secrets>", "<list>", name, "<details>"}
	}

	var result strings.Builder
	for i, bc := range breadcrumbs {
		if i > 0 {
			result.WriteString(" ")
		}
		result.WriteString(breadcrumbStyle.Render(bc))
	}

	return result.String()
}

func (m model) renderAuthMethodSelection() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	instructionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("51")).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	var content strings.Builder

	content.WriteString(titleStyle.Render("Welcome to lazyaws!") + "\n\n")
	content.WriteString(labelStyle.Render("Choose your AWS authentication method:") + "\n\n")

	// Option 0: Environment Variables
	envAvailable := aws.CheckEnvVarsAvailable()
	envText := "Environment Variables"
	if envAvailable {
		envText += " (detected)"
	}
	if m.selectedAuthMethod == 0 {
		content.WriteString(selectedStyle.Render("> "+envText) + "\n")
	} else {
		content.WriteString(normalStyle.Render("  "+envText) + "\n")
	}
	content.WriteString(instructionStyle.Render("  Uses AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY") + "\n\n")

	// Option 1: AWS Profile
	profileText := "AWS Profile"
	if m.selectedAuthMethod == 1 {
		content.WriteString(selectedStyle.Render("> "+profileText) + "\n")
	} else {
		content.WriteString(normalStyle.Render("  "+profileText) + "\n")
	}
	content.WriteString(instructionStyle.Render("  Uses credentials from ~/.aws/config or ~/.aws/credentials") + "\n\n")

	// Option 2: SSO
	ssoText := "AWS SSO (IAM Identity Center)"
	if m.selectedAuthMethod == 2 {
		content.WriteString(selectedStyle.Render("> "+ssoText) + "\n")
	} else {
		content.WriteString(normalStyle.Render("  "+ssoText) + "\n")
	}
	content.WriteString(instructionStyle.Render("  Uses AWS Single Sign-On for multi-account access") + "\n\n\n")

	content.WriteString(instructionStyle.Render("Use ↑/↓ or j/k to select | Enter to continue | ESC to quit") + "\n")

	return content.String()
}

func (m model) renderProfileConfig() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	instructionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

	var content strings.Builder

	content.WriteString(titleStyle.Render("AWS Profile Configuration") + "\n\n")
	content.WriteString(labelStyle.Render("Enter the name of your AWS profile:") + "\n\n")

	content.WriteString(instructionStyle.Render("This should match a profile name in:") + "\n")
	content.WriteString(instructionStyle.Render("  • ~/.aws/credentials") + "\n")
	content.WriteString(instructionStyle.Render("  • ~/.aws/config") + "\n\n")

	content.WriteString(instructionStyle.Render("Common profile names: default, dev, prod, staging") + "\n\n")

	content.WriteString(labelStyle.Render("Profile name:") + "\n")
	content.WriteString(m.profileInput.View() + "\n\n")

	if m.statusMessage != "" {
		content.WriteString(errorStyle.Render(m.statusMessage) + "\n\n")
	}

	content.WriteString(instructionStyle.Render("Press Enter to save | ESC to go back") + "\n")

	return content.String()
}

func (m model) renderSSOConfig() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	instructionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

	var content strings.Builder

	content.WriteString(titleStyle.Render("AWS SSO Configuration") + "\n\n")
	content.WriteString(labelStyle.Render("To use lazyaws, you need to configure your AWS SSO (IAM Identity Center) start URL.") + "\n\n")

	content.WriteString(instructionStyle.Render("This is typically a URL like:") + "\n")
	content.WriteString(instructionStyle.Render("  https://d-xxxxxxxxxx.awsapps.com/start") + "\n\n")

	content.WriteString(instructionStyle.Render("You can find this URL in:") + "\n")
	content.WriteString(instructionStyle.Render("  • AWS IAM Identity Center console") + "\n")
	content.WriteString(instructionStyle.Render("  • Your AWS SSO login page") + "\n")
	content.WriteString(instructionStyle.Render("  • Your organization's AWS access portal") + "\n\n")

	content.WriteString(labelStyle.Render("Enter your SSO Start URL:") + "\n")
	content.WriteString(m.ssoURLInput.View() + "\n\n")

	content.WriteString(labelStyle.Render("Enter your SSO Region (where Identity Center is set up):") + "\n")
	content.WriteString(m.ssoRegionInput.View() + "\n\n")

	if m.statusMessage != "" {
		content.WriteString(errorStyle.Render(m.statusMessage) + "\n\n")
	}

	content.WriteString(instructionStyle.Render("Press Enter to save | ESC to go back") + "\n")

	return content.String()
}

func (m model) renderAccountSelection() string {
	title := lipgloss.NewStyle().Bold(true).Render("AWS Account Selection")
	if m.vimState.LastSearch != "" {
		title += lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(fmt.Sprintf(" [search: %s]", m.vimState.LastSearch))
	}

	if m.loading {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("Loading accounts...")
	}

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		return title + "\n\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	// Use filtered accounts if VIM search is active, otherwise use all accounts
	accounts := m.ssoAccounts
	if len(m.ssoFilteredAccounts) > 0 {
		accounts = m.ssoFilteredAccounts
	} else if m.vimState.LastSearch != "" {
		// Search is active but no results
		accounts = []aws.SSOAccount{}
	}

	if len(accounts) == 0 {
		if m.vimState.LastSearch != "" {
			return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No accounts match your search")
		}
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No accounts found")
	}

	// Ensure selected item is visible and get viewport range
	m.ensureVisible(m.ssoSelectedIndex, len(accounts))
	start, end := m.getVisibleRange(len(accounts))

	// Build table header (k9s style)
	var content strings.Builder

	// Title with count - k9s style
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true) // Cyan
	searchInfo := ""
	if m.vimState.LastSearch != "" {
		searchInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Render("(" + m.vimState.LastSearch + ")")
	}
	tableTitle := fmt.Sprintf("AWS-Accounts%s[%d]", searchInfo, len(accounts))
	titleText := titleStyle.Render(tableTitle)

	// Center the title with dashes on both sides
	titleWidth := len(tableTitle)
	totalWidth := 120
	dashesWidth := (totalWidth - titleWidth - 2) / 2
	if dashesWidth < 1 {
		dashesWidth = 1
	}

	content.WriteString(strings.Repeat("─", dashesWidth) + " ")
	content.WriteString(titleText)
	content.WriteString(" " + strings.Repeat("─", dashesWidth) + "\n")

	// Table header - k9s uses uppercase and symbols
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Underline(true)
	content.WriteString(headerStyle.Render(fmt.Sprintf("%-30s %-20s %-35s %-30s",
		"ACCOUNT NAME", "ACCOUNT ID", "ROLE", "EMAIL")) + "\n")

	// Build table rows (only visible items)
	for i := start; i < end; i++ {
		account := accounts[i]

		// Build row with proper spacing
		row := fmt.Sprintf("%-30s %-20s %-35s %-30s",
			uiShared.Truncate(account.AccountName, 30),
			account.AccountID,
			uiShared.Truncate(account.RoleName, 35),
			uiShared.Truncate(account.EmailAddress, 30),
		)

		if i == m.ssoSelectedIndex {
			// Highlight the selected row - k9s style with cyan background
			for len(row) < 118 {
				row += " "
			}
			selectedStyle := lipgloss.NewStyle().
				Background(lipgloss.Color("51")).
				Foreground(lipgloss.Color("0")).
				Bold(true)
			content.WriteString(selectedStyle.Render(row) + "\n")
		} else {
			// Normal row
			normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
			content.WriteString(normalStyle.Render(row) + "\n")
		}
	}

	// Add scroll indicators
	if start > 0 || end < len(accounts) {
		scrollInfo := fmt.Sprintf("\n[Showing %d-%d of %d | Use j/k or ↓/↑ to navigate]", start+1, end, len(accounts))
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(scrollInfo))
	}

	return content.String()
}

func getServices() []string {
	return []string{"EC2", "S3", "EKS", "ECS", "ECR", "Secrets"}
}

func (m model) renderServiceSelection() string {
	title := lipgloss.NewStyle().Bold(true).Render("AWS Service Selection")

	services := getServices()
	if len(services) == 0 {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No services available")
	}

	// Ensure selected item is visible and get viewport range
	m.ensureVisible(m.serviceSelectedIndex, len(services))
	start, end := m.getVisibleRange(len(services))

	var content strings.Builder

	// Title with count - k9s style
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true)
	tableTitle := fmt.Sprintf("AWS-Services[%d]", len(services))
	titleText := titleStyle.Render(tableTitle)

	// Center the title with dashes on both sides
	titleWidth := len(tableTitle)
	totalWidth := 60
	dashesWidth := (totalWidth - titleWidth - 2) / 2
	if dashesWidth < 1 {
		dashesWidth = 1
	}

	content.WriteString(strings.Repeat("─", dashesWidth) + " ")
	content.WriteString(titleText)
	content.WriteString(" " + strings.Repeat("─", dashesWidth) + "\n")

	// Header
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Underline(true)
	content.WriteString(headerStyle.Render(fmt.Sprintf("%-20s %-30s", "SERVICE", "DESCRIPTION")) + "\n")

	for i := start; i < end; i++ {
		service := services[i]
		desc := ""
		switch service {
		case "EC2":
			desc = "Instances, details, SSM"
		case "S3":
			desc = "Buckets, objects, policies"
		case "EKS":
			desc = "Clusters, node groups, addons"
		case "ECS":
			desc = "Clusters, services, tasks"
		case "ECR":
			desc = "Repositories, images"
		case "Secrets":
			desc = "Secrets Manager"
		default:
			desc = ""
		}

		row := fmt.Sprintf("%-20s %-30s", service, desc)
		if i == m.serviceSelectedIndex {
			for len(row) < 52 {
				row += " "
			}
			selectedStyle := lipgloss.NewStyle().
				Background(lipgloss.Color("51")).
				Foreground(lipgloss.Color("0")).
				Bold(true)
			content.WriteString(selectedStyle.Render(row) + "\n")
		} else {
			normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
			content.WriteString(normalStyle.Render(row) + "\n")
		}
	}

	// Add scroll indicators
	if start > 0 || end < len(services) {
		scrollInfo := fmt.Sprintf("\n[Showing %d-%d of %d | Use j/k or ↓/↑ to navigate]", start+1, end, len(services))
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(scrollInfo))
	}

	content.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Press Enter to open a service"))

	return content.String()
}

func (m model) renderRegionSelection() string {
	title := lipgloss.NewStyle().Bold(true).Render("AWS Region Selection")

	if m.config == nil || len(m.config.Regions) == 0 {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No regions configured")
	}

	regions := m.config.Regions

	// Ensure selected item is visible and get viewport range
	m.ensureVisible(m.regionSelectedIndex, len(regions))
	start, end := m.getVisibleRange(len(regions))

	// Build table header (k9s style)
	var content strings.Builder

	// Title with count - k9s style
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true) // Cyan
	tableTitle := fmt.Sprintf("AWS-Regions[%d]", len(regions))
	titleText := titleStyle.Render(tableTitle)

	// Center the title with dashes on both sides
	titleWidth := len(tableTitle)
	totalWidth := 80
	dashesWidth := (totalWidth - titleWidth - 2) / 2
	if dashesWidth < 1 {
		dashesWidth = 1
	}

	content.WriteString(strings.Repeat("─", dashesWidth) + " ")
	content.WriteString(titleText)
	content.WriteString(" " + strings.Repeat("─", dashesWidth) + "\n")

	// Table header - k9s uses uppercase and symbols
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Underline(true)
	content.WriteString(headerStyle.Render(fmt.Sprintf("%-30s %-40s",
		"REGION", "STATUS")) + "\n")

	// Build table rows (only visible items)
	for i := start; i < end; i++ {
		region := regions[i]

		// Show current region indicator
		status := ""
		if region == m.config.Region {
			status = "✓ CURRENT"
		}

		// Build row with proper spacing
		row := fmt.Sprintf("%-30s %-40s",
			region,
			status,
		)

		if i == m.regionSelectedIndex {
			// Highlight the selected row - k9s style with cyan background
			for len(row) < 70 {
				row += " "
			}
			selectedStyle := lipgloss.NewStyle().
				Background(lipgloss.Color("51")).
				Foreground(lipgloss.Color("0")).
				Bold(true)
			content.WriteString(selectedStyle.Render(row) + "\n")
		} else {
			// Normal row
			normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
			content.WriteString(normalStyle.Render(row) + "\n")
		}
	}

	// Add scroll indicators
	if start > 0 || end < len(regions) {
		scrollInfo := fmt.Sprintf("\n[Showing %d-%d of %d | Use j/k or ↓/↑ to navigate | Enter to select | ESC to cancel]", start+1, end, len(regions))
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(scrollInfo))
	} else {
		helpText := "\n[Use j/k or ↓/↑ to navigate | Enter to select | ESC to cancel]"
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(helpText))
	}

	return content.String()
}

func (m model) renderEC2() string {
	title := lipgloss.NewStyle().Bold(true).Render("EC2 Instances")
	if m.filter != "" {
		title += lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf(" (filtered by: %s)", m.filter))
	}
	if m.vimState.LastSearch != "" {
		title += lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(fmt.Sprintf(" [search: %s]", m.vimState.LastSearch))
	}

	if m.loading {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("Loading instances...")
	}

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		return title + "\n\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	// Use filtered instances if VIM search is active, otherwise use legacy filter or all instances
	var filteredInstances []aws.Instance
	if len(m.ec2.FilteredInstances) > 0 {
		filteredInstances = m.ec2.FilteredInstances
	} else if m.vimState.LastSearch != "" {
		// Search is active but no results
		filteredInstances = []aws.Instance{}
	} else if m.filter == "" {
		filteredInstances = m.ec2.Instances
	} else {
		if strings.Contains(m.filter, "=") {
			parts := strings.SplitN(m.filter, "=", 2)
			tagKey := parts[0]
			tagValue := parts[1]
			for _, inst := range m.ec2.Instances {
				for _, tag := range inst.Tags {
					if tag.Key == tagKey && strings.Contains(strings.ToLower(tag.Value), strings.ToLower(tagValue)) {
						filteredInstances = append(filteredInstances, inst)
						break
					}
				}
			}
		} else {
			for _, inst := range m.ec2.Instances {
				if strings.Contains(strings.ToLower(inst.State), strings.ToLower(m.filter)) || strings.Contains(strings.ToLower(inst.Name), strings.ToLower(m.filter)) || strings.Contains(strings.ToLower(inst.ID), strings.ToLower(m.filter)) {
					filteredInstances = append(filteredInstances, inst)
				}
			}
		}
	}

	if len(filteredInstances) == 0 {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No instances found")
	}

	// Ensure selected item is visible and get viewport range
	m.ensureVisible(m.ec2.SelectedIndex, len(filteredInstances))
	start, end := m.getVisibleRange(len(filteredInstances))

	// Build table header (k9s style)
	var content strings.Builder

	// Title with count - k9s style
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true) // Cyan
	searchInfo := ""
	if m.vimState.LastSearch != "" {
		searchInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Render("(" + m.vimState.LastSearch + ")")
	}
	tableTitle := fmt.Sprintf("EC2-Instances%s[%d]", searchInfo, len(filteredInstances))
	titleText := titleStyle.Render(tableTitle)

	// Center the title with dashes on both sides
	// Use actual string width not ANSI-coded width
	titleWidth := len(tableTitle)                    // Visual width without ANSI codes
	totalWidth := 100                                // Reasonable fixed width for the table
	dashesWidth := (totalWidth - titleWidth - 2) / 2 // -2 for spaces around title
	if dashesWidth < 1 {
		dashesWidth = 1
	}

	content.WriteString(strings.Repeat("─", dashesWidth) + " ")
	content.WriteString(titleText)
	content.WriteString(" " + strings.Repeat("─", dashesWidth) + "\n")

	// Table header - k9s uses uppercase and symbols
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Underline(true)
	content.WriteString(headerStyle.Render(fmt.Sprintf("%-1s  %-20s %-30s %-15s %-15s %-15s",
		"✓", "INSTANCE ID", "NAME", "STATE", "TYPE", "IP")) + "\n")

	// Build table rows (only visible items)
	for i := start; i < end; i++ {
		inst := filteredInstances[i]
		name := inst.Name
		if name == "" {
			name = "-" // Don't use lipgloss styling - it breaks alignment
		}

		ip := inst.PublicIP
		if ip == "" {
			ip = inst.PrivateIP
		}
		if ip == "" {
			ip = "-"
		}

		// Check if instance is selected for bulk action
		checkmarkStr := " "
		if m.ec2.SelectedInstances[inst.ID] {
			checkmarkStr = "✓"
		}

		// Build row with proper spacing (format first, then apply colors to specific fields)
		// Don't use styled strings in sprintf as ANSI codes break alignment
		row := fmt.Sprintf("%-1s  %-20s %-30s %-15s %-15s %-15s",
			checkmarkStr,
			inst.ID,
			uiShared.Truncate(name, 30),
			inst.State,
			inst.InstanceType,
			ip,
		)

		// Apply state color to the state field within the row
		// (we'll handle this differently to maintain alignment)

		if i == m.ec2.SelectedIndex {
			// Highlight the selected row - k9s style with cyan background
			// Ensure row is padded to exactly 98 characters (the column widths sum)
			// This prevents any terminal interpretation issues
			for len(row) < 98 {
				row += " "
			}
			// Use ANSI codes directly to avoid lipgloss adding extra width
			// \x1b[48;5;51m = cyan background, \x1b[38;5;0m = black foreground, \x1b[1m = bold, \x1b[0m = reset
			row = "\x1b[48;5;51m\x1b[38;5;0m\x1b[1m" + row + "\x1b[0m"
		}

		content.WriteString(row + "\n")
	}

	selectedCount := len(m.ec2.SelectedInstances)

	// Show scroll position and total
	scrollInfo := fmt.Sprintf("\nShowing %d-%d of %d instances", start+1, end, len(filteredInstances))
	content.WriteString(scrollInfo)

	if selectedCount > 0 {
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(fmt.Sprintf(" | Selected: %d", selectedCount)))
	}
	if m.autoRefresh {
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(" | Auto-refresh: ON"))
	}

	return content.String()
}

func (m model) renderEC2Details() string {
	title := lipgloss.NewStyle().Bold(true).Render("EC2 Instance Details")

	if m.loading {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("Loading instance details...")
	}

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		return title + "\n\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	if m.ec2.InstanceDetails == nil {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No instance details available")
	}

	details := m.ec2.InstanceDetails
	var content strings.Builder
	content.WriteString(title + "\n\n")

	// Section styling
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	valueStyle := lipgloss.NewStyle()

	// Basic Information
	content.WriteString(sectionStyle.Render("Basic Information") + "\n")
	content.WriteString(labelStyle.Render("  Instance ID:     ") + valueStyle.Render(details.ID) + "\n")
	content.WriteString(labelStyle.Render("  Name:            ") + valueStyle.Render(details.Name) + "\n")
	content.WriteString(labelStyle.Render("  State:           ") + getStateStyle(details.State).Render(details.State) + "\n")
	content.WriteString(labelStyle.Render("  Instance Type:   ") + valueStyle.Render(details.InstanceType) + "\n")
	content.WriteString(labelStyle.Render("  Architecture:    ") + valueStyle.Render(details.Architecture) + "\n")
	if details.Platform != "" {
		content.WriteString(labelStyle.Render("  Platform:        ") + valueStyle.Render(details.Platform) + "\n")
	}
	if details.LaunchTime != "" {
		content.WriteString(labelStyle.Render("  Launch Time:     ") + valueStyle.Render(details.LaunchTime) + "\n")
	}
	if details.KeyName != "" {
		content.WriteString(labelStyle.Render("  Key Name:        ") + valueStyle.Render(details.KeyName) + "\n")
	}
	content.WriteString("\n")

	// Instance Type Specifications
	if details.InstanceTypeInfo != nil {
		typeInfo := details.InstanceTypeInfo
		content.WriteString(sectionStyle.Render("Instance Type Specifications") + "\n")
		if typeInfo.VCpus > 0 {
			content.WriteString(labelStyle.Render("  vCPUs:           ") + valueStyle.Render(fmt.Sprintf("%d", typeInfo.VCpus)) + "\n")
		}
		if typeInfo.Memory > 0 {
			memoryGB := float64(typeInfo.Memory) / 1024.0
			content.WriteString(labelStyle.Render("  Memory:          ") + valueStyle.Render(fmt.Sprintf("%.2f GiB", memoryGB)) + "\n")
		}
		if typeInfo.NetworkPerformance != "" {
			content.WriteString(labelStyle.Render("  Network:         ") + valueStyle.Render(typeInfo.NetworkPerformance) + "\n")
		}
		if typeInfo.StorageType != "" {
			storageInfo := typeInfo.StorageType
			if typeInfo.InstanceStorageGB > 0 {
				storageInfo += fmt.Sprintf(" (%d GB)", typeInfo.InstanceStorageGB)
			}
			content.WriteString(labelStyle.Render("  Storage:         ") + valueStyle.Render(storageInfo) + "\n")
		}
		if typeInfo.EbsOptimized {
			content.WriteString(labelStyle.Render("  EBS Optimized:   ") + valueStyle.Render("Yes") + "\n")
		}
		content.WriteString("\n")
	}

	// Network Information
	content.WriteString(sectionStyle.Render("Network Information") + "\n")
	content.WriteString(labelStyle.Render("  VPC ID:          ") + valueStyle.Render(details.VpcID) + "\n")
	content.WriteString(labelStyle.Render("  Subnet ID:       ") + valueStyle.Render(details.SubnetID) + "\n")
	content.WriteString(labelStyle.Render("  Availability Zone: ") + valueStyle.Render(details.AZ) + "\n")
	if details.PublicIP != "" {
		content.WriteString(labelStyle.Render("  Public IP:       ") + valueStyle.Render(details.PublicIP) + "\n")
	}
	if details.PrivateIP != "" {
		content.WriteString(labelStyle.Render("  Private IP:      ") + valueStyle.Render(details.PrivateIP) + "\n")
	}
	content.WriteString("\n")

	// Security Groups
	if len(details.SecurityGroups) > 0 {
		content.WriteString(sectionStyle.Render("Security Groups") + "\n")
		for _, sg := range details.SecurityGroups {
			content.WriteString(labelStyle.Render("  • ") + valueStyle.Render(fmt.Sprintf("%s (%s)", sg.Name, sg.ID)) + "\n")
		}
		content.WriteString("\n")
	}

	// Block Devices
	if len(details.BlockDevices) > 0 {
		content.WriteString(sectionStyle.Render("Block Devices") + "\n")
		for _, bd := range details.BlockDevices {
			volumeInfo := bd.VolumeID
			if bd.VolumeSize > 0 {
				volumeInfo += fmt.Sprintf(" (%d GB, %s)", bd.VolumeSize, bd.VolumeType)
			}
			if bd.DeleteOnTermination {
				volumeInfo += " [Delete on Termination]"
			}
			content.WriteString(labelStyle.Render(fmt.Sprintf("  %s: ", bd.DeviceName)) + valueStyle.Render(volumeInfo) + "\n")
		}
		content.WriteString("\n")
	}

	// Network Interfaces
	if len(details.NetworkInterfaces) > 0 {
		content.WriteString(sectionStyle.Render("Network Interfaces") + "\n")
		for i, ni := range details.NetworkInterfaces {
			content.WriteString(labelStyle.Render(fmt.Sprintf("  Interface %d:\n", i+1)))
			content.WriteString(labelStyle.Render("    ID:           ") + valueStyle.Render(ni.ID) + "\n")
			content.WriteString(labelStyle.Render("    MAC Address:  ") + valueStyle.Render(ni.MacAddress) + "\n")
			content.WriteString(labelStyle.Render("    Private IP:   ") + valueStyle.Render(ni.PrivateIP) + "\n")
			if ni.PublicIP != "" {
				content.WriteString(labelStyle.Render("    Public IP:    ") + valueStyle.Render(ni.PublicIP) + "\n")
			}
			content.WriteString(labelStyle.Render("    Subnet:       ") + valueStyle.Render(ni.SubnetID) + "\n")
			if len(ni.SecurityGroups) > 0 {
				content.WriteString(labelStyle.Render("    Security Groups: "))
				sgNames := make([]string, len(ni.SecurityGroups))
				for j, sg := range ni.SecurityGroups {
					sgNames[j] = sg.Name
				}
				content.WriteString(valueStyle.Render(strings.Join(sgNames, ", ")) + "\n")
			}
			content.WriteString("\n")
		}
	}

	// Health Status
	if m.ec2.InstanceStatus != nil {
		content.WriteString(sectionStyle.Render("Health Status") + "\n")

		// System status
		systemStatusColor := lipgloss.Color("1") // Red by default
		if m.ec2.InstanceStatus.SystemStatusOk {
			systemStatusColor = lipgloss.Color("2") // Green
		}
		systemStatusStyle := lipgloss.NewStyle().Foreground(systemStatusColor)
		content.WriteString(labelStyle.Render("  System Status:   ") +
			systemStatusStyle.Render(m.ec2.InstanceStatus.SystemStatus) + "\n")

		// Instance status
		instanceStatusColor := lipgloss.Color("1") // Red by default
		if m.ec2.InstanceStatus.InstanceStatusOk {
			instanceStatusColor = lipgloss.Color("2") // Green
		}
		instanceStatusStyle := lipgloss.NewStyle().Foreground(instanceStatusColor)
		content.WriteString(labelStyle.Render("  Instance Status: ") +
			instanceStatusStyle.Render(m.ec2.InstanceStatus.InstanceStatus) + "\n")

		// Scheduled events
		if len(m.ec2.InstanceStatus.ScheduledEvents) > 0 {
			content.WriteString(labelStyle.Render("  Scheduled Events:\n"))
			for _, event := range m.ec2.InstanceStatus.ScheduledEvents {
				content.WriteString(labelStyle.Render(fmt.Sprintf("    • %s: %s\n",
					event.Code, event.Description)))
				if event.NotBefore != "" {
					content.WriteString(labelStyle.Render(fmt.Sprintf("      Start: %s\n",
						event.NotBefore)))
				}
			}
		}
		content.WriteString("\n")
	}

	// CloudWatch Metrics
	if m.ec2.InstanceMetrics != nil {
		content.WriteString(sectionStyle.Render("CloudWatch Metrics (Last 5 Minutes)") + "\n")
		content.WriteString(labelStyle.Render("  CPU Utilization: ") +
			valueStyle.Render(fmt.Sprintf("%.2f%%", m.ec2.InstanceMetrics.CPUUtilization)) + "\n")
		content.WriteString(labelStyle.Render("  Network In:      ") +
			valueStyle.Render(fmt.Sprintf("%.2f MB", m.ec2.InstanceMetrics.NetworkIn/1024/1024)) + "\n")
		content.WriteString(labelStyle.Render("  Network Out:     ") +
			valueStyle.Render(fmt.Sprintf("%.2f MB", m.ec2.InstanceMetrics.NetworkOut/1024/1024)) + "\n")
		if m.ec2.InstanceMetrics.DiskReadBytes > 0 || m.ec2.InstanceMetrics.DiskWriteBytes > 0 {
			content.WriteString(labelStyle.Render("  Disk Read:       ") +
				valueStyle.Render(fmt.Sprintf("%.2f MB", m.ec2.InstanceMetrics.DiskReadBytes/1024/1024)) + "\n")
			content.WriteString(labelStyle.Render("  Disk Write:      ") +
				valueStyle.Render(fmt.Sprintf("%.2f MB", m.ec2.InstanceMetrics.DiskWriteBytes/1024/1024)) + "\n")
		}
		content.WriteString("\n")
	}

	// SSM Connectivity
	if m.ec2.SSMStatus != nil {
		content.WriteString(sectionStyle.Render("Systems Manager (SSM)") + "\n")
		if m.ec2.SSMStatus.Connected {
			connectStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
			content.WriteString(labelStyle.Render("  Status:          ") +
				connectStyle.Render("Connected") + "\n")
			content.WriteString(labelStyle.Render("  Ping Status:     ") +
				valueStyle.Render(m.ec2.SSMStatus.PingStatus) + "\n")
			if m.ec2.SSMStatus.AgentVersion != "" {
				content.WriteString(labelStyle.Render("  Agent Version:   ") +
					valueStyle.Render(m.ec2.SSMStatus.AgentVersion) + "\n")
			}
			if m.ec2.SSMStatus.PlatformName != "" {
				content.WriteString(labelStyle.Render("  Platform:        ") +
					valueStyle.Render(m.ec2.SSMStatus.PlatformName) + "\n")
			}
			if m.ec2.SSMStatus.LastPingTime != "" {
				content.WriteString(labelStyle.Render("  Last Ping:       ") +
					valueStyle.Render(m.ec2.SSMStatus.LastPingTime) + "\n")
			}
			// Add hint about connecting
			hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Italic(true)
			content.WriteString(labelStyle.Render("  ") +
				hintStyle.Render("Press 'C' to open SSM session in new terminal") + "\n")
		} else {
			disconnectStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
			content.WriteString(labelStyle.Render("  Status:          ") +
				disconnectStyle.Render("Not Connected") + "\n")
			content.WriteString(labelStyle.Render("  Note:            ") +
				valueStyle.Render("SSM agent may not be installed or configured") + "\n")
		}
		content.WriteString("\n")
	}

	// Additional Information
	content.WriteString(sectionStyle.Render("Additional Information") + "\n")
	content.WriteString(labelStyle.Render("  Root Device:     ") + valueStyle.Render(details.RootDeviceType) + "\n")
	if details.Monitoring != "" {
		content.WriteString(labelStyle.Render("  Monitoring:      ") + valueStyle.Render(details.Monitoring) + "\n")
	}
	if details.IamInstanceProfile != "" {
		content.WriteString(labelStyle.Render("  IAM Role:        ") + valueStyle.Render(details.IamInstanceProfile) + "\n")
	}
	content.WriteString("\n")

	// Tags
	if len(details.Tags) > 0 {
		content.WriteString(sectionStyle.Render("Tags") + "\n")
		for _, tag := range details.Tags {
			if tag.Key != "Name" { // Skip Name tag as it's already shown
				content.WriteString(labelStyle.Render(fmt.Sprintf("  %s: ", tag.Key)) + valueStyle.Render(tag.Value) + "\n")
			}
		}
	}

	return m.renderWithViewport(content.String())
}

func (m model) renderS3() string {
	title := lipgloss.NewStyle().Bold(true).Render("S3 Buckets")
	if m.vimState.LastSearch != "" {
		title += lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(fmt.Sprintf(" [search: %s]", m.vimState.LastSearch))
	}

	if m.loading {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("Loading buckets...")
	}

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		return title + "\n\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	// Use filtered buckets if VIM search is active
	buckets := m.s3.Buckets
	if len(m.s3.FilteredBuckets) > 0 {
		buckets = m.s3.FilteredBuckets
	} else if m.vimState.LastSearch != "" {
		// Search is active but no results
		buckets = []aws.Bucket{}
	}

	if len(buckets) == 0 {
		if m.vimState.LastSearch != "" {
			return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No buckets match your search")
		}
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No buckets found")
	}

	// Ensure selected item is visible and get viewport range
	m.ensureVisible(m.s3.SelectedBucketIndex, len(buckets))
	start, end := m.getVisibleRange(len(buckets))

	// Build table header (k9s style)
	var content strings.Builder

	// Title with count - k9s style
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true) // Cyan
	searchInfo := ""
	if m.vimState.LastSearch != "" {
		searchInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Render("(" + m.vimState.LastSearch + ")")
	}
	tableTitle := fmt.Sprintf("S3-Buckets%s[%d]", searchInfo, len(buckets))
	titleText := titleStyle.Render(tableTitle)

	// Center the title with dashes on both sides
	titleWidth := len(tableTitle)
	totalWidth := 100
	dashesWidth := (totalWidth - titleWidth - 2) / 2
	if dashesWidth < 1 {
		dashesWidth = 1
	}

	content.WriteString(strings.Repeat("─", dashesWidth) + " ")
	content.WriteString(titleText)
	content.WriteString(" " + strings.Repeat("─", dashesWidth) + "\n")

	// Table header
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Underline(true)
	content.WriteString(headerStyle.Render(fmt.Sprintf("%-40s %-25s %-20s",
		"BUCKET NAME", "CREATION DATE", "REGION")) + "\n")

	// Build table rows (only visible items)
	for i := start; i < end; i++ {
		bucket := buckets[i]
		creationDate := bucket.CreationDate
		if creationDate == "" {
			creationDate = "-"
		}

		region := bucket.Region
		if region == "" {
			region = "-"
		}

		// Highlight selected row
		row := fmt.Sprintf("%-40s %-25s %-20s",
			uiShared.Truncate(bucket.Name, 40),
			creationDate,
			region,
		)

		if i == m.s3.SelectedBucketIndex {
			// Highlight the selected row - k9s style with cyan background
			// Use ANSI codes directly to avoid lipgloss adding extra width
			// \x1b[K clears to end of line with background color
			row = "\x1b[48;5;51m\x1b[38;5;0m\x1b[1m" + row + "\x1b[K\x1b[0m"
		}

		content.WriteString(row + "\n")
	}

	// Show scroll position and total
	content.WriteString(fmt.Sprintf("\nShowing %d-%d of %d buckets", start+1, end, len(buckets)))

	return content.String()
}

func (m model) renderS3Browse() string {
	// Build breadcrumb
	breadcrumbStyle := lipgloss.NewStyle().Bold(true)
	bucketStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	separatorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	var breadcrumbs strings.Builder
	breadcrumbs.WriteString(bucketStyle.Render(m.s3.CurrentBucket))

	if m.s3.CurrentPrefix != "" {
		parts := strings.Split(strings.TrimSuffix(m.s3.CurrentPrefix, "/"), "/")
		for _, part := range parts {
			if part != "" {
				breadcrumbs.WriteString(separatorStyle.Render(" > "))
				breadcrumbs.WriteString(part)
			}
		}
	}

	title := breadcrumbStyle.Render("S3 Browser: ") + breadcrumbs.String()
	if m.vimState.LastSearch != "" {
		title += lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(fmt.Sprintf(" [search: %s]", m.vimState.LastSearch))
	}

	if m.loading {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("Loading objects...")
	}

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		return title + "\n\n" + errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	// Use filtered objects if VIM search is active
	objects := m.s3.Objects
	if len(m.s3.FilteredObjects) > 0 {
		objects = m.s3.FilteredObjects
	} else if m.vimState.LastSearch != "" {
		// Search is active but no results
		objects = []aws.S3Object{}
	}

	if len(objects) == 0 {
		if m.vimState.LastSearch != "" {
			return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No objects match your search")
		}
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No objects found (empty folder)")
	}

	// Ensure selected item is visible and get viewport range
	m.ensureVisible(m.s3.SelectedObjectIndex, len(objects))
	start, end := m.getVisibleRange(len(objects))

	// Build table header (k9s style)
	var content strings.Builder

	// Title with count - k9s style
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true) // Cyan
	searchInfo := ""
	if m.vimState.LastSearch != "" {
		searchInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Render("(" + m.vimState.LastSearch + ")")
	}

	// Breadcrumb for location
	bucketPath := m.s3.CurrentBucket
	if m.s3.CurrentPrefix != "" {
		bucketPath += "/" + strings.TrimSuffix(m.s3.CurrentPrefix, "/")
	}

	tableTitle := fmt.Sprintf("S3-Objects(%s)%s[%d]", bucketPath, searchInfo, len(objects))
	titleText := titleStyle.Render(tableTitle)

	// Center the title with dashes on both sides
	titleWidth := len(tableTitle)
	totalWidth := 100
	dashesWidth := (totalWidth - titleWidth - 2) / 2
	if dashesWidth < 1 {
		dashesWidth = 1
	}

	content.WriteString(strings.Repeat("─", dashesWidth) + " ")
	content.WriteString(titleText)
	content.WriteString(" " + strings.Repeat("─", dashesWidth) + "\n")

	// Table header
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Underline(true)
	content.WriteString(headerStyle.Render(fmt.Sprintf("%-6s %-50s %-15s %-25s %-20s",
		"TYPE", "NAME", "SIZE", "LAST MODIFIED", "STORAGE CLASS")) + "\n")

	// Build table rows (only visible items)
	for i := start; i < end; i++ {
		obj := objects[i]
		var typeIcon, name, size, lastModified, storageClass string

		if obj.IsFolder {
			typeIcon = "DIR"
			// Extract folder name from full key
			folderName := strings.TrimSuffix(obj.Key, "/")
			if m.s3.CurrentPrefix != "" {
				folderName = strings.TrimPrefix(folderName, m.s3.CurrentPrefix)
			}
			name = folderName + "/"
			size = "-"
			lastModified = "-"
			storageClass = "-"
		} else {
			typeIcon = "FILE"
			// Extract file name from full key
			fileName := obj.Key
			if m.s3.CurrentPrefix != "" {
				fileName = strings.TrimPrefix(fileName, m.s3.CurrentPrefix)
			}
			name = fileName
			size = formatBytes(obj.Size)
			lastModified = obj.LastModified
			storageClass = obj.StorageClass
		}

		// Style for folders
		if obj.IsFolder {
			typeIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Render(typeIcon)
			name = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true).Render(name)
		}

		// Highlight selected row
		row := fmt.Sprintf("%-6s %-50s %-15s %-25s %-20s",
			typeIcon,
			uiShared.Truncate(name, 50),
			size,
			lastModified,
			uiShared.Truncate(storageClass, 20),
		)

		if i == m.s3.SelectedObjectIndex {
			// Highlight the selected row - k9s style with cyan background
			// Use ANSI codes directly to avoid lipgloss adding extra width
			// \x1b[K clears to end of line with background color
			row = "\x1b[48;5;51m\x1b[38;5;0m\x1b[1m" + row + "\x1b[K\x1b[0m"
		}

		content.WriteString(row + "\n")
	}

	// Pagination and scroll info
	content.WriteString(fmt.Sprintf("\nShowing %d-%d of %d objects", start+1, end, len(objects)))
	if m.s3.IsTruncated {
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(" (more available - press 'n' for next page)"))
	}

	return content.String()
}

func (m model) renderECS() string {
	title := lipgloss.NewStyle().Bold(true).Render("ECS Clusters")
	if m.vimState.LastSearch != "" {
		title += lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(fmt.Sprintf(" [search: %s]", m.vimState.LastSearch))
	}

	if m.loading {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("Loading ECS clusters...")
	}
	if m.err != nil {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
	}

	clusters := m.ecs.Clusters
	if len(m.ecs.Filtered) > 0 {
		clusters = m.ecs.Filtered
	} else if m.vimState.LastSearch != "" {
		clusters = []aws.ECSCluster{}
	}

	if len(clusters) == 0 {
		if m.vimState.LastSearch != "" {
			return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No clusters match your search")
		}
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No ECS clusters found")
	}

	m.ensureVisible(m.ecs.SelectedIndex, len(clusters))
	startIdx, endIdx := m.getVisibleRange(len(clusters))

	var content strings.Builder
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true)
	searchInfo := ""
	if m.vimState.LastSearch != "" {
		searchInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Render("(" + m.vimState.LastSearch + ")")
	}
	tableTitle := fmt.Sprintf("ECS-Clusters%s[%d]", searchInfo, len(clusters))
	titleText := titleStyle.Render(tableTitle)
	totalWidth := 80
	dashesWidth := (totalWidth - len(tableTitle) - 2) / 2
	if dashesWidth < 1 {
		dashesWidth = 1
	}
	content.WriteString(strings.Repeat("─", dashesWidth) + " ")
	content.WriteString(titleText)
	content.WriteString(" " + strings.Repeat("─", dashesWidth) + "\n")

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Underline(true)
	content.WriteString(headerStyle.Render(fmt.Sprintf("%-30s %-10s %-10s %-10s %-10s", "CLUSTER", "STATUS", "RUN", "PEND", "SERV")) + "\n")

	for i := startIdx; i < endIdx; i++ {
		cl := clusters[i]
		row := fmt.Sprintf("%-30s %-10s %-10d %-10d %-10d",
			uiShared.Truncate(cl.Name, 30),
			cl.Status,
			cl.RunningTasksCount,
			cl.PendingTasksCount,
			cl.ActiveServicesCount,
		)
		if i == m.ecs.SelectedIndex {
			row = "\x1b[48;5;51m\x1b[38;5;0m\x1b[1m" + row + "\x1b[K\x1b[0m"
		}
		content.WriteString(row + "\n")
	}

	content.WriteString(fmt.Sprintf("\nShowing %d-%d of %d clusters", startIdx+1, endIdx, len(clusters)))

	return content.String()
}

func (m model) renderECSServices() string {
	title := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("ECS Services - %s", m.ecs.CurrentCluster))
	if m.loading {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("Loading ECS services...")
	}
	if m.err != nil {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
	}
	services := m.ecs.Services
	if len(m.ecs.FilteredSvcs) > 0 {
		services = m.ecs.FilteredSvcs
	} else if m.vimState.LastSearch != "" {
		services = []aws.ECSService{}
	}
	if len(services) == 0 {
		emptyMsg := "No ECS services found"
		if m.vimState.LastSearch != "" {
			emptyMsg = "No services match your search"
		}
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(emptyMsg)
	}

	m.ensureVisible(m.ecs.SelectedSvc, len(services))
	start, end := m.getVisibleRange(len(services))

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Underline(true)
	var b strings.Builder
	b.WriteString(title + "\n\n")
	b.WriteString(headerStyle.Render(fmt.Sprintf("%-28s %-10s %-8s %-8s %-8s %-10s %-25s", "SERVICE", "STATUS", "DESIRED", "RUN", "PEND", "LAUNCH", "TASK DEF")) + "\n")
	for i := start; i < end; i++ {
		svc := services[i]
		row := fmt.Sprintf("%-28s %-10s %-8d %-8d %-8d %-10s %-25s",
			uiShared.Truncate(svc.Name, 28),
			svc.Status,
			svc.DesiredCount,
			svc.RunningCount,
			svc.PendingCount,
			uiShared.Truncate(svc.LaunchType, 10),
			uiShared.Truncate(svc.TaskDefinition, 25),
		)
		if i == m.ecs.SelectedSvc {
			row = "\x1b[48;5;51m\x1b[38;5;0m\x1b[1m" + row + "\x1b[K\x1b[0m"
		}
		b.WriteString(row + "\n")
	}
	b.WriteString(fmt.Sprintf("\nShowing %d-%d of %d services", start+1, end, len(services)))

	// Selected service health summary
	if len(services) > 0 && m.ecs.SelectedSvc < len(services) {
		svc := services[m.ecs.SelectedSvc]
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Service Health") + "\n")
		b.WriteString(fmt.Sprintf("Status: %s | Desired: %d | Running: %d | Pending: %d | Grace: %ds\n",
			svc.Status, svc.DesiredCount, svc.RunningCount, svc.PendingCount, svc.HealthCheckGracePeriodSecs))
		if len(svc.Deployments) > 0 {
			b.WriteString("Deployments:\n")
			for _, dep := range svc.Deployments {
				created := "-"
				if dep.Created != nil {
					created = dep.Created.Format("2006-01-02 15:04:05")
				}
				b.WriteString(fmt.Sprintf(" - %s | Desired: %d | Running: %d | Pending: %d | Created: %s\n",
					dep.Status, dep.Desired, dep.Running, dep.Pending, created))
			}
		}
		if len(svc.Events) > 0 {
			b.WriteString("Recent Events:\n")
			for _, ev := range svc.Events {
				when := "-"
				if ev.When != nil {
					when = ev.When.Format("2006-01-02 15:04:05")
				}
				b.WriteString(fmt.Sprintf(" - %s (%s)\n", ev.Message, when))
			}
		}
	}

	return b.String()
}

func (m model) renderECSTasks() string {
	title := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("ECS Tasks - %s / %s", m.ecs.CurrentCluster, m.ecs.CurrentService))
	if m.loading {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("Loading ECS tasks...")
	}
	if m.err != nil {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
	}

	tasks := m.ecs.Tasks
	if len(m.ecs.FilteredTasks) > 0 {
		tasks = m.ecs.FilteredTasks
	} else if m.vimState.LastSearch != "" {
		tasks = []aws.ECSTask{}
	}

	if len(tasks) == 0 {
		msg := "No tasks found for this service"
		if m.vimState.LastSearch != "" {
			msg = "No tasks match your search"
		}
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(msg)
	}

	m.ensureVisible(m.ecs.SelectedTask, len(tasks))
	start, end := m.getVisibleRange(len(tasks))

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Underline(true)
	var b strings.Builder
	b.WriteString(title + "\n\n")
	b.WriteString(headerStyle.Render(fmt.Sprintf("%-34s %-10s %-10s %-8s %-8s %-10s %-10s", "TASK", "STATUS", "HEALTH", "CPU", "MEM", "LAUNCH", "AZ")) + "\n")
	for i := start; i < end; i++ {
		t := tasks[i]
		row := fmt.Sprintf("%-34s %-10s %-10s %-8s %-8s %-10s %-10s",
			uiShared.Truncate(t.ID, 34),
			t.Status,
			t.Health,
			uiShared.Truncate(t.CPU, 8),
			uiShared.Truncate(t.Memory, 8),
			uiShared.Truncate(t.LaunchType, 10),
			uiShared.Truncate(t.AvailabilityZone, 10),
		)
		if i == m.ecs.SelectedTask {
			row = "\x1b[48;5;51m\x1b[38;5;0m\x1b[1m" + row + "\x1b[K\x1b[0m"
		}
		b.WriteString(row + "\n")
	}
	b.WriteString(fmt.Sprintf("\nShowing %d-%d of %d tasks", start+1, end, len(tasks)))

	// Quick container summary for selected task
	if len(tasks) > 0 && m.ecs.SelectedTask < len(tasks) {
		task := tasks[m.ecs.SelectedTask]
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Selected Task") + "\n")
		b.WriteString(fmt.Sprintf("TaskDef: %s | Status: %s | Health: %s | CPU/Mem: %s/%s\n",
			task.TaskDefinition, task.Status, task.Health, task.CPU, task.Memory))
		if len(task.Containers) > 0 {
			b.WriteString("Containers:\n")
			for _, c := range task.Containers {
				b.WriteString(fmt.Sprintf(" - %s: %s (Health: %s)\n", c.Name, c.LastStatus, c.HealthStatus))
			}
		}
	}

	return b.String()
}

func (m model) renderECSTaskDetails() string {
	title := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("ECS Task Details - %s / %s", m.ecs.CurrentCluster, m.ecs.CurrentService))
	var b strings.Builder
	b.WriteString(title + "\n\n")

	tasks := m.ecs.Tasks
	if len(m.ecs.FilteredTasks) > 0 {
		tasks = m.ecs.FilteredTasks
	}
	if len(tasks) == 0 || m.ecs.SelectedTask >= len(tasks) {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No task selected"))
		return b.String()
	}

	task := tasks[m.ecs.SelectedTask]
	b.WriteString(fmt.Sprintf("Task: %s\nStatus: %s | Health: %s\nTaskDef: %s\nLaunch: %s | AZ: %s\nCPU/Memory: %s/%s\nCreated: %s | Started: %s\n",
		task.ID,
		task.Status,
		task.Health,
		task.TaskDefinition,
		task.LaunchType,
		task.AvailabilityZone,
		task.CPU,
		task.Memory,
		formatTime(task.CreatedAt),
		formatTime(task.StartedAt),
	))

	if len(task.Attachments) > 0 {
		b.WriteString("\nNetwork:\n")
		for _, att := range task.Attachments {
			b.WriteString(fmt.Sprintf(" - %s\n", att.Type))
			for k, v := range att.Details {
				b.WriteString(fmt.Sprintf("    %s: %s\n", k, v))
			}
		}
	}

	if len(task.Containers) > 0 {
		b.WriteString("\nContainers:\n")
		for _, c := range task.Containers {
			b.WriteString(fmt.Sprintf(" - %s: %s (Health: %s)\n", c.Name, c.LastStatus, c.HealthStatus))
			if len(c.PrivateIPs) > 0 {
				b.WriteString(fmt.Sprintf("    IPs: %s\n", strings.Join(c.PrivateIPs, ", ")))
			}
			if len(c.Ports) > 0 {
				var ports []string
				for _, p := range c.Ports {
					ports = append(ports, fmt.Sprintf("%d:%d %s", p.HostPort, p.ContainerPort, p.Protocol))
				}
				b.WriteString(fmt.Sprintf("    Ports: %s\n", strings.Join(ports, ", ")))
			}
		}
	}

	b.WriteString("\nLogs:\n")
	if m.loading && len(m.ecs.TaskLogs) == 0 {
		b.WriteString(" Loading logs...\n")
	} else if m.err != nil {
		b.WriteString(fmt.Sprintf(" Error loading logs: %v\n", m.err))
	} else if len(m.ecs.TaskLogs) == 0 {
		b.WriteString(" No log streams found for this task\n")
	} else {
		for _, stream := range m.ecs.TaskLogs {
			b.WriteString(fmt.Sprintf(" [%s] %s (%s)\n", stream.Container, stream.LogStream, stream.LogGroup))
			if len(stream.Events) == 0 {
				b.WriteString("   (no events)\n")
				continue
			}
			for _, ev := range stream.Events {
				b.WriteString(fmt.Sprintf("   [%s] %s\n", ev.Timestamp.Format("2006-01-02 15:04:05"), ev.Message))
			}
		}
	}

	return m.renderWithViewport(b.String())
}

func (m model) renderECRRepositories() string {
	title := lipgloss.NewStyle().Bold(true).Render("ECR Repositories")
	if m.loading {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("Loading ECR repositories...")
	}
	if m.err != nil {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
	}

	repos := m.ecr.Repositories
	if len(m.ecr.FilteredRepos) > 0 {
		repos = m.ecr.FilteredRepos
	} else if m.vimState.LastSearch != "" {
		repos = []aws.ECRRepository{}
	}

	if len(repos) == 0 {
		msg := "No ECR repositories found"
		if m.vimState.LastSearch != "" {
			msg = "No repositories match your search"
		}
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(msg)
	}

	m.ensureVisible(m.ecr.SelectedRepoIndex, len(repos))
	start, end := m.getVisibleRange(len(repos))

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Underline(true)
	var b strings.Builder
	b.WriteString(title + "\n\n")
	b.WriteString(headerStyle.Render(fmt.Sprintf("%-26s %-36s %-10s %-8s", "REPOSITORY", "URI", "SCAN", "TAGS")) + "\n")
	for i := start; i < end; i++ {
		repo := repos[i]
		scan := "off"
		if repo.ScanOnPush {
			scan = "on"
		}
		row := fmt.Sprintf("%-26s %-36s %-10s %-8s",
			uiShared.Truncate(repo.Name, 26),
			uiShared.Truncate(repo.URI, 36),
			scan,
			uiShared.Truncate(repo.TagMutability, 8),
		)
		if i == m.ecr.SelectedRepoIndex {
			row = "\x1b[48;5;51m\x1b[38;5;0m\x1b[1m" + row + "\x1b[K\x1b[0m"
		}
		b.WriteString(row + "\n")
	}
	b.WriteString(fmt.Sprintf("\nShowing %d-%d of %d repositories", start+1, end, len(repos)))

	// Details for selected repo
	if len(repos) > 0 && m.ecr.SelectedRepoIndex < len(repos) {
		repo := repos[m.ecr.SelectedRepoIndex]
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Repository Details") + "\n")
		b.WriteString(fmt.Sprintf("Name: %s\nURI: %s\nCreated: %s\nRegistry: %s\nScan on Push: %t\nTag Mutability: %s\nEncryption: %s %s\n",
			repo.Name,
			repo.URI,
			formatTime(repo.CreatedAt),
			repo.RegistryID,
			repo.ScanOnPush,
			repo.TagMutability,
			repo.EncryptionType,
			repo.KMSKey))
		if repo.PolicyText != "" {
			b.WriteString("\nPolicy:\n")
			b.WriteString(m.renderWithViewport(repo.PolicyText))
		}
		if repo.LifecyclePolicy != "" {
			b.WriteString("\nLifecycle Policy:\n")
			b.WriteString(repo.LifecyclePolicy + "\n")
		}
	}

	return m.renderWithViewport(b.String())
}

func (m model) renderECRImages() string {
	title := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("ECR Images - %s", m.ecr.CurrentRepo))
	if m.loading {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("Loading ECR images...")
	}
	if m.err != nil {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
	}

	images := m.ecr.Images
	if len(m.ecr.FilteredImages) > 0 {
		images = m.ecr.FilteredImages
	} else if m.vimState.LastSearch != "" {
		images = []aws.ECRImage{}
	}

	if len(images) == 0 {
		msg := "No images found for this repository"
		if m.vimState.LastSearch != "" {
			msg = "No images match your search"
		}
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(msg)
	}

	m.ensureVisible(m.ecr.SelectedImage, len(images))
	start, end := m.getVisibleRange(len(images))

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Underline(true)
	var b strings.Builder
	b.WriteString(title + "\n\n")
	b.WriteString(headerStyle.Render(fmt.Sprintf("%-15s %-25s %-10s %-18s %-12s", "DIGEST", "TAGS", "SIZE", "PUSHED", "MANIFEST")) + "\n")
	for i := start; i < end; i++ {
		img := images[i]
		digest := img.Digest
		if len(digest) > 12 {
			digest = digest[len(digest)-12:]
		}
		tags := "<untagged>"
		if len(img.Tags) > 0 {
			tags = strings.Join(img.Tags, ",")
		}
		row := fmt.Sprintf("%-15s %-25s %-10s %-18s %-12s",
			digest,
			uiShared.Truncate(tags, 25),
			formatBytes(img.SizeBytes),
			formatTime(img.PushedAt),
			uiShared.Truncate(img.ManifestType, 12),
		)
		if i == m.ecr.SelectedImage {
			row = "\x1b[48;5;51m\x1b[38;5;0m\x1b[1m" + row + "\x1b[K\x1b[0m"
		}
		b.WriteString(row + "\n")
	}
	b.WriteString(fmt.Sprintf("\nShowing %d-%d of %d images", start+1, end, len(images)))

	// Quick details
	if len(images) > 0 && m.ecr.SelectedImage < len(images) {
		img := images[m.ecr.SelectedImage]
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Selected Image") + "\n")
		tags := "<untagged>"
		if len(img.Tags) > 0 {
			tags = strings.Join(img.Tags, ",")
		}
		b.WriteString(fmt.Sprintf("Digest: %s\nTags: %s\nSize: %s\nPushed: %s\nManifest: %s\n",
			img.Digest,
			tags,
			formatBytes(img.SizeBytes),
			formatTime(img.PushedAt),
			img.ManifestType))
	}

	return b.String()
}

func (m model) renderECRImageDetails() string {
	title := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("ECR Image Details - %s", m.ecr.CurrentRepo))
	var b strings.Builder
	b.WriteString(title + "\n\n")

	images := m.ecr.Images
	if len(m.ecr.FilteredImages) > 0 {
		images = m.ecr.FilteredImages
	}
	if len(images) == 0 || m.ecr.SelectedImage >= len(images) {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No image selected"))
		return b.String()
	}

	img := images[m.ecr.SelectedImage]
	tags := "<untagged>"
	if len(img.Tags) > 0 {
		tags = strings.Join(img.Tags, ",")
	}

	b.WriteString(fmt.Sprintf("Digest: %s\nTags: %s\nSize: %s\nPushed: %s\nManifest: %s\nArtifact: %s\nRegistry: %s\n",
		img.Digest,
		tags,
		formatBytes(img.SizeBytes),
		formatTime(img.PushedAt),
		img.ManifestType,
		img.ArtifactType,
		img.RegistryID,
	))

	if img.Severity != nil && len(img.Severity) > 0 {
		b.WriteString("\nVulnerability summary:\n")
		var sevKeys []string
		for k := range img.Severity {
			sevKeys = append(sevKeys, k)
		}
		sort.Strings(sevKeys)
		for _, k := range sevKeys {
			b.WriteString(fmt.Sprintf(" - %s: %d\n", k, img.Severity[k]))
		}
	}

	b.WriteString("\nScan Findings (use 'r' to reload):\n")
	if m.loading {
		b.WriteString(" Loading scan...\n")
	} else if m.err != nil && m.ecr.ImageScanResult == nil {
		b.WriteString(fmt.Sprintf(" Error loading scan: %v\n", m.err))
	} else if m.ecr.ImageScanResult == nil {
		b.WriteString(" No scan data available\n")
	} else {
		scan := m.ecr.ImageScanResult
		b.WriteString(fmt.Sprintf(" Status: %s (%s)\n", scan.Status, scan.Description))
		b.WriteString(fmt.Sprintf(" Completed: %s | DB Updated: %s\n", formatTime(scan.CompletedAt), formatTime(scan.DBUpdatedAt)))
		if len(scan.SeverityCount) > 0 {
			b.WriteString(" Severity counts:\n")
			var keys []string
			for k := range scan.SeverityCount {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				b.WriteString(fmt.Sprintf("  - %s: %d\n", k, scan.SeverityCount[k]))
			}
		}
		if len(scan.Findings) > 0 {
			b.WriteString("\nFindings (first 10):\n")
			for i, f := range scan.Findings {
				b.WriteString(fmt.Sprintf(" %d. %s (%s)\n", i+1, f.Name, f.Severity))
				if f.Description != "" {
					b.WriteString("    " + f.Description + "\n")
				}
				if f.URI != "" {
					b.WriteString("    " + f.URI + "\n")
				}
			}
		}
	}

	return m.renderWithViewport(b.String())
}

func (m model) renderSecrets() string {
	title := lipgloss.NewStyle().Bold(true).Render("Secrets Manager")
	if m.loading {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("Loading secrets...")
	}
	if m.err != nil {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
	}

	list := m.secrets
	if len(m.secretsFiltered) > 0 {
		list = m.secretsFiltered
	} else if m.vimState.LastSearch != "" {
		list = []aws.SecretSummary{}
	}
	if len(list) == 0 {
		msg := "No secrets found"
		if m.vimState.LastSearch != "" {
			msg = "No secrets match your search"
		}
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(msg)
	}

	m.ensureVisible(m.selectedSecretIndex, len(list))
	start, end := m.getVisibleRange(len(list))

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Underline(true)
	var bld strings.Builder
	bld.WriteString(title + "\n\n")
	bld.WriteString(headerStyle.Render(fmt.Sprintf("%-32s %-10s %-19s %-19s", "SECRET", "ROTATION", "CHANGED", "ACCESSED")) + "\n")
	for i := start; i < end; i++ {
		sec := list[i]
		rot := "off"
		if sec.RotationEnabled {
			rot = "on"
		}
		row := fmt.Sprintf("%-32s %-10s %-19s %-19s",
			uiShared.Truncate(sec.Name, 32),
			rot,
			formatTime(sec.LastChanged),
			formatTime(sec.LastAccessed),
		)
		if i == m.selectedSecretIndex {
			row = "\x1b[48;5;51m\x1b[38;5;0m\x1b[1m" + row + "\x1b[K\x1b[0m"
		}
		bld.WriteString(row + "\n")
	}
	bld.WriteString(fmt.Sprintf("\nShowing %d-%d of %d secrets", start+1, end, len(list)))

	// quick summary
	if len(list) > 0 && m.selectedSecretIndex < len(list) {
		sec := list[m.selectedSecretIndex]
		bld.WriteString("\n\n")
		bld.WriteString(lipgloss.NewStyle().Bold(true).Render("Selected Secret") + "\n")
		bld.WriteString(fmt.Sprintf("Name: %s\nDesc: %s\nRotation: %t (Next: %s)\nKMS: %s\nService: %s\nPrimary Region: %s\n",
			sec.Name, sec.Description, sec.RotationEnabled, formatTime(sec.NextRotation), sec.KMSKeyID, sec.OwningService, sec.PrimaryRegion))
	}

	return bld.String()
}

func (m model) renderSecretDetails() string {
	title := lipgloss.NewStyle().Bold(true).Render("Secret Details")
	val := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	if m.loading && m.currentSecretDetails == nil {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("Loading secret details...")
	}
	if m.err != nil && m.currentSecretDetails == nil {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("Error: %v", m.err))
	}
	if m.currentSecretDetails == nil {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No secret selected")
	}

	sec := m.currentSecretDetails
	var b strings.Builder
	b.WriteString(title + "\n\n")
	b.WriteString(fmt.Sprintf("Name: %s\nARN: %s\nDesc: %s\nKMS: %s\nService: %s\nCreated: %s\nChanged: %s\nAccessed: %s\nRotation: %t (Next: %s) Lambda: %s\n",
		sec.Name, sec.Arn, sec.Description, sec.KMSKeyID, sec.OwningService, formatTime(sec.CreatedAt), formatTime(sec.LastChanged), formatTime(sec.LastAccessed), sec.RotationEnabled, formatTime(sec.NextRotation), sec.RotationARN))
	if sec.Rotation != nil {
		days := int64(0)
		if sec.Rotation.AutomaticallyAfterDays != nil {
			days = *sec.Rotation.AutomaticallyAfterDays
		}
		dur := ""
		if sec.Rotation.Duration != nil {
			dur = *sec.Rotation.Duration
		}
		sched := ""
		if sec.Rotation.ScheduleExpression != nil {
			sched = *sec.Rotation.ScheduleExpression
		}
		b.WriteString(fmt.Sprintf("Rotation Rules: after %d days duration %s schedule %s\n",
			days, dur, sched))
	}

	if len(sec.Versions) > 0 {
		b.WriteString("\nVersions (newest first):\n")
		limit := len(sec.Versions)
		if limit > 10 {
			limit = 10
		}
		for _, v := range sec.Versions[:limit] {
			b.WriteString(fmt.Sprintf(" - %s [%s] created %s\n", val(v.VersionId), strings.Join(v.VersionStages, ","), formatTime(v.CreatedDate)))
		}
	}

	if len(sec.Replication) > 0 {
		b.WriteString("\nReplication:\n")
		for _, r := range sec.Replication {
			b.WriteString(fmt.Sprintf(" - %s status %s kms %s lastAccess %s\n", val(r.Region), r.Status, val(r.KmsKeyId), formatTime(r.LastAccessedDate)))
		}
	}

	if len(sec.Tags) > 0 {
		b.WriteString("\nTags:\n")
		for _, t := range sec.Tags {
			b.WriteString(fmt.Sprintf(" - %s: %s\n", val(t.Key), val(t.Value)))
		}
	}

	b.WriteString("\nSecret Value:\n")
	if sec.RawJSON != "" {
		b.WriteString(sec.RawJSON + "\n")
	} else if sec.ValueString != "" {
		b.WriteString(sec.ValueString + "\n")
	} else {
		b.WriteString("<binary or empty>\n")
	}

	return m.renderWithViewport(b.String())
}

// Placeholder renders to keep build intact after refactors
func (m model) renderS3ObjectDetails() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("S3 object details view not implemented yet")
}

func (m model) renderEKS() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("EKS view not implemented yet")
}

func (m model) renderEKSDetails() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("EKS details view not implemented yet")
}

func (m model) renderHelp() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Help view not implemented yet")
}

// Utility helpers restored
func getStateStyle(state string) lipgloss.Style {
	switch strings.ToLower(state) {
	case "running", "active":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	case "stopped", "inactive":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	}
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("2006-01-02 15:04:05")
}

// Entry point - simplified runner
func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	m := initialModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
