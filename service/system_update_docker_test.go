package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type systemUpdateDockerRoundTripper struct {
	mu        sync.Mutex
	responses []systemUpdateDockerRoundTripResponse
	calls     int
}

type systemUpdateDockerRoundTripResponse struct {
	statusCode int
	body       string
	err        error
}

func (r *systemUpdateDockerRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if len(r.responses) == 0 {
		return nil, errors.New("no Docker response configured")
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	if response.err != nil {
		return nil, response.err
	}
	return &http.Response{
		StatusCode: response.statusCode,
		Body:       io.NopCloser(bytes.NewBufferString(response.body)),
		Header:     make(http.Header),
	}, nil
}

func (r *systemUpdateDockerRoundTripper) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestDockerPullRetriesTransientRegistryFailure(t *testing.T) {
	transport := &systemUpdateDockerRoundTripper{
		responses: []systemUpdateDockerRoundTripResponse{
			{
				statusCode: http.StatusInternalServerError,
				body:       `{"message":"Get \"https://registry-1.docker.io/v2/\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)"}`,
			},
			{
				statusCode: http.StatusOK,
				body:       `{"status":"Downloaded"}` + "\n",
			},
		},
	}
	client := &dockerEngineClient{
		httpClient:      &http.Client{Transport: transport},
		pullRetryDelays: []time.Duration{0},
	}

	err := client.pullImage(context.Background(), "c1cadabob/nexustok:latest", nil)

	require.NoError(t, err)
	assert.Equal(t, 2, transport.callCount())
}

func TestDockerPullDoesNotRetryPermanentRegistryFailure(t *testing.T) {
	transport := &systemUpdateDockerRoundTripper{
		responses: []systemUpdateDockerRoundTripResponse{
			{
				statusCode: http.StatusInternalServerError,
				body:       `{"message":"pull access denied for c1cadabob/nexustok, repository does not exist or may require authorization"}`,
			},
		},
	}
	client := &dockerEngineClient{
		httpClient:      &http.Client{Transport: transport},
		pullRetryDelays: []time.Duration{0, 0},
	}

	err := client.pullImage(context.Background(), "c1cadabob/nexustok:latest", nil)

	require.Error(t, err)
	assert.Equal(t, 1, transport.callCount())
	assert.NotContains(t, err.Error(), "awaiting headers")
}

func TestFormatDockerImagePullErrorExplainsRegistryTimeout(t *testing.T) {
	err := formatDockerImagePullError(
		"c1cadabob/nexustok:latest",
		&dockerPullError{
			statusCode: http.StatusInternalServerError,
			message:    `{"message":"Get \"https://registry-1.docker.io/v2/\": Client.Timeout exceeded while awaiting headers"}`,
		},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "host network and DNS")
	assert.Contains(t, err.Error(), "Docker registry mirror")
	assert.Contains(t, err.Error(), "registry-1.docker.io")
}

func TestWaitForDockerPullRetryStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForDockerPullRetry(ctx, time.Second)

	require.ErrorIs(t, err, context.Canceled)
}
