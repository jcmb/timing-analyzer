package gsofbaseline

import (
	"strings"

	"timing-analyzer/internal/core"
)

// StreamEndpointFromConfig builds a snapshot stream descriptor from listener config.
func StreamEndpointFromConfig(c core.Config) StreamEndpoint {
	mode := "tcp"
	if strings.EqualFold(strings.TrimSpace(c.IP), "udp") {
		mode = "udp"
	}
	return StreamEndpoint{
		Mode: mode,
		Host: strings.TrimSpace(c.Host),
		Port: c.Port,
	}
}
