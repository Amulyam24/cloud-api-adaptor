// (C) Copyright IBM Corp. 2022.
// SPDX-License-Identifier: Apache-2.0
//go:build ibmcloud

package ibmcloud

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/confidential-containers/cloud-api-adaptor/pkg/adaptor/hypervisor"
	"github.com/confidential-containers/cloud-api-adaptor/pkg/podnetwork"

	"github.com/containerd/ttrpc"
	pbHypervisor "github.com/kata-containers/kata-containers/src/runtime/protocols/hypervisor"
)

type PowerVCServer struct {
	socketPath string

	ttRpc   *ttrpc.Server
	service pbHypervisor.HypervisorService

	workerNode podnetwork.WorkerNode

	readyCh  chan struct{}
	stopCh   chan struct{}
	stopOnce sync.Once
}

func NewPowerVCServer(cfg hypervisor.Config, cloudCfg PowerVCConfig, workerNode podnetwork.WorkerNode, daemonPort string) hypervisor.Server {

	logger.Printf("hypervisor config %v", cfg)
	// logger.Printf("cloud config %v", cloudCfg.Redact())

	powervc, err := NewPowerVCService()
	if err != nil {
		panic(err)
	}

	s := &PowerVCServer{
		socketPath: cfg.SocketPath,
		service:    newPowerVCService(powervc, &cloudCfg, &cfg, workerNode, cfg.PodsDir, daemonPort),
		workerNode: workerNode,
		readyCh:    make(chan struct{}),
		stopCh:     make(chan struct{}),
	}
	return s
}

func (s *PowerVCServer) Start(ctx context.Context) (err error) {

	ttRpc, err := ttrpc.NewServer()
	if err != nil {
		return err
	}
	s.ttRpc = ttRpc
	if err := os.MkdirAll(filepath.Dir(s.socketPath), os.ModePerm); err != nil {
		return err
	}
	if err := os.RemoveAll(s.socketPath); err != nil { // just in case socket wasn't cleaned
		return err
	}
	pbHypervisor.RegisterHypervisorService(s.ttRpc, s.service)

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}

	ttRpcErr := make(chan error)
	go func() {
		defer close(ttRpcErr)
		if err := s.ttRpc.Serve(ctx, listener); err != nil {
			ttRpcErr <- err
		}
	}()
	defer func() {
		ttRpcShutdownErr := s.ttRpc.Shutdown(context.Background())
		if ttRpcShutdownErr != nil && err == nil {
			err = ttRpcShutdownErr
		}
	}()

	close(s.readyCh)

	select {
	case <-ctx.Done():
		shutdownErr := s.Shutdown()
		if shutdownErr != nil && err == nil {
			err = shutdownErr
		}
	case <-s.stopCh:
	case err = <-ttRpcErr:
	}
	return err
}

func (s *PowerVCServer) Shutdown() error {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	return nil
}

func (s *PowerVCServer) Ready() chan struct{} {
	return s.readyCh
}
