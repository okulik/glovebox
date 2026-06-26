package state

import (
	"context"

	"github.com/okulik/glovebox/internal/config"
	"github.com/okulik/glovebox/internal/dockerx"
)

// Reconcile walks the store's persisted records and ensures the corresponding
// Docker resources exist.
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
		agentName := config.ContainerAgentPrefix + pid
		if id, _, _ := dk.ContainerByName(ctx, agentName); id != "" {
			_ = dk.ConnectNetwork(ctx, agentName, plan.NetworkName)
		}
	}
	return nil
}
