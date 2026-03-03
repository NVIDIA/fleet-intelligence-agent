// Package all contains all the components.
package all

import (
	"github.com/NVIDIA/fleet-intelligence-sdk/components"

	componentsacceleratornvidiaclockspeed "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/clock-speed"
	componentsacceleratornvidiadcgmclock "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/clock"
	componentsacceleratornvidiadcgmcpu "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/cpu"
	componentsacceleratornvidiadcgminforom "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/inforom"
	componentsacceleratornvidiadcgmmem "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/mem"
	componentsacceleratornvidiadcgmnvlink "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/nvlink"
	componentsacceleratornvidiadcgmnvswitch "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/nvswitch"
	componentsacceleratornvidiadcgmpcie "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/pcie"
	componentsacceleratornvidiadcgmpower "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/power"
	componentsacceleratornvidiadcgmprof "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/prof"
	componentsacceleratornvidiadcgmthermal "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/thermal"
	componentsacceleratornvidiadcgmutilization "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/utilization"
	componentsacceleratornvidiadcgmxid "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/dcgm/xid"
	componentsacceleratornvidiaecc "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/ecc"
	componentsacceleratornvidiafabricmanager "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/fabric-manager"
	componentsacceleratornvidiagpm "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/gpm"
	componentsacceleratornvidiagpucounts "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/gpu-counts"
	componentsacceleratornvidiahwslowdown "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/hw-slowdown"
	componentsacceleratornvidiainfiniband "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/infiniband"
	componentsacceleratornvidiamemory "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/memory"
	componentsacceleratornvidianccl "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/nccl"
	componentsacceleratornvidianvlink "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/nvlink"
	componentsacceleratornvidiapeermem "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/peermem"
	componentsacceleratornvidiapersistencemode "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/persistence-mode"
	componentsacceleratornvidiapower "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/power"
	componentsacceleratornvidiaprocesses "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/processes"
	componentsacceleratornvidiaremappedrows "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/remapped-rows"
	componentsacceleratornvidiasxid "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/sxid"
	componentsacceleratornvidiatemperature "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/temperature"
	componentsacceleratornvidiautilization "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/utilization"
	componentsacceleratornvidiaxid "github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/xid"
	componentscontainerd "github.com/NVIDIA/fleet-intelligence-sdk/components/containerd"
	componentscpu "github.com/NVIDIA/fleet-intelligence-sdk/components/cpu"
	componentsdisk "github.com/NVIDIA/fleet-intelligence-sdk/components/disk"
	componentsdocker "github.com/NVIDIA/fleet-intelligence-sdk/components/docker"
	componentsfuse "github.com/NVIDIA/fleet-intelligence-sdk/components/fuse"
	componentskernelmodule "github.com/NVIDIA/fleet-intelligence-sdk/components/kernel-module"
	componentskubelet "github.com/NVIDIA/fleet-intelligence-sdk/components/kubelet"
	componentslibrary "github.com/NVIDIA/fleet-intelligence-sdk/components/library"
	componentsmemory "github.com/NVIDIA/fleet-intelligence-sdk/components/memory"
	componentsnetworkethernet "github.com/NVIDIA/fleet-intelligence-sdk/components/network/ethernet"
	componentsnetworklatency "github.com/NVIDIA/fleet-intelligence-sdk/components/network/latency"
	componentsnfs "github.com/NVIDIA/fleet-intelligence-sdk/components/nfs"
	componentsos "github.com/NVIDIA/fleet-intelligence-sdk/components/os"
	componentspci "github.com/NVIDIA/fleet-intelligence-sdk/components/pci"
	componentstailscale "github.com/NVIDIA/fleet-intelligence-sdk/components/tailscale"
)

type Component struct {
	Name     string
	InitFunc components.InitFunc
}

func All() []Component {
	return componentInits
}

