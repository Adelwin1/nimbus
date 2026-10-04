package monitoring

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

var ErrCheckInProgress = errors.New("health check already in progress")

type Service struct {
	repository *Repository
	checker    *Checker
	inFlight   sync.Map
}

type RunSummary struct {
	Attempted int
	Completed int
	Failed    int
	Skipped   int
}

func NewService(
	repository *Repository,
	checker *Checker,
) *Service {
	return &Service{
		repository: repository,
		checker:    checker,
	}
}

func (s *Service) CheckNow(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) (HealthCheck, ApplicationState, error) {
	target, err := s.repository.GetCheckTarget(
		ctx,
		applicationID,
		userID,
	)
	if err != nil {
		return HealthCheck{}, ApplicationState{}, err
	}

	if !s.acquire(applicationID) {
		return HealthCheck{}, ApplicationState{},
			ErrCheckInProgress
	}
	defer s.release(applicationID)

	return s.checkAndStore(ctx, target)
}

func (s *Service) GetHistory(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
	limit int,
	offset int,
) (HistoryPage, error) {
	if limit <= 0 {
		limit = 25
	}

	if limit > 100 {
		limit = 100
	}

	if offset < 0 {
		offset = 0
	}

	return s.repository.ListHistory(
		ctx,
		applicationID,
		userID,
		limit,
		offset,
	)
}

func (s *Service) GetOverview(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) (HealthOverview, error) {
	return s.repository.GetOverview(
		ctx,
		applicationID,
		userID,
	)
}

func (s *Service) RunDueChecks(
	ctx context.Context,
	limit int,
	workerCount int,
) (RunSummary, error) {
	applications, err := s.repository.FindDueApplications(
		ctx,
		limit,
	)
	if err != nil {
		return RunSummary{}, err
	}

	summary := RunSummary{
		Attempted: len(applications),
	}

	if len(applications) == 0 {
		return summary, nil
	}

	if workerCount <= 0 {
		workerCount = 4
	}

	if workerCount > len(applications) {
		workerCount = len(applications)
	}

	jobs := make(chan DueApplication)

	var waitGroup sync.WaitGroup
	var summaryMutex sync.Mutex

	worker := func() {
		defer waitGroup.Done()

		for application := range jobs {
			if !s.acquire(application.ID) {
				summaryMutex.Lock()
				summary.Skipped++
				summaryMutex.Unlock()
				continue
			}

			_, _, checkErr := s.checkAndStore(
				ctx,
				application,
			)

			s.release(application.ID)

			summaryMutex.Lock()

			if checkErr != nil {
				summary.Failed++
			} else {
				summary.Completed++
			}

			summaryMutex.Unlock()
		}
	}

	waitGroup.Add(workerCount)

	for index := 0; index < workerCount; index++ {
		go worker()
	}

	for _, application := range applications {
		select {
		case jobs <- application:
		case <-ctx.Done():
			close(jobs)
			waitGroup.Wait()

			return summary, ctx.Err()
		}
	}

	close(jobs)
	waitGroup.Wait()

	return summary, nil
}

func (s *Service) checkAndStore(
	ctx context.Context,
	application DueApplication,
) (HealthCheck, ApplicationState, error) {

	rules, err := s.repository.LoadCheckRules(ctx, application.ID)
	if err != nil {
		return HealthCheck{}, ApplicationState{}, err
	}
	result := s.checker.Check(
		ctx,
		application.HealthURL,
		application.LatencyThresholdMS,
		rules,
	)

	addTroubleshooting(&result)

	check, state, err := s.repository.StoreResult(
		ctx,
		application.ID,
		result,
	)
	if err != nil {
		return HealthCheck{}, ApplicationState{},
			fmt.Errorf("store monitoring result: %w", err)
	}

	return check, state, nil
}

func (s *Service) acquire(applicationID uuid.UUID) bool {
	_, alreadyRunning := s.inFlight.LoadOrStore(
		applicationID,
		struct{}{},
	)

	return !alreadyRunning
}

func (s *Service) release(applicationID uuid.UUID) {
	s.inFlight.Delete(applicationID)
}
