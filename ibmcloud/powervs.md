# Setup instructions

## Prerequisities
1. Kubernetes cluster configured with custom containerd

- Install containerd

Build from source
```
git clone -b CC-main https://github.com/confidential-containers/containerd.git
cd containerd
make && make install
```
    
Sample containerd configuration:
`/etc/containerd/config.toml`
```
version = 2
root = "/var/lib/containerd"
state = "/run/containerd"
oom_score = -999

[grpc]
  address = "/run/containerd/containerd.sock"
  uid = 0
  gid = 0

[debug]
  address = "/run/containerd/debug.sock"
  uid = 0
  gid = 0
  level = "debug"

[plugins]
  [plugins."io.containerd.runtime.v1.linux"]
    shim_debug = true
  [plugins."io.containerd.grpc.v1.cri"]
    [plugins."io.containerd.grpc.v1.cri".containerd]
      default_runtime_name = "runc"
      [plugins."io.containerd.grpc.v1.cri".containerd.runtimes]
        [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
          runtime_type = "io.containerd.runc.v2"
        [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.kata]
          runtime_type = "io.containerd.kata.v2"
          cri_handler = "cc"
```

2. After the cluster is setup, `ssh` into the worker node and install 
`containerd-kata-shim-v2`
```
git clone -b CCv0 https://github.com/kata-containers/kata-containers.git
cd kata-containers/src/runtime
make containerd-shim-v2 && make install-containerd-shim-v2
```

Sample kata-containers configuration:
`/etc/kata-containers/configuration.toml`
```
[runtime]
internetworking_model = "none"
disable_new_netns = true
disable_guest_seccomp = true
enable_pprof = true
enable_debug = true
[hypervisor.remote]
remote_hypervisor_socket = "/run/peerpod/hypervisor.sock"
remote_hypervisor_timeout = 1200
[agent.kata]
[image]
service_offload = true
```


## Running cloud-api-adaptor

```
./cloud-api-adaptor ibmcloud-powervs \
    -api-key ${IBMCLOUD_API_KEY} \
    -service-instance-id <> \
    -zone osa21 \
    -image-id <> \
    -network-id <> \
    -ssh-key <> \ 
    -pods-dir /run/peerpod/pods \
    -cri-runtime-endpoint /run/containerd/containerd.sock \
    -socket /run/peerpod/hypervisor.sock
```

## Deploy sample pod
```
kubectl apply -f ibmcloud/demo/nginx.yaml
```