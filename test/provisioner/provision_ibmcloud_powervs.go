//go:build ibmcloud_powervs

// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package provisioner

import (
	"context"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

func init() {
	newProvisionerFunctions["ibmcloud-powervs"] = NewIBMCloudPowerVSProvisioner
	newInstallOverlayFunctions["ibmcloud-powervs"] = NewIBMCloudPowerVSInstallOverlay
}

type IBMCloudPowerVSProvisioner struct {
}

func NewIBMCloudPowerVSProvisioner(properties map[string]string) (CloudProvisioner, error) {
	if err := initProperties(properties); err != nil {
		return nil, err
	}

	return &IBMCloudPowerVSProvisioner{}, nil
}

func (p *IBMCloudPowerVSProvisioner) CreateCluster(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (p *IBMCloudPowerVSProvisioner) DeleteCluster(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (p *IBMCloudPowerVSProvisioner) CreateVPC(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (p *IBMCloudPowerVSProvisioner) DeleteVPC(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (p *IBMCloudPowerVSProvisioner) GetProperties(ctx context.Context, cfg *envconf.Config) map[string]string {
	return nil
}

func (p *IBMCloudPowerVSProvisioner) UploadPodvm(imagePath string, ctx context.Context, cfg *envconf.Config) error {
	return nil
}

type IBMCloudPowerVSInstallOverlay struct {
	overlay *KustomizeOverlay
}

func NewIBMCloudPowerVSInstallOverlay() (InstallOverlay, error) {
	overlay, err := NewKustomizeOverlay("../../install/overlays/ibmcloud-powervs")
	if err != nil {
		return nil, err
	}

	return &IBMCloudPowerVSInstallOverlay{
		overlay: overlay,
	}, nil
}

func (lio *IBMCloudPowerVSInstallOverlay) Apply(ctx context.Context, cfg *envconf.Config) error {
	return lio.overlay.Apply(ctx, cfg)
}

func (lio *IBMCloudPowerVSInstallOverlay) Delete(ctx context.Context, cfg *envconf.Config) error {
	return lio.overlay.Delete(ctx, cfg)
}

func (lio *IBMCloudPowerVSInstallOverlay) Edit(ctx context.Context, cfg *envconf.Config, properties map[string]string) error {
	return nil
}
