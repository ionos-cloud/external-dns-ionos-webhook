package ionos

import (
	"context"
	"errors"
	"fmt"
)

var ErrZoneNotFound = errors.New("zone not found")

// RetryLoadZones is a helper function to retry the API call and reload zones if a zone is not found
// this is useful in the unlikely but possible case where a zone has been deleted and recreated with a different ID
func RetryLoadZones[T any](
	ctx context.Context,
	setupZones func(context.Context) error,
	call func() (T, error),
) (T, error) {
	var zero T
	res, err := call()
	if err != nil && errors.Is(err, ErrZoneNotFound) {
		if setupErr := setupZones(ctx); setupErr != nil {
			return zero, fmt.Errorf("failed to load zones: %w", setupErr)
		}
		return call()
	}
	return res, err
}
