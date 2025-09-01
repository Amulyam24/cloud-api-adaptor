// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package byom

import (
	"strings"

	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-providers/util"
)

// vmPoolIPs represents a flag for VM pool IP addresses
type vmPoolIPs []string

// String returns the string representation of the vmPoolIPs
func (v *vmPoolIPs) String() string {
	return strings.Join(*v, ",")
}

// Set parses the input string and sets the vmPoolIPs value
func (v *vmPoolIPs) Set(value string) error {
	if len(value) == 0 {
		*v = make(vmPoolIPs, 0)
		return nil
	}

	*v = strings.Split(value, ",")
	// Trim spaces from each IP
	for i := range *v {
		(*v)[i] = strings.TrimSpace((*v)[i])
	}
	return nil
}

// Config holds the BYOM provider configuration
type Config struct {
	VMPoolIPs            vmPoolIPs // VM pool IP addresses (required)
	SSHUserName          string    // SSH username for VM access
	SSHPubKeyPath        string    // SSH public key file path
	SSHPrivKeyPath       string    // SSH private key file path
	SSHPubKey            string    // SSH public key content (populated from file)
	SSHPrivKey           string    // SSH private key content (populated from file)
	SSHTimeout           int       // SSH connection timeout in seconds
	SSHInsecureIgnoreHostKey bool  // Skip SSH host key verification (for debugging)
}

// Redact returns a copy of the config with sensitive information redacted
func (c Config) Redact() Config {
	return *util.RedactStruct(&c, "SSHPrivKey").(*Config)
}