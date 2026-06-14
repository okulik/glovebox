package state

import (
	"context"

	"github.com/okulik/glovebox/internal/dockerx"
)

// Reconcile walks the store's persisted records and ensures the corresponding
// Docker resources exist. Missing networks/volumes are created and missing
// containers are recreated and started. Containers that are already present
// are left alone (whether running or stopped).
//
// For each project, the per-project agent container (glovebox-agent-<pid>) is
// (re-)attached to the project's stack network so DNS to services works across
// controller restarts. If the agent container is not yet present, the attach
// is skipped silently.
func Reconcile(ctx context.Context, s *Store, dk dockerx.ControllerClient) error {
	for pid, rec := range s.All() {
		if rec.Manifest == nil {
			continue
		}
		plan := dockerx.Plan(pid, rec.Manifest)
		if err := dk.EnsureNetwork(ctx, plan.NetworkName, true); err != nil {
			return err
		}
		for _, v := range plan.Volumes {
			_ = dk.EnsureVolume(ctx, v)
		}
		for _, c := range plan.Containers {
			id, _, _ := dk.ContainerByName(ctx, c.Name)
			if id == "" {
				_ = dk.PullImage(ctx, c.Image)
				newID, err := dk.CreateContainer(ctx, c, plan.NetworkName)
				if err != nil {
					return err
				}
				_ = dk.StartContainer(ctx, newID)
			}
		}
		agentName := "glovebox-agent-" + pid
		if id, _, _ := dk.ContainerByName(ctx, agentName); id != "" {
			_ = dk.ConnectNetwork(ctx, agentName, plan.NetworkName)
		}
	}
	return nil
}
