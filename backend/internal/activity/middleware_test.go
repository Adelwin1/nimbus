package activity

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type memoryRecorder struct {
	inputs []RecordInput
}

func (r *memoryRecorder) Record(
	input RecordInput,
) {
	r.inputs = append(r.inputs, input)
}

func TestWrapHandlerRecordsSuccessfulAction(
	t *testing.T,
) {
	t.Parallel()

	recorder := &memoryRecorder{}

	handler := WrapHandler(
		recorder,
		HandlerOptions{
			Action:     ActionApplicationUpdated,
			EntityType: EntityApplication,
			Summary:    "Application was updated.",
		},
		func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			w.WriteHeader(http.StatusOK)
		},
	)

	request := httptest.NewRequest(
		http.MethodPatch,
		"http://example.com/apps/test",
		nil,
	)

	response := httptest.NewRecorder()

	handler(response, request)

	if len(recorder.inputs) != 1 {
		t.Fatalf(
			"recorded %d activities, expected 1",
			len(recorder.inputs),
		)
	}

	if recorder.inputs[0].Action !=
		ActionApplicationUpdated {
		t.Fatalf(
			"action = %q",
			recorder.inputs[0].Action,
		)
	}
}

func TestWrapHandlerSkipsFailedAction(
	t *testing.T,
) {
	t.Parallel()

	recorder := &memoryRecorder{}

	handler := WrapHandler(
		recorder,
		HandlerOptions{
			Action:     ActionApplicationDeleted,
			EntityType: EntityApplication,
			Summary:    "Application was deleted.",
		},
		func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			w.WriteHeader(
				http.StatusBadRequest,
			)
		},
	)

	request := httptest.NewRequest(
		http.MethodDelete,
		"http://example.com/apps/test",
		nil,
	)

	response := httptest.NewRecorder()

	handler(response, request)

	if len(recorder.inputs) != 0 {
		t.Fatalf(
			"recorded %d failed activities",
			len(recorder.inputs),
		)
	}
}
