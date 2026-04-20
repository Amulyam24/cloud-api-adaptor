// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package byom

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	pv "github.com/confidential-containers/cloud-api-adaptor/src/cloud-api-adaptor/test/provisioner"
	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-api-adaptor/test/provisioner/docker"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// ByomProvisioner extends DockerProvisioner for BYOM-specific functionality
type ByomProvisioner struct {
	*docker.DockerProvisioner
	provisionerCreatedContainers []string // Track containers created by this provisioner instance
}

// ByomInstallChart implements the InstallChart interface
type ByomInstallChart struct {
	Helm *pv.Helm
}

type ByomProperties struct {
	SSHSecretPrivKeyPath string
	SSHSecretPubKeyPath  string
	SSHUsername          string
	VMPoolIPs            string
	PoolSize             int // Number of containers to create for the pool
	ClusterName          string
	ContainerRuntime     string
	DockerHost           string
	DockerNetworkName    string
	DockerPodvmImage     string
	CaaImage             string
	CaaImageTag          string
	TunnelType           string
	VxlanPort            string
}

var ByomProps = &ByomProperties{}

func initByomProperties(properties map[string]string) error {
	// Parse pool size with default of 3
	poolSize := 3
	if poolSizeStr := properties["POOL_SIZE"]; poolSizeStr != "" {
		if parsed, err := strconv.Atoi(poolSizeStr); err == nil && parsed > 0 {
			poolSize = parsed
		}
	}

	ByomProps = &ByomProperties{
		SSHSecretPrivKeyPath: properties["SSH_SECRET_PRIV_KEY_PATH"],
		SSHSecretPubKeyPath:  properties["SSH_SECRET_PUB_KEY_PATH"],
		SSHUsername:          properties["SSH_USERNAME"],
		VMPoolIPs:            properties["VM_POOL_IPS"],
		PoolSize:             poolSize,
		ClusterName:          properties["CLUSTER_NAME"],
		ContainerRuntime:     properties["CONTAINER_RUNTIME"],
		DockerHost:           properties["DOCKER_HOST"],
		DockerNetworkName:    properties["DOCKER_NETWORK_NAME"],
		DockerPodvmImage:     properties["DOCKER_PODVM_IMAGE"],
		CaaImage:             properties["CAA_IMAGE"],
		CaaImageTag:          properties["CAA_IMAGE_TAG"],
		TunnelType:           properties["TUNNEL_TYPE"],
		VxlanPort:            properties["VXLAN_PORT"],
	}

	// Set defaults
	if ByomProps.SSHUsername == "" {
		ByomProps.SSHUsername = "peerpod"
	}
	if ByomProps.ClusterName == "" {
		ByomProps.ClusterName = "peer-pods"
	}
	if ByomProps.ContainerRuntime == "" {
		ByomProps.ContainerRuntime = "containerd"
	}
	if ByomProps.DockerNetworkName == "" {
		ByomProps.DockerNetworkName = "kind"
	}

	return nil
}

func NewByomProvisioner(properties map[string]string) (pv.CloudProvisioner, error) {
	if err := initByomProperties(properties); err != nil {
		return nil, err
	}

	// Create docker properties from BYOM properties
	dockerProps := map[string]string{
		"DOCKER_HOST":         ByomProps.DockerHost,
		"DOCKER_NETWORK_NAME": ByomProps.DockerNetworkName,
		"DOCKER_PODVM_IMAGE":  ByomProps.DockerPodvmImage,
		"CLUSTER_NAME":        ByomProps.ClusterName,
		"CONTAINER_RUNTIME":   ByomProps.ContainerRuntime,
		"CAA_IMAGE":           ByomProps.CaaImage,
		"CAA_IMAGE_TAG":       ByomProps.CaaImageTag,
		"TUNNEL_TYPE":         ByomProps.TunnelType,
		"VXLAN_PORT":          ByomProps.VxlanPort,
	}

	dockerProvisioner, err := docker.NewDockerProvisioner(dockerProps)
	if err != nil {
		return nil, err
	}

	return &ByomProvisioner{
		DockerProvisioner:            dockerProvisioner.(*docker.DockerProvisioner),
		provisionerCreatedContainers: make([]string, 0),
	}, nil
}

func (b *ByomProvisioner) CreateCluster(ctx context.Context, cfg *envconf.Config) error {
	return b.DockerProvisioner.CreateCluster(ctx, cfg)
}

