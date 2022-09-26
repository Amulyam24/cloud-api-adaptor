// (C) Copyright IBM Corp. 2022.
// SPDX-License-Identifier: Apache-2.0
//go:build ibmcloud

package ibmcloud

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/IBM-Cloud/power-go-client/power/models"
	"github.com/IBM/go-sdk-core/v5/core"

	"github.com/confidential-containers/cloud-api-adaptor/pkg/adaptor/hypervisor"
	"github.com/confidential-containers/cloud-api-adaptor/pkg/adaptor/proxy"
	daemon "github.com/confidential-containers/cloud-api-adaptor/pkg/forwarder"
	"github.com/confidential-containers/cloud-api-adaptor/pkg/podnetwork"
	"github.com/confidential-containers/cloud-api-adaptor/pkg/util/cloudinit"
	"github.com/confidential-containers/cloud-api-adaptor/pkg/util/hvutil"
	"github.com/containerd/containerd/pkg/cri/annotations"

	pb "github.com/kata-containers/kata-containers/src/runtime/protocols/hypervisor"
)

type hypervisorPVSService struct {
	powervsService   PowerVS
	serviceConfig    *PowerVSConfig
	hypervisorConfig *hypervisor.Config
	sandboxes        map[sandboxID]*sandbox
	podsDir          string
	daemonPort       string
	nodeName         string
	workerNode       podnetwork.WorkerNode
	sync.Mutex
}

const maxInstanceNameLength = 45

func newPowerVSService(powervs PowerVS, config *PowerVSConfig, hypervisorConfig *hypervisor.Config, workerNode podnetwork.WorkerNode, podsDir, daemonPort string) pb.HypervisorService {

	logger.Printf("service config %v", config.Redact())

	hostname, err := os.Hostname()
	if err != nil {
		panic(fmt.Errorf("failed to get hostname: %w", err))
	}

	i := strings.Index(hostname, ".")
	if i >= 0 {
		hostname = hostname[0:i]
	}

	return &hypervisorPVSService{
		powervsService:   powervs,
		serviceConfig:    config,
		hypervisorConfig: hypervisorConfig,
		sandboxes:        map[sandboxID]*sandbox{},
		podsDir:          podsDir,
		daemonPort:       daemonPort,
		nodeName:         hostname,
		workerNode:       workerNode,
	}
}

func (s *hypervisorPVSService) Version(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	return &pb.VersionResponse{Version: Version}, nil
}

func (s *hypervisorPVSService) CreateVM(ctx context.Context, req *pb.CreateVMRequest) (*pb.CreateVMResponse, error) {

	sid := sandboxID(req.Id)

	if sid == "" {
		return nil, errors.New("empty sandbox id")
	}
	s.Lock()
	defer s.Unlock()
	if _, exists := s.sandboxes[sid]; exists {
		return nil, fmt.Errorf("sandbox %s already exists", sid)
	}
	pod := req.Annotations[annotations.SandboxName]
	if pod == "" {
		return nil, fmt.Errorf("pod name %s is missing in annotations", annotations.SandboxName)
	}
	namespace := req.Annotations[annotations.SandboxNamespace]
	if namespace == "" {
		return nil, fmt.Errorf("namespace name %s is missing in annotations", annotations.SandboxNamespace)
	}

	podDirPath := filepath.Join(s.podsDir, string(sid))
	if err := os.MkdirAll(podDirPath, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create a pod directory: %s: %w", podDirPath, err)
	}

	logger.Printf("PodsDIRpath: %s", podDirPath)

	socketPath := filepath.Join(podDirPath, proxy.SocketName)
	logger.Printf("SocketPath: %s", socketPath)

	netNSPath := req.NetworkNamespacePath

	podNetworkConfig, err := s.workerNode.Inspect(netNSPath)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect netns %s: %w", netNSPath, err)
	}

	agentProxy := proxy.NewAgentProxy(socketPath, s.hypervisorConfig.CriSocketPath, s.hypervisorConfig.PauseImage)

	sandbox := &sandbox{
		id:               sid,
		pod:              pod,
		namespace:        namespace,
		netNSPath:        netNSPath,
		podDirPath:       podDirPath,
		agentProxy:       agentProxy,
		podNetworkConfig: podNetworkConfig,
	}
	s.sandboxes[sid] = sandbox
	logger.Printf("create a sandbox %s for pod %s in namespace %s (netns: %s)", req.Id, pod, namespace, sandbox.netNSPath)
	return &pb.CreateVMResponse{AgentSocketPath: socketPath}, nil
}

