package tracker

import (
	"time"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/utils"
)

// The builder struct to build a FairnessTracker
type FairnessTrackerBuilder struct {
	configuration *config.FairnessTrackerConfig
}

func NewFairnessTrackerBuilder() *FairnessTrackerBuilder {
	return &FairnessTrackerBuilder{
		configuration: config.DefaultFairnessTrackerConfig(),
	}
}

func (bl *FairnessTrackerBuilder) BuildWithDefaultConfig() (*FairnessTracker, error) {
	return NewFairnessTracker(config.DefaultFairnessTrackerConfig())
}

func (bl *FairnessTrackerBuilder) BuildWithConfig(configuration *config.FairnessTrackerConfig) (*FairnessTracker, error) {
	return NewFairnessTracker(configuration)
}

func (bl *FairnessTrackerBuilder) Build() (*FairnessTracker, error) {
	return NewFairnessTracker(bl.configuration)
}

func (bl *FairnessTrackerBuilder) SetL(L uint32) {
	bl.configuration.L = L
}

func (bl *FairnessTrackerBuilder) SetM(M uint32) {
	bl.configuration.M = M
}

func (bl *FairnessTrackerBuilder) SetPd(Pd float64) {
	bl.configuration.Pd = Pd
}

func (bl *FairnessTrackerBuilder) SetPi(Pi float64) {
	bl.configuration.Pi = Pi
}

func (bl *FairnessTrackerBuilder) SetLambda(Lambda float64) {
	bl.configuration.Lambda = Lambda
}

func (bl *FairnessTrackerBuilder) SetIncludeStats(IncludeStats bool) {
	bl.configuration.IncludeStats = IncludeStats
}

func (bl *FairnessTrackerBuilder) SetRotationFrequency(rotationFrequency time.Duration) {
	bl.configuration.RotationFrequency = rotationFrequency
}

// The public facing errors from the FairnessTracker
type FairnessTrackerError struct {
	*utils.BaseError
}

func NewFairnessTrackerError(wrapped error, msg string, args ...any) *FairnessTrackerError {
	return &FairnessTrackerError{
		BaseError: utils.NewBaseError(wrapped, msg, args...),
	}
}
