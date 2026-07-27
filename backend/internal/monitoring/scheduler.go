package monitoring

import (
	"context"
	"log/slog"
	"time"
)

type Scheduler struct {
	service     *Service
	logger      *slog.Logger
	interval    time.Duration
	batchSize   int
	workerCount int
}

func NewScheduler(
	service *Service,
	logger *slog.Logger,
	interval time.Duration,
) *Scheduler {
	if interval <= 0 {
		interval = 15 * time.Second
	}

	return &Scheduler{
		service:     service,
		logger:      logger,
		interval:    interval,
		batchSize:   25,
		workerCount: 4,
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	s.logger.Info(
		"monitoring scheduler started",
		"interval", s.interval.String(),
	)

	s.runOnce(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runOnce(ctx)

		case <-ctx.Done():
			s.logger.Info("monitoring scheduler stopped")
			return
		}
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	startedAt := time.Now()

	summary, err := s.service.RunDueChecks(
		ctx,
		s.batchSize,
		s.workerCount,
	)
	if err != nil {
		if ctx.Err() != nil {
			return
		}

		s.logger.Error(
			"monitoring scheduler cycle failed",
			"error", err,
		)
		return
	}

	if summary.Attempted == 0 {
		return
	}

	s.logger.Info(
		"monitoring scheduler cycle completed",
		"attempted", summary.Attempted,
		"completed", summary.Completed,
		"failed", summary.Failed,
		"skipped", summary.Skipped,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
}
