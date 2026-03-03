package session

import (
	"context"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	gpudmanager "github.com/NVIDIA/fleet-intelligence-sdk/pkg/gpud-manager"
)

// processPackageStatus handles the packageStatus request
func (s *Session) processPackageStatus(ctx context.Context, response *Response) {
	packageStatus, err := gpudmanager.GlobalController.Status(ctx)
	if err != nil {
		response.Error = err.Error()
		return
	}
	var result []apiv1.PackageStatus
	for _, currPackage := range packageStatus {
		packagePhase := apiv1.UnknownPhase
		if currPackage.IsInstalled {
			packagePhase = apiv1.InstalledPhase
		} else if currPackage.Installing {
			packagePhase = apiv1.InstallingPhase
		}
		status := "Unhealthy"
		if currPackage.Status {
			status = "Healthy"
		}
		result = append(result, apiv1.PackageStatus{
			Name:           currPackage.Name,
			Phase:          packagePhase,
			Status:         status,
			CurrentVersion: currPackage.CurrentVersion,
		})
	}
	response.PackageStatus = result
}
