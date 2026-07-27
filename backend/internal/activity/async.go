package activity

import (
	"context"
	"log/slog"
)

const defaultQueueSize = 512

type BufferedRecorder struct {
	sink   Recorder
	logger *slog.Logger
	queue  chan RecordInput
}

func NewBufferedRecorder(
	ctx context.Context,
	sink Recorder,
	logger *slog.Logger,
	queueSize int,
) *BufferedRecorder {
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}

	recorder := &BufferedRecorder{
		sink:   sink,
		logger: logger,
		queue:  make(chan RecordInput, queueSize),
	}

	go recorder.run(ctx)

	return recorder
}

func (r *BufferedRecorder) Record(
	input RecordInput,
) {
	if r == nil || r.sink == nil {
		return
	}

	select {
	case r.queue <- input:
	default:
		if r.logger != nil {
			r.logger.Warn(
				"activity queue is full",
				"action",
				RedactText(input.Action),
				"entity_type",
				RedactText(input.EntityType),
			)
		}
	}
}

func (r *BufferedRecorder) run(
	ctx context.Context,
) {
	for {
		select {
		case input := <-r.queue:
			r.sink.Record(input)

		case <-ctx.Done():
			r.drain()
			return
		}
	}
}

func (r *BufferedRecorder) drain() {
	for {
		select {
		case input := <-r.queue:
			r.sink.Record(input)

		default:
			return
		}
	}
}
