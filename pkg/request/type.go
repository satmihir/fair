package request

import "context"

// Outcome represents the result of a request for resource allocation.
// It is used to adjust throttling probabilities for future requests.
type Outcome int

const (
	// OutcomeSuccess means the request managed to obtain the resource.
	OutcomeSuccess Outcome = iota

	// OutcomeFailure means the request failed to obtain the resource.
	// This may not always map to a failure in your business logic. For
	// example - failing to validate a request or failing to reach an
	// upstream service because of a network error would not qualify
	// as a failure here. See ReportOutcome function for when to report.
	OutcomeFailure
)

// RegisterRequestResult is returned from RegisterRequest and indicates whether
// the request should be throttled.
type RegisterRequestResult struct {
	// If true, this request should be throttled
	ShouldThrottle bool
	// Probabilities and other useful debugging information
	ResultStats *ResultStats
}

// ResultStats contains probabilities and other debugging information collected
// while registering a request.
type ResultStats struct {
	// The final probability used to make the throttling decision
	FinalProbability float64
	// The chosen bucket index at every level
	BucketIndexes []int
	// The probabilities of the chosen buckets
	BucketProbabilities []float64
}

// ReportOutcomeResult is returned from ReportOutcome. It currently carries no
// fields but exists for future expansion.
type ReportOutcomeResult struct{}

// Tracker defines the operations required by the underlying data structure used
// to make throttling decisions.
type Tracker interface {
	// Return the int ID of this structure. Used for implementing moving hashes.
	GetID() uint64

	// Register an incoming request from a client identified by a clientIdentifier
	// The clientIdentifier needs to be unique and consistent for every client as
	// it will be used to hash and locate the corresponding buckets.
	RegisterRequest(ctx context.Context, clientIdentifier []byte) *RegisterRequestResult

	// Report the outcome of a request from the given client so we can update the
	// probabilities of the corresponding buckets.
	// Only report the outcomes on the requests where you could either conclusively
	// get the resource or not. For outcomes such as user errors or network failures
	// or timeout with upstream, do NOT report any outcome, or we may wrongly throttle
	// requests based on things not related to resource contention.
	// You don't have to report an outcome to every registered request.
	ReportOutcome(ctx context.Context, clientIdentifier []byte, outcome Outcome) *ReportOutcomeResult

	// Close this tracker when shutting down
	Close()
}
