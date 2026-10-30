package cloudfront

import (
	"errors"
	"testing"

	awscloudfront "github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/kbdb/internal/repository/cloudfront/mocks"
)

type InvalidatorSuite struct {
	suite.Suite

	client      *mocks.MockCloudFrontAPI
	invalidator *Invalidator
}

func TestInvalidatorSuite(t *testing.T) {
	suite.Run(t, new(InvalidatorSuite))
}

func (s *InvalidatorSuite) SetupTest() {
	s.client = mocks.NewMockCloudFrontAPI(s.T())
	s.invalidator = &Invalidator{client: s.client, distributionID: "EDIST"}
}

func (s *InvalidatorSuite) TestInvalidate_SendsPathsToDistribution() {
	var refs []string
	s.client.EXPECT().
		CreateInvalidation(mock.Anything, mock.MatchedBy(func(in *awscloudfront.CreateInvalidationInput) bool {
			refs = append(refs, *in.InvalidationBatch.CallerReference)
			return *in.DistributionId == "EDIST" &&
				*in.InvalidationBatch.Paths.Quantity == 2 &&
				in.InvalidationBatch.Paths.Items[0] == "/a" && in.InvalidationBatch.Paths.Items[1] == "/b/*"
		})).
		Return(&awscloudfront.CreateInvalidationOutput{}, nil).Twice()

	s.Require().NoError(s.invalidator.Invalidate(s.T().Context(), "/a", "/b/*"))
	s.Require().NoError(s.invalidator.Invalidate(s.T().Context(), "/a", "/b/*"))
	s.NotEqual(refs[0], refs[1])
}

func (s *InvalidatorSuite) TestInvalidate_NoDistribution_DoesNothing() {
	s.invalidator.distributionID = ""

	s.Require().NoError(s.invalidator.Invalidate(s.T().Context(), "/a"))
}

func (s *InvalidatorSuite) TestInvalidate_NoPaths_DoesNothing() {
	s.Require().NoError(s.invalidator.Invalidate(s.T().Context()))
}

func (s *InvalidatorSuite) TestInvalidate_Error_Propagates() {
	s.client.EXPECT().CreateInvalidation(mock.Anything, mock.Anything).Return(nil, errors.New("throttled"))

	s.Require().ErrorContains(s.invalidator.Invalidate(s.T().Context(), "/a"), "throttled")
}