func (s *hypervisorPVSService) StartVM(ctx context.Context, req *pb.StartVMRequest) (*pb.StartVMResponse, error) {

	sandbox, err := s.getSandbox(req.Id)
	if err != nil {
		return nil, err
	}

	vmName := hvutil.CreateInstanceName(sandbox.pod, string(sandbox.id), maxInstanceNameLength)

	daemonConfig := daemon.Config{
		PodNamespace: sandbox.namespace,
		PodName:      sandbox.pod,
		PodNetwork:   sandbox.podNetworkConfig,
	}
	daemonJSON, err := json.MarshalIndent(daemonConfig, "", "    ")
	if err != nil {
		return nil, err
	}

	// Store daemon.json in worker node for debugging
	if err := os.WriteFile(filepath.Join(sandbox.podDirPath, "daemon.json"), daemonJSON, 0666); err != nil {
		return nil, fmt.Errorf("failed to store daemon.json at %s: %w", sandbox.podDirPath, err)
	}
	logger.Printf("store daemon.json at %s", sandbox.podDirPath)

	cloudConfig := &cloudinit.CloudConfig{
		WriteFiles: []cloudinit.WriteFile{
			{
				Path:    daemon.DefaultConfigPath,
				Content: string(daemonJSON),
			},
		},
	}

	userData, err := cloudConfig.Generate()
	if err != nil {
		return nil, err
	}
	body := &models.PVMInstanceCreate{
		ServerName:  &vmName,
		ImageID:     &s.serviceConfig.ImageID,
		KeyPairName: s.serviceConfig.SSHKey,
		Networks: []*models.PVMInstanceAddNetwork{
			{
				NetworkID: &s.serviceConfig.NetworkID,
			}},
		Memory:     core.Float64Ptr(4),
		Processors: core.Float64Ptr(0.25),
		ProcType:   core.StringPtr("shared"),
		SysType:    "s922",
		UserData:   base64.StdEncoding.EncodeToString([]byte(userData)),
	}

	response, err := s.powervsService.CreateInstance(body)
	if err != nil {
		logger.Printf("failed to create an instance : %v", err)
		return nil, err
	}

	ins := (*response)[0]
	sandbox.vsi = *ins.PvmInstanceID
	vmState := ins.Status

	logger.Printf("created an instance %s for sandbox %s", *ins.ServerName, req.Id)
	var podNodeIPs []net.IP

	for *vmState != "ACTIVE" {
		in, err := s.powervsService.Get(*ins.PvmInstanceID)
		if err != nil {
			return nil, fmt.Errorf("failed to get the instance: %w", err)
		}
		logger.Printf("Current VM state: %s", *vmState)
		vmState = in.Status
	}
	logger.Printf("instance is in desired state: %s", *vmState)

	podNodeIPs, err = getVMIPs(ins, s.powervsService)
	if err != nil {
		return nil, fmt.Errorf("failed to get IPs for the instance : %w", err)
	}
	logger.Printf("IPs fetched for the instance")

	if err := s.workerNode.Setup(sandbox.netNSPath, podNodeIPs, sandbox.podNetworkConfig); err != nil {
		return nil, fmt.Errorf("failed to set up pod network tunnel on netns %s: %w", sandbox.netNSPath, err)
	}

	serverURL := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(podNodeIPs[0].String(), s.daemonPort),
		Path:   daemon.AgentURLPath,
	}
	logger.Printf("Service URL hostpath: %s%s", serverURL.Host, serverURL.Path)

	errCh := make(chan error)
	go func() {
		defer close(errCh)

		if err := sandbox.agentProxy.Start(context.Background(), serverURL); err != nil {
			logger.Printf("error running agent proxy: %v", err)
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		_ = sandbox.agentProxy.Shutdown()
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, err
	case <-sandbox.agentProxy.Ready():
	}

	logger.Printf("agent proxy is ready")
	return &pb.StartVMResponse{}, nil
}

func (s *hypervisorPVSService) getSandbox(id string) (*sandbox, error) {

	sid := sandboxID(id)

	if id == "" {
		return nil, errors.New("empty sandbox id")
	}
	s.Lock()
	defer s.Unlock()
	if _, exists := s.sandboxes[sid]; !exists {
		return nil, fmt.Errorf("sandbox %s does not exist", sid)
	}
	return s.sandboxes[sid], nil
}

func (s *hypervisorPVSService) deleteSandbox(id string) error {
	sid := sandboxID(id)
	if id == "" {
		return errors.New("empty sandbox id")
	}
	s.Lock()
	defer s.Unlock()
	delete(s.sandboxes, sid)
	return nil
}

func getVMIPs(instance *models.PVMInstance, service PowerVS) ([]net.IP, error) {
	var podNodeIPs []net.IP
	in, err := service.Get(*instance.PvmInstanceID)
	if err != nil {
		fmt.Println("failed to get the instance: %w", err)
	}
	for i, nic := range in.Networks {

		addr := &nic.IPAddress
		ip := net.ParseIP(*addr)
		if ip == nil {
			return nil, fmt.Errorf("failed to parse pod node IP %q", *addr)
		}

		podNodeIPs = append(podNodeIPs, ip)
		logger.Printf("podNodeIP[%d]=%s", i, ip.String())
	}

	return podNodeIPs, nil
}

func (s *hypervisorPVSService) deleteInstance(ctx context.Context, id string) error {

	err := s.powervsService.DeleteInstance(id)
	if err != nil {
		logger.Printf("failed to delete an instance: %v", err)
		return err
	}
	logger.Printf("deleted an instance %s", id)
	return nil
}

func (s *hypervisorPVSService) StopVM(ctx context.Context, req *pb.StopVMRequest) (*pb.StopVMResponse, error) {
	sandbox, err := s.getSandbox(req.Id)
	if err != nil {
		return nil, err
	}

	if err := sandbox.agentProxy.Shutdown(); err != nil {
		logger.Printf("failed to stop agent proxy: %v", err)
	}

	if err := s.deleteInstance(ctx, sandbox.vsi); err != nil {
		return nil, err
	}

	if err := s.workerNode.Teardown(sandbox.netNSPath, sandbox.podNetworkConfig); err != nil {
		return nil, fmt.Errorf("failed to tear down netns %s: %w", sandbox.netNSPath, err)
	}
	err = s.deleteSandbox(req.Id)
	if err != nil {
		return nil, err
	}
	return &pb.StopVMResponse{}, nil
}
