package nomad

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/hashicorp/nomad/api"
)

// QueueWorkload is one job waiting in or being placed from a node pool's batch job queue.
type QueueWorkload struct {
	JobID               string
	Namespace           string
	Position            int
	Status              string // "placing", "queued"
	AdjustedPriority    int
	BasePriority        int
	FairshareAdjustment int
	AgeAdjustment       int
	CPUAdjustment       int
	MemoryAdjustment    int
	Tenant              string
	CreatedAt           time.Time
}

// BatchQueueJobs lists the workloads waiting in or being placed from the batch
// job queue of a node pool. It returns an empty list when the cluster has no
// batch queue support (Nomad < 2.1) or the token has no list-jobs capability.
func (c *Client) BatchQueueJobs(ctx context.Context, pool string) ([]QueueWorkload, error) {
	if pool == "" {
		return nil, nil
	}

	opts := c.query(ctx, "")
	opts.Params = map[string]string{
		"node_pool": pool,
		"sort":      "priority",
	}

	resp, _, err := c.api.BatchQueue().Jobs(opts)
	if err != nil {
		if answered(err, http.StatusNotFound) || answered(err, http.StatusForbidden) {
			return nil, nil
		}

		return nil, err
	}

	if resp == nil {
		return nil, nil
	}

	return decodeWorkloads(resp.Type, resp.Workloads)
}

func decodeWorkloads(queueType api.BatchQueueType, raw any) ([]QueueWorkload, error) {
	if raw == nil {
		return nil, nil
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	switch queueType {
	case api.BatchQueueTypeDynamic:
		return decodeDynamicWorkloads(data)
	case api.BatchQueueTypeFifo:
		return decodeFifoWorkloads(data)
	default:
		return nil, nil
	}
}

func decodeDynamicWorkloads(data []byte) ([]QueueWorkload, error) {
	var items []struct {
		JobID               string `json:"JobID"`
		Namespace           string `json:"Namespace"`
		Tenant              string `json:"Tenant"`
		Status              string `json:"Status"`
		Position            int    `json:"Position"`
		AdjustedPriority    int    `json:"AdjustedPriority"`
		BasePriority        int    `json:"BasePriority"`
		FairshareAdjustment int    `json:"FairshareAdjustment"`
		AgeAdjustment       int    `json:"AgeAdjustment"`
		CPUAdjustment       int    `json:"CpuAdjustment"`
		MemoryAdjustment    int    `json:"MemoryAdjustment"`
		CreatedAt           int64  `json:"CreatedAt"`
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}

	out := make([]QueueWorkload, 0, len(items))
	for _, it := range items {
		out = append(out, QueueWorkload{
			JobID:               it.JobID,
			Namespace:           it.Namespace,
			Position:            it.Position,
			Status:              it.Status,
			AdjustedPriority:    it.AdjustedPriority,
			BasePriority:        it.BasePriority,
			FairshareAdjustment: it.FairshareAdjustment,
			AgeAdjustment:       it.AgeAdjustment,
			CPUAdjustment:       it.CPUAdjustment,
			MemoryAdjustment:    it.MemoryAdjustment,
			Tenant:              it.Tenant,
			CreatedAt:           unixCreatedAt(it.CreatedAt),
		})
	}

	return out, nil
}

func decodeFifoWorkloads(data []byte) ([]QueueWorkload, error) {
	var items []struct {
		JobID     string `json:"JobID"`
		Namespace string `json:"Namespace"`
		Position  int    `json:"Position"`
		Status    string `json:"Status"`
		CreatedAt int64  `json:"CreatedAt"`
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}

	out := make([]QueueWorkload, 0, len(items))
	for _, it := range items {
		out = append(out, QueueWorkload{
			JobID:     it.JobID,
			Namespace: it.Namespace,
			Position:  it.Position,
			Status:    it.Status,
			CreatedAt: unixCreatedAt(it.CreatedAt),
		})
	}

	return out, nil
}

func unixCreatedAt(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	if v > 1e12 {
		return time.Unix(0, v)
	}

	return time.Unix(v, 0)
}
