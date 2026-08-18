package presentation

import "github.com/noelruault/lazyaws/apps/aws"

// GetSecretDisplayStrings never touches plaintext.
func GetSecretDisplayStrings(s *aws.SecretSummary) []string {
	rotation := "off"
	if s.RotationEnabled {
		rotation = "on"
	}
	status := "-"
	if s.DeletedDate != nil {
		status = "pending deletion"
	}
	return []string{s.Name, rotation, status}
}
