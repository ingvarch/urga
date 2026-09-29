package nomad

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/nomad/api"
)

// SchedulerConfig is how the scheduler of the cluster places work.
type SchedulerConfig struct {
	// Algorithm is binpack or spread.
	Algorithm string

	// Preempt says a job of the type may take the place of a lower one.
	PreemptSystem, PreemptSysBatch, PreemptBatch, PreemptService bool

	MemoryOversubscription bool

	// RejectJobRegistration and PauseEvalBroker stop new work from coming
	// in, or from being scheduled.
	RejectJobRegistration, PauseEvalBroker bool
}

// ErrSchedulerChanged is a configuration saved over one that changed since
// it was read: the cluster keeps the newer one.
var ErrSchedulerChanged = errors.New("the scheduler configuration changed since it was read")

// Scheduler reads how the scheduler of the cluster places work.
func (c *Client) Scheduler(ctx context.Context) (SchedulerConfig, error) {
	config, err := c.scheduler(ctx)
	if err != nil {
		return SchedulerConfig{}, err
	}

	return SchedulerConfig{
		Algorithm:              string(config.SchedulerAlgorithm),
		PreemptSystem:          config.PreemptionConfig.SystemSchedulerEnabled,
		PreemptSysBatch:        config.PreemptionConfig.SysBatchSchedulerEnabled,
		PreemptBatch:           config.PreemptionConfig.BatchSchedulerEnabled,
		PreemptService:         config.PreemptionConfig.ServiceSchedulerEnabled,
		MemoryOversubscription: config.MemoryOversubscriptionEnabled,
		RejectJobRegistration:  config.RejectJobRegistration,
		PauseEvalBroker:        config.PauseEvalBroker,
	}, nil
}

// SchedulerSpec is the configuration of the scheduler as a file, with the
// index it was read at.
func (c *Client) SchedulerSpec(ctx context.Context) (string, error) {
	config, err := c.scheduler(ctx)
	if err != nil {
		return "", err
	}

	return asJSON(config)
}

// SubmitScheduler sends the configuration of the scheduler back, over the
// one it was read from: a configuration that changed since is not
// overwritten.
func (c *Client) SubmitScheduler(ctx context.Context, source string) error {
	var config api.SchedulerConfiguration
	if err := json.Unmarshal([]byte(source), &config); err != nil {
		return fmt.Errorf("the scheduler configuration is not valid JSON: %w", err)
	}

	answer, _, err := c.api.Operator().SchedulerCASConfiguration(&config, c.write(ctx, ""))
	if err != nil {
		return err
	}

	if !answer.Updated {
		return ErrSchedulerChanged
	}

	return nil
}

func (c *Client) scheduler(ctx context.Context) (*api.SchedulerConfiguration, error) {
	answer, _, err := c.api.Operator().SchedulerGetConfiguration(c.query(ctx, ""))
	if err != nil {
		return nil, err
	}

	return answer.SchedulerConfig, nil
}
