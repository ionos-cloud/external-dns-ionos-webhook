package ionos

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryLoadZones_SuccessWithoutRetry(t *testing.T) {
	ctx := context.Background()
	setupZones := func(ctx context.Context) error {
		return nil
	}

	// Mock call function that succeeds without retry
	call := func() (string, error) {
		return "success", nil
	}

	result, err := RetryLoadZones(ctx, setupZones, call)
	require.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestRetryLoadZones_SuccessWithRetry(t *testing.T) {
	ctx := context.Background()

	setupZonesCalled := false
	setupZones := func(ctx context.Context) error {
		setupZonesCalled = true
		return nil
	}

	// Mock call function that fails first and succeeds on retry
	callCount := 0
	call := func() (string, error) {
		callCount++
		if callCount == 1 {
			return "", ErrZoneNotFound
		}
		return "retried success", nil
	}

	result, err := RetryLoadZones(ctx, setupZones, call)
	require.NoError(t, err)
	assert.Equal(t, "retried success", result)
	assert.True(t, setupZonesCalled)
}

func TestRetryLoadZones_FailureOnSetupZones(t *testing.T) {
	ctx := context.Background()
	randomErr := errors.New("random error")
	// Mock setupZones function that fails
	setupZones := func(ctx context.Context) error {
		return randomErr
	}

	call := func() (string, error) {
		return "", ErrZoneNotFound
	}

	result, err := RetryLoadZones(ctx, setupZones, call)
	require.Error(t, err)
	assert.ErrorIs(t, err, randomErr)
	assert.Empty(t, result)
}

func TestRetryLoadZones_FailureOnRetry(t *testing.T) {
	ctx := context.Background()

	setupZones := func(ctx context.Context) error {
		return nil
	}

	// Mock call function that keeps failing
	call := func() (string, error) {
		return "", ErrZoneNotFound
	}

	result, err := RetryLoadZones(ctx, setupZones, call)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrZoneNotFound)
	assert.Empty(t, result)
}
