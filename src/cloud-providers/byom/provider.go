// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package byom

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"time"

	provider "github.com/confidential-containers/cloud-api-adaptor/src/cloud-providers"
	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-providers/util"
	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-providers/util/cloudinit"
)

var logger = log.New(log.Writer(), "[adaptor/cloud/byom] ", log.LstdFlags|log.Lmsgprefix)

const (
	sshPort    = "22"
	remoteFile = "/media/cidata/user-data" // Standard cloud-init user-data location
	rebootFile = "/media/cidata/reboot"    // Reboot trigger file
)

// byomProvider implements the Provider interface for BYOM
type byomProvider struct {
	serviceConfig *Config
	vmPool        *VMPool
}

// NewProvider creates a new BYOM provider instance
func NewProvider(config *Config) (provider.Provider, error) {
	logger.Printf("BYOM config: %+v", config.Redact())

	// Initialize SSH keys for authentication
	sshConfig := &util.SSHConfig{
		PublicKey:      config.SSHPubKey,
		PrivateKey:     config.SSHPrivKey,
		PublicKeyPath:  config.SSHPubKeyPath,
		PrivateKeyPath: config.SSHPrivKeyPath,
		Username:       config.SSHUserName,
		EnableSFTP:     true, // Always enabled for BYOM
	}

	if err := util.InitializeSSHKeys(sshConfig); err != nil {
		return nil, fmt.Errorf("failed to initialize SSH keys: %w", err)
	}

	// Update config with initialized keys
	config.SSHPubKey = sshConfig.PublicKey
	config.SSHPrivKey = sshConfig.PrivateKey

	// Initialize VM pool
	vmPool, err := NewVMPool(config.VMPoolIPs)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize VM pool: %w", err)
	}

	p := &byomProvider{
		serviceConfig: config,
		vmPool:        vmPool,
	}

	// Log pool status
	total, available, _ := p.vmPool.GetStatus()
	logger.Printf("Initialized BYOM provider with %d VMs (%d available)", total, available)

	return p, nil
}

// CreateInstance allocates a VM from the pool and configures it
func (p *byomProvider) CreateInstance(ctx context.Context, podName, sandboxID string, cloudConfig cloudinit.CloudConfigGenerator, spec provider.InstanceTypeSpec) (*provider.Instance, error) {
	// Generate allocation ID
	allocationID := fmt.Sprintf("%s-%s", podName, sandboxID)

	// Allocate IP from pool
	ip, err := p.vmPool.AllocateIP(allocationID)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate IP from pool: %w", err)
	}

	// Generate cloud config data
	cloudConfigData, err := cloudConfig.Generate()
	if err != nil {
		// Rollback allocation on error
		p.vmPool.DeallocateByAllocationID(allocationID)
		return nil, fmt.Errorf("failed to generate cloud config: %w", err)
	}

	// Send config to the VM via SFTP
	if err := p.sendConfigFile(cloudConfigData, ip); err != nil {
		// Rollback allocation on error
		p.vmPool.DeallocateByAllocationID(allocationID)
		return nil, fmt.Errorf("failed to send config to VM %s: %w", ip.String(), err)
	}

	// Create instance object
	instance := &provider.Instance{
		ID:   ip.String(), // Use IP as instance ID for BYOM
		Name: fmt.Sprintf("byom-%s", ip.String()),
		IPs:  []netip.Addr{ip},
	}

	// Log current pool status
	total, available, inUse := p.vmPool.GetStatus()
	logger.Printf("Created instance %s: total=%d, available=%d, inUse=%d", instance.ID, total, available, inUse)

	return instance, nil
}

// DeleteInstance returns a VM back to the pool
func (p *byomProvider) DeleteInstance(ctx context.Context, instanceID string) error {
	// Parse instance ID (which is the IP address)
	ip, err := netip.ParseAddr(instanceID)
	if err != nil {
		return fmt.Errorf("invalid instance ID %s: %w", instanceID, err)
	}

	// Send reboot trigger file to VM before deallocating
	if err := p.sendRebootFile(ip); err != nil {
		logger.Printf("Warning: failed to send reboot file to VM %s: %v", ip.String(), err)
		// Continue with deallocation even if reboot file sending fails
	}

	// Return IP to pool
	if err := p.vmPool.DeallocateByIP(ip); err != nil {
		return fmt.Errorf("failed to deallocate IP %s: %w", ip.String(), err)
	}

	logger.Printf("Returned VM to pool (not deleted): IP=%s", ip.String())

	// Log current pool status
	total, available, inUse := p.vmPool.GetStatus()
	logger.Printf("Pool status after deallocation: total=%d, available=%d, inUse=%d", total, available, inUse)

	return nil
}

// Teardown cleans up resources
func (p *byomProvider) Teardown() error {
	logger.Printf("BYOM provider teardown completed")
	return nil
}

// ConfigVerifier validates the provider configuration
func (p *byomProvider) ConfigVerifier() error {
	if len(p.serviceConfig.VMPoolIPs) == 0 {
		return fmt.Errorf("vm-pool-ips is required and cannot be empty")
	}

	if p.serviceConfig.SSHUserName == "" {
		return fmt.Errorf("ssh-username is required")
	}

	if p.serviceConfig.SSHPrivKey == "" {
		return fmt.Errorf("SSH private key is required")
	}

	// Test SSH connectivity to all VMs using common utility
	logger.Printf("Verifying SSH connectivity to %d VMs...", len(p.serviceConfig.VMPoolIPs))

	sshClientConfig := &util.SSHClientConfig{
		Username:              p.serviceConfig.SSHUserName,
		PrivateKey:            p.serviceConfig.SSHPrivKey,
		Timeout:               time.Duration(p.serviceConfig.SSHTimeout) * time.Second,
		InsecureIgnoreHostKey: p.serviceConfig.SSHInsecureIgnoreHostKey,
	}

	sshConfig, err := util.CreateSSHClientConfig(sshClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create SSH config: %w", err)
	}

	if err := util.ValidateSSHConnectivityToIPs(p.serviceConfig.VMPoolIPs, sshPort, sshConfig); err != nil {
		return err
	}

	logger.Printf("SSH connectivity verified for all %d VMs", len(p.serviceConfig.VMPoolIPs))
	return nil
}

// sendConfigFile sends cloud-init user-data to a VM via SFTP using common utility
func (p *byomProvider) sendConfigFile(userData string, ip netip.Addr) error {
	sshClientConfig := &util.SSHClientConfig{
		Username:              p.serviceConfig.SSHUserName,
		PrivateKey:            p.serviceConfig.SSHPrivKey,
		Timeout:               time.Duration(p.serviceConfig.SSHTimeout) * time.Second,
		InsecureIgnoreHostKey: p.serviceConfig.SSHInsecureIgnoreHostKey,
	}

	sshConfig, err := util.CreateSSHClientConfig(sshClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create SSH config: %w", err)
	}

	address := net.JoinHostPort(ip.String(), sshPort)
	if err := util.SendFileViaSFTP(address, sshConfig, remoteFile, []byte(userData)); err != nil {
		return fmt.Errorf("failed to send user-data to VM %s: %w", ip.String(), err)
	}

	logger.Printf("Successfully sent user-data to VM %s", ip.String())
	return nil
}

// sendRebootFile sends a reboot trigger file to a VM via SFTP
func (p *byomProvider) sendRebootFile(ip netip.Addr) error {
	sshClientConfig := &util.SSHClientConfig{
		Username:              p.serviceConfig.SSHUserName,
		PrivateKey:            p.serviceConfig.SSHPrivKey,
		Timeout:               time.Duration(p.serviceConfig.SSHTimeout) * time.Second,
		InsecureIgnoreHostKey: p.serviceConfig.SSHInsecureIgnoreHostKey,
	}

	sshConfig, err := util.CreateSSHClientConfig(sshClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create SSH config: %w", err)
	}

	address := net.JoinHostPort(ip.String(), sshPort)
	rebootData := []byte("reboot")
	if err := util.SendFileViaSFTP(address, sshConfig, rebootFile, rebootData); err != nil {
		return fmt.Errorf("failed to send reboot file to VM %s: %w", ip.String(), err)
	}

	logger.Printf("Successfully sent reboot trigger to VM %s", ip.String())
	return nil
}
