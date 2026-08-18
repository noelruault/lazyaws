package presentation

import "github.com/noelruault/lazyaws/apps/aws"

// GetVPCDisplayStrings leads with the CIDR because that is what a VPC gets recognised by when tracing whether two networks can reach each other.
func GetVPCDisplayStrings(v *aws.VPC) []string {
	label := v.Name
	if label == "" {
		label = "(no name)"
	}
	if v.IsDefault {
		label += " (default)"
	}

	return []string{
		StatusCell(v.State, StatusStyleIcon),
		v.CIDR,
		label,
		v.ID,
	}
}

// GetVPCEndpointDisplayStrings labels the row by service rather than by id, because endpoints are usually untagged and one id looks like the next.
func GetVPCEndpointDisplayStrings(e *aws.VPCEndpoint) []string {
	label := e.Name
	if label == "" {
		label = e.ShortService()
	}

	return []string{
		StatusCell(e.State, StatusStyleIcon),
		label,
		e.ID,
		e.Type,
		e.VpcID,
	}
}
