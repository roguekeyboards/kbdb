package moderation

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/stretchr/testify/suite"
)

type DecisionSuite struct {
	suite.Suite
	logs     bytes.Buffer
	previous *slog.Logger
}

func TestDecisionSuite(t *testing.T) {
	suite.Run(t, new(DecisionSuite))
}

func (s *DecisionSuite) SetupTest() {
	s.logs.Reset()
	s.previous = slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&s.logs, nil)))
}

func (s *DecisionSuite) TearDownTest() {
	slog.SetDefault(s.previous)
}

func label(name, parent string, level int32, confidence float32) types.ModerationLabel {
	return types.ModerationLabel{
		Name:          aws.String(name),
		ParentName:    aws.String(parent),
		TaxonomyLevel: aws.Int32(level),
		Confidence:    aws.Float32(confidence),
	}
}

func (s *DecisionSuite) TestThresholdCategoriesAreInTheTaxonomy() {
	for category := range thresholds {
		s.True(categories[category], category)
	}
}

func (s *DecisionSuite) TestMinConfidence() {
	s.InDelta(80, minConfidence(false), 0)
	s.InDelta(0, minConfidence(true), 0)
}

func (s *DecisionSuite) TestNoLabels_Approved() {
	s.Equal(decision{}, decide(nil))
}

func (s *DecisionSuite) TestLabelAtThreshold_RejectsWithTheLabel() {
	d := decide([]types.ModerationLabel{
		label("Violence", "", 1, 80),
		label("Graphic Violence", "Violence", 2, 80),
	})

	s.True(d.Rejected)
	s.Equal("Violence", d.Label)
}

func (s *DecisionSuite) TestHighestConfidenceLabelIsTheReason() {
	d := decide([]types.ModerationLabel{
		label("Explicit", "", 1, 91),
		label("Explicit Nudity", "Explicit", 2, 97),
		label("Exposed Female Nipple", "Explicit Nudity", 3, 95),
	})

	s.Equal(decision{Rejected: true, Label: "Explicit Nudity"}, d)
}

func (s *DecisionSuite) TestBelowThreshold_Approved() {
	d := decide([]types.ModerationLabel{
		label("Hate Symbols", "", 1, 79.9),
		label("Extremist", "Hate Symbols", 2, 79.9),
	})

	s.False(d.Rejected)
}

func (s *DecisionSuite) TestCategoryWithoutThreshold_Ignored() {
	d := decide([]types.ModerationLabel{
		label("Alcohol", "", 1, 99),
		label("Alcoholic Beverages", "Alcohol", 2, 99),
	})

	s.False(d.Rejected)
	s.Empty(s.logs.String())
}

func (s *DecisionSuite) TestAncestorMissingFromResponse_LoggedAsUnknown() {
	d := decide([]types.ModerationLabel{
		label("Corpses", "Death and Emaciation", 3, 88),
	})

	s.Equal(decision{Rejected: false}, d, "Death and Emaciation isn't listed, so the walk stops there")
	s.Contains(s.logs.String(), "unknown moderation category")
}

func (s *DecisionSuite) TestL2WithItsParent_UsesTheParentsThreshold() {
	d := decide([]types.ModerationLabel{
		label("Death and Emaciation", "Visually Disturbing", 2, 88),
		label("Corpses", "Death and Emaciation", 3, 70),
	})

	s.Equal(decision{Rejected: true, Label: "Death and Emaciation"}, d)
}

func (s *DecisionSuite) TestUnknownTopLevelCategory_LoggedAndIgnored() {
	d := decide([]types.ModerationLabel{label("Brand New Category", "", 1, 99)})

	s.False(d.Rejected)
	s.Contains(s.logs.String(), "unknown moderation category")
	s.Contains(s.logs.String(), "Brand New Category")
}
