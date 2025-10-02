package ssh

import (
	"fmt"
	"strings"

	"github.com/gobwas/glob"
)

// HostValidator validates SSH hosts against allowed patterns
type HostValidator struct {
	patterns []glob.Glob
}

// NewHostValidator creates a new host validator with the given allowed hosts
// allowedHosts is a comma-separated list of host patterns (supports glob: *.example.com)
func NewHostValidator(allowedHosts string) (*HostValidator, error) {
	if allowedHosts == "" {
		return nil, fmt.Errorf("no allowed hosts specified")
	}

	hosts := strings.Split(allowedHosts, ",")
	patterns := make([]glob.Glob, 0, len(hosts))

	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}

		pattern, err := glob.Compile(host)
		if err != nil {
			return nil, fmt.Errorf("invalid host pattern '%s': %w", host, err)
		}

		patterns = append(patterns, pattern)
	}

	if len(patterns) == 0 {
		return nil, fmt.Errorf("no valid host patterns provided")
	}

	return &HostValidator{
		patterns: patterns,
	}, nil
}

// Validate checks if the given host is allowed
func (v *HostValidator) Validate(host string) error {
	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}

	for _, pattern := range v.patterns {
		if pattern.Match(host) {
			return nil
		}
	}

	return fmt.Errorf("host '%s' is not in the allowed hosts list", host)
}