// Calling this method means we are not using existing pre-created container IPs. Instead asking the provisioner
// to create new containers from the uploaded image and use their IPs.
func (b *ByomProvisioner) CreatePodVMInstance(ctx context.Context, cfg *envconf.Config) error {
	log.Infof("Creating %d BYOM container instances for testing", ByomProps.PoolSize)

	var poolIPs []string

	for i := 0; i < ByomProps.PoolSize; i++ {
		// Create container with unique name (timestamp ensures uniqueness across test runs)
		containerName := fmt.Sprintf("byom-container-%d-%d", time.Now().Unix(), i)

		log.Infof("Creating container %d/%d: %s", i+1, ByomProps.PoolSize, containerName)
		if err := b.createContainerFromImage(containerName); err != nil {
			return fmt.Errorf("failed to create container %d: %w", i, err)
		}

		// Track this container as created by the provisioner
		b.provisionerCreatedContainers = append(b.provisionerCreatedContainers, containerName)

		// Get container IP address
		ip, err := b.getContainerIPAddress(containerName)
		if err != nil {
			return fmt.Errorf("failed to get IP for container %d: %w", i, err)
		}

		poolIPs = append(poolIPs, ip)
		log.Infof("Created container %d/%d: %s with IP: %s", i+1, ByomProps.PoolSize, containerName, ip)
	}

	// Set pool to all IPs (comma-separated)
	ByomProps.VMPoolIPs = strings.Join(poolIPs, ",")

	log.Infof("Successfully created %d containers with pool IPs: %s", ByomProps.PoolSize, ByomProps.VMPoolIPs)
	return nil
}

func (b *ByomProvisioner) DeleteCluster(ctx context.Context, cfg *envconf.Config) error {
	return b.DockerProvisioner.DeleteCluster(ctx, cfg)
}

func (b *ByomProvisioner) DeletePodVMInstance(ctx context.Context, cfg *envconf.Config) error {
	// Only delete containers that were created by this provisioner instance
	if len(b.provisionerCreatedContainers) == 0 {
		log.Info("No containers created by this provisioner to clean up")
		return nil
	}

	log.Infof("Cleaning up %d containers created by this provisioner", len(b.provisionerCreatedContainers))

	for _, containerName := range b.provisionerCreatedContainers {
		log.Infof("Destroying provisioner-created container: %s", containerName)
		if err := b.destroyContainer(containerName); err != nil {
			log.Warnf("Failed to destroy container %s: %v", containerName, err)
		}
	}

	// Clear the tracking and container pool IPs
	b.provisionerCreatedContainers = make([]string, 0)
	ByomProps.VMPoolIPs = ""

	return nil
}

func (b *ByomProvisioner) GetProperties(ctx context.Context, cfg *envconf.Config) map[string]string {
	return map[string]string{
		"VM_POOL_IPS":              ByomProps.VMPoolIPs,
		"SSH_SECRET_PRIV_KEY_PATH": ByomProps.SSHSecretPrivKeyPath,
		"SSH_SECRET_PUB_KEY_PATH":  ByomProps.SSHSecretPubKeyPath,
		"SSH_USERNAME":             ByomProps.SSHUsername,
		"CLUSTER_NAME":             ByomProps.ClusterName,
		"CONTAINER_RUNTIME":        ByomProps.ContainerRuntime,
		"DOCKER_HOST":              ByomProps.DockerHost,
		"DOCKER_NETWORK_NAME":      ByomProps.DockerNetworkName,
		"DOCKER_PODVM_IMAGE":       ByomProps.DockerPodvmImage,
		"CAA_IMAGE":                ByomProps.CaaImage,
		"CAA_IMAGE_TAG":            ByomProps.CaaImageTag,
		"TUNNEL_TYPE":              ByomProps.TunnelType,
		"VXLAN_PORT":               ByomProps.VxlanPort,
	}
}

func NewByomInstallChart(installDir, provider string) (pv.InstallChart, error) {
	chartPath := filepath.Join(installDir, "charts", "peerpods")
	namespace := pv.GetCAANamespace()
	releaseName := "peerpods"
	debug := false

	helm, err := pv.NewHelm(chartPath, namespace, releaseName, provider, debug)
	if err != nil {
		return nil, err
	}

	return &ByomInstallChart{
		Helm: helm,
	}, nil
}

func (b *ByomInstallChart) Install(ctx context.Context, cfg *envconf.Config) error {
	return b.Helm.Install(ctx, cfg)
}

func (b *ByomInstallChart) Uninstall(ctx context.Context, cfg *envconf.Config) error {
	return b.Helm.Uninstall(ctx, cfg)
}

