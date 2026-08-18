package ui

import (
	"context"
	"os"

	awsapp "github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/config"
)

// Run checks authentication before gocui owns the terminal so recovery guidance remains readable.
func Run(cfg config.Config) error {
	profile := currentProfileName()
	profiles := listAWSProfiles()

	client, err := awsapp.NewClient(context.Background(), &awsapp.Config{Region: cfg.Region})
	if err != nil {
		// Nothing to switch to means nothing to start for; otherwise the Profiles panel is the way out.
		if len(profiles) == 0 {
			return &startupError{message: clientFailureMessage(profile, err, profiles)}
		}
		client = nil
	}

	degraded, authErr := preflight(client, profile, profiles)
	if authErr != nil {
		return authErr
	}

	gui, err := NewGui(&cfg, client, make(chan error))
	if err != nil {
		return err
	}
	gui.CurrentProfile = os.Getenv("AWS_PROFILE")
	if degraded {
		gui.authProblem = credentialsProblem(client, err)
	}

	return gui.Run()
}
