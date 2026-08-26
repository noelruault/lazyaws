package presentation

import (
	"fmt"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// SecretWeights gives the secret name the slack; both columns to its right are fixed phrases.
func SecretWeights() []int {
	return []int{1, 0, 0}
}

// GetSecretDisplayCells never touches plaintext.
func GetSecretDisplayCells(s *aws.SecretSummary) []utils.Cell {
	status := utils.Cell{Text: "-"}
	if s.DeletedDate != nil {
		status = utils.Cell{Text: "pending deletion", Color: color.FgRed}
	}

	return []utils.Cell{
		{Text: s.Name, Color: color.Bold},
		rotationCell(s),
		status,
	}
}

// rotationCell labels its own value because the column has no header: "7d" alone in a list of secrets reads as an age.
// Rotation that is on with no day cadence is reported as on rather than as a number, since a secret scheduled by expression carries no AutomaticallyAfterDays until it has rotated once, and printing "rotation 0d" there would invent a cadence.
func rotationCell(s *aws.SecretSummary) utils.Cell {
	switch {
	case !s.RotationEnabled:
		return utils.Cell{Text: "rotation off"}
	case s.RotationDays > 0:
		return utils.Cell{Text: fmt.Sprintf("rotation %dd", s.RotationDays)}
	default:
		return utils.Cell{Text: "rotation on"}
	}
}
