package data

import (
	"context"

	"github.com/satmihir/fair/pkg/utils"
)

// The enum for outcome for a request
type Outcome int

const (
	// The success outcome means the request managed to get the resource
	OutcomeSuccess Outcome = iota

	// The failure outcome means the request failed to get the resource
	// This may not always map to a failure in your business logic. For
	// example - failing to validate a request or failing to reach an
	// upstream service because of a network error would not qualify
	// as a failure here. See ReportOutcome function for when to report.
	OutcomeFailure
)

// The data struecture interface
type IStructure interface {
	// Return the int ID of this structure. Used for implementing moving hashes.
	GetId() uint32

	// Register an incoming request from a client identified by a clientIdentifier
	// The clientIdentifier needs to be unique and consistent for every client as
	// it will be used to hash and locate the corresponding buckets.
	RegisterRequest(ctx context.Context, clientIdentifier []byte) (*RegisterResponse, error)

	// Report the outcome of a requests from the given client so we can update the
	// probabilities of the corresponding buckets.
	// Only report the outcomes on the requests where you could either conclusively
	// get the resource or not. For outcomes such as user errors or network failures
	// or timeout with upstream, do NOT report any outcome or we may wrongly throttle
	// requests based on things not related to resource contention.
	// You don't have to report an outcome to every registered request.
	ReportOutcome(ctx context.Context, clientIdentifier []byte, outcome Outcome) error
}

// The response object of the RegisterRequest function
type RegisterResponse struct {
	// If true, this request should be throttled
	ShouldThrottle bool
}

// The umbrella error for this package
type DataError struct {
	*utils.BaseError
}

func NewDataError(wrapped error, msg string, args ...any) *DataError {
	return &DataError{
		BaseError: utils.NewBaseError(wrapped, msg, args...),
	}
}
