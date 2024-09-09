package request

import "context"

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

// The response object of the RegisterRequest function
type RegisterRequestResult struct {
	// If true, this request should be throttled
	ShouldThrottle bool
}

// The response object of the ReportOutcome function
type ReportOutcomeResult struct{}

// The data struecture interface
type Tracker interface {
	// Return the int ID of this structure. Used for implementing moving hashes.
	GetId() uint64

	// Register an incoming request from a client identified by a clientIdentifier
	// The clientIdentifier needs to be unique and consistent for every client as
	// it will be used to hash and locate the corresponding buckets.
	RegisterRequest(ctx context.Context, clientIdentifier []byte) (*RegisterRequestResult, error)

	// Report the outcome of a requests from the given client so we can update the
	// probabilities of the corresponding buckets.
	// Only report the outcomes on the requests where you could either conclusively
	// get the resource or not. For outcomes such as user errors or network failures
	// or timeout with upstream, do NOT report any outcome or we may wrongly throttle
	// requests based on things not related to resource contention.
	// You don't have to report an outcome to every registered request.
	ReportOutcome(ctx context.Context, clientIdentifier []byte, outcome Outcome) (*ReportOutcomeResult, error)

	// Close this tracker when shutting down
	Close()
}
