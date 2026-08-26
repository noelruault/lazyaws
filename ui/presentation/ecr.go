package presentation

import (
	"strings"

	"github.com/fatih/color"

	"github.com/noelruault/lazyaws/apps/aws"
	"github.com/noelruault/lazyaws/ui/utils"
)

// ECRRepositoryWeights gives the repository name the slack; both policy columns are fixed words.
func ECRRepositoryWeights() []int {
	return []int{1, 0, 0}
}

func GetECRRepositoryDisplayCells(r *aws.ECRRepository) []utils.Cell {
	scan := "scan off"
	if r.ScanOnPush {
		scan = "scan on"
	}

	return []utils.Cell{
		{Text: r.Name, Color: color.Bold},
		ecrMutabilityBadge(r.TagMutability),
		{Text: scan},
	}
}

// ecrMutabilityBadge colours only the mutable case: a mutable tag can be overwritten under a running deployment, so the digest behind "latest" is not fixed, while an immutable repository is the posture nobody needs alerting to.
// ECR reports four values, so the match is on the prefix; the two _WITH_EXCLUSION variants carry an exclusion list that makes the policy partial, and are starred rather than rendered as a blanket policy they are not. The raw value is on the repository's Config tab.
func ecrMutabilityBadge(mutability string) utils.Cell {
	star := ""
	if strings.HasSuffix(mutability, "_WITH_EXCLUSION") {
		star = "*"
	}

	switch {
	case strings.HasPrefix(mutability, "IMMUTABLE"):
		return utils.Cell{Text: "immutable" + star}
	case strings.HasPrefix(mutability, "MUTABLE"):
		return utils.Cell{Text: "● mutable" + star, Color: color.FgYellow}
	case mutability == "":
		return utils.Cell{Text: "-"}
	default:
		// An enum value this build does not know about is shown as AWS sent it rather than guessed at.
		return utils.Cell{Text: mutability}
	}
}