var componentInits = []Component{
	{Name: componentsacceleratornvidiaclockspeed.Name, InitFunc: componentsacceleratornvidiaclockspeed.New},
	// DCGM components
	{Name: componentsacceleratornvidiadcgmclock.Name, InitFunc: componentsacceleratornvidiadcgmclock.New},
	{Name: componentsacceleratornvidiadcgmcpu.Name, InitFunc: componentsacceleratornvidiadcgmcpu.New},
	{Name: componentsacceleratornvidiadcgminforom.Name, InitFunc: componentsacceleratornvidiadcgminforom.New},
	{Name: componentsacceleratornvidiadcgmmem.Name, InitFunc: componentsacceleratornvidiadcgmmem.New},
	{Name: componentsacceleratornvidiadcgmprof.Name, InitFunc: componentsacceleratornvidiadcgmprof.New},
	{Name: componentsacceleratornvidiadcgmnvlink.Name, InitFunc: componentsacceleratornvidiadcgmnvlink.New},
	{Name: componentsacceleratornvidiadcgmnvswitch.Name, InitFunc: componentsacceleratornvidiadcgmnvswitch.New},
	{Name: componentsacceleratornvidiadcgmpcie.Name, InitFunc: componentsacceleratornvidiadcgmpcie.New},
	{Name: componentsacceleratornvidiadcgmpower.Name, InitFunc: componentsacceleratornvidiadcgmpower.New},
	{Name: componentsacceleratornvidiadcgmthermal.Name, InitFunc: componentsacceleratornvidiadcgmthermal.New},
	{Name: componentsacceleratornvidiadcgmutilization.Name, InitFunc: componentsacceleratornvidiadcgmutilization.New},
	{Name: componentsacceleratornvidiadcgmxid.Name, InitFunc: componentsacceleratornvidiadcgmxid.New},
	// NVML components
	{Name: componentsacceleratornvidiaecc.Name, InitFunc: componentsacceleratornvidiaecc.New},
	{Name: componentsacceleratornvidiafabricmanager.Name, InitFunc: componentsacceleratornvidiafabricmanager.New},
	{Name: componentsacceleratornvidiagpm.Name, InitFunc: componentsacceleratornvidiagpm.New},
	{Name: componentsacceleratornvidiagpucounts.Name, InitFunc: componentsacceleratornvidiagpucounts.New},
	{Name: componentsacceleratornvidiahwslowdown.Name, InitFunc: componentsacceleratornvidiahwslowdown.New},
	{Name: componentsacceleratornvidiainfiniband.Name, InitFunc: componentsacceleratornvidiainfiniband.New},
	{Name: componentsacceleratornvidiamemory.Name, InitFunc: componentsacceleratornvidiamemory.New},
	{Name: componentsacceleratornvidianccl.Name, InitFunc: componentsacceleratornvidianccl.New},
	{Name: componentsacceleratornvidianvlink.Name, InitFunc: componentsacceleratornvidianvlink.New},
	{Name: componentsacceleratornvidiapeermem.Name, InitFunc: componentsacceleratornvidiapeermem.New},
	{Name: componentsacceleratornvidiapersistencemode.Name, InitFunc: componentsacceleratornvidiapersistencemode.New},
	{Name: componentsacceleratornvidiapower.Name, InitFunc: componentsacceleratornvidiapower.New},
	{Name: componentsacceleratornvidiaprocesses.Name, InitFunc: componentsacceleratornvidiaprocesses.New},
	{Name: componentsacceleratornvidiaremappedrows.Name, InitFunc: componentsacceleratornvidiaremappedrows.New},
	{Name: componentsacceleratornvidiasxid.Name, InitFunc: componentsacceleratornvidiasxid.New},
	{Name: componentsacceleratornvidiatemperature.Name, InitFunc: componentsacceleratornvidiatemperature.New},
	{Name: componentsacceleratornvidiautilization.Name, InitFunc: componentsacceleratornvidiautilization.New},
	{Name: componentsacceleratornvidiaxid.Name, InitFunc: componentsacceleratornvidiaxid.New},
	// System components
	{Name: componentscontainerd.Name, InitFunc: componentscontainerd.New},
	{Name: componentscpu.Name, InitFunc: componentscpu.New},
	{Name: componentsdisk.Name, InitFunc: componentsdisk.New},
	{Name: componentsdocker.Name, InitFunc: componentsdocker.New},
	{Name: componentsfuse.Name, InitFunc: componentsfuse.New},
	{Name: componentskernelmodule.Name, InitFunc: componentskernelmodule.New},
	{Name: componentskubelet.Name, InitFunc: componentskubelet.New},
	{Name: componentslibrary.Name, InitFunc: componentslibrary.New},
	{Name: componentsmemory.Name, InitFunc: componentsmemory.New},
	{Name: componentsnetworkethernet.Name, InitFunc: componentsnetworkethernet.New},
	{Name: componentsnetworklatency.Name, InitFunc: componentsnetworklatency.New},
	{Name: componentsnfs.Name, InitFunc: componentsnfs.New},
	{Name: componentsos.Name, InitFunc: componentsos.New},
	{Name: componentspci.Name, InitFunc: componentspci.New},
	{Name: componentstailscale.Name, InitFunc: componentstailscale.New},
}