func (b *ByomInstallChart) Configure(ctx context.Context, cfg *envconf.Config, properties map[string]string) error {
	// Handle CAA image - already split into CAA_IMAGE and CAA_IMAGE_TAG
	if properties["CAA_IMAGE"] != "" {
		b.Helm.OverrideValues["image.name"] = properties["CAA_IMAGE"]
	}
	if properties["CAA_IMAGE_TAG"] != "" {
		b.Helm.OverrideValues["image.tag"] = properties["CAA_IMAGE_TAG"]
	}

	// Mapping the internal properties to Helm chart values.
	mapProps := map[string]string{
		"VM_POOL_IPS":              "VM_POOL_IPS",
		"SSH_USERNAME":             "SSH_USERNAME",
		"SSH_SECRET_PRIV_KEY_PATH": "SSH_SECRET_PRIV_KEY_PATH",
		"SSH_SECRET_PUB_KEY_PATH":  "SSH_SECRET_PUB_KEY_PATH",
		"DOCKER_HOST":              "DOCKER_HOST",
		"DOCKER_NETWORK_NAME":      "DOCKER_NETWORK_NAME",
		"DOCKER_PODVM_IMAGE":       "DOCKER_PODVM_IMAGE",
		"TUNNEL_TYPE":              "TUNNEL_TYPE",
		"VXLAN_PORT":               "VXLAN_PORT",
		"INITDATA":                 "INITDATA",
	}

	for k, v := range mapProps {
		if properties[k] != "" {
			b.Helm.OverrideProviderValues[v] = properties[k]
		}
	}

	return nil
}

func (b *ByomProvisioner) createContainerFromImage(containerName string) error {
	log.Infof("Creating container %s from Docker image", containerName)

	// Use docker run to create and start container
	// --restart=always ensures container restarts after reboot trigger
	cmd := exec.Command("docker", "run",
		"-d",
		"--name", containerName,
		"--network", ByomProps.DockerNetworkName,
		"--privileged",
		"--restart=always",
		ByomProps.DockerPodvmImage)

	stdoutStderr, err := cmd.CombinedOutput()
	log.Tracef("%v, output: %s", cmd, stdoutStderr)
	if err != nil {
		return fmt.Errorf("failed to create container with docker run: %w, output: %s", err, string(stdoutStderr))
	}

	log.Infof("Container %s created and started successfully", containerName)

	// Copy SSH public key to container if provided
	if ByomProps.SSHSecretPubKeyPath != "" {
		if err := b.copySSHKeyToContainer(containerName); err != nil {
			return fmt.Errorf("failed to copy SSH key to container: %w", err)
		}
	}

	return nil
}

func (b *ByomProvisioner) copySSHKeyToContainer(containerName string) error {
	log.Infof("Copying SSH public key from %s to container %s", ByomProps.SSHSecretPubKeyPath, containerName)

	// Copy the public key file to container
	cmd := exec.Command("docker", "cp",
		ByomProps.SSHSecretPubKeyPath,
		fmt.Sprintf("%s:/home/peerpod/.ssh/authorized_keys", containerName))

	stdoutStderr, err := cmd.CombinedOutput()
	log.Tracef("%v, output: %s", cmd, stdoutStderr)
	if err != nil {
		return fmt.Errorf("failed to copy SSH key: %w, output: %s", err, string(stdoutStderr))
	}

	// Fix permissions on the authorized_keys file
	cmd = exec.Command("docker", "exec", containerName,
		"chmod", "600", "/home/peerpod/.ssh/authorized_keys")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set permissions on authorized_keys: %w", err)
	}

	// Fix ownership of the authorized_keys file
	cmd = exec.Command("docker", "exec", containerName,
		"chown", "peerpod:peerpod", "/home/peerpod/.ssh/authorized_keys")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set ownership on authorized_keys: %w", err)
	}

	log.Infof("SSH public key copied and configured successfully")
	return nil
}

func (b *ByomProvisioner) getContainerIPAddress(containerName string) (string, error) {
	log.Infof("Getting IP address for container %s", containerName)

	// Wait up to 2 minutes for container to get an IP
	timeout := time.Now().Add(2 * time.Minute)
	for time.Now().Before(timeout) {
		cmd := exec.Command("docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", containerName)
		output, err := cmd.Output()
		if err != nil {
			log.Debugf("Error getting container IP (retrying): %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		ip := strings.TrimSpace(string(output))
		if ip != "" {
			log.Infof("Found IP %s for container %s", ip, containerName)
			return ip, nil
		}

		log.Debugf("No IP found yet for container %s, retrying...", containerName)
		time.Sleep(5 * time.Second)
	}

	return "", fmt.Errorf("timeout waiting for container %s to get IP address", containerName)
}

func (b *ByomProvisioner) destroyContainer(containerName string) error {
	log.Infof("Destroying container %s", containerName)

	// Stop container (force if necessary)
	if err := exec.Command("docker", "stop", containerName).Run(); err != nil {
		log.Warnf("Failed to stop container %s: %v", containerName, err)
	}

	// Remove container
	if err := exec.Command("docker", "rm", "-f", containerName).Run(); err != nil {
		return fmt.Errorf("Failed to remove container %s: %v", containerName, err)
	}

	log.Infof("Container %s destroyed successfully", containerName)
	return nil
}
