package dynamo

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/repository"
)

type ImageStatusSuite struct {
	suite.Suite
}

func TestImageStatusSuite(t *testing.T) {
	suite.Run(t, new(ImageStatusSuite))
}

func (s *ImageStatusSuite) condition(from repository.ImageStatus) string {
	expr, err := expression.NewBuilder().WithCondition(statusIs(expression.Name("status"), from)).Build()
	s.Require().NoError(err)
	return *expr.Condition()
}

func (s *ImageStatusSuite) TestStatusIs_Approved_AlsoMatchesMissingStatus() {
	s.Contains(s.condition(repository.ImageStatusApproved), "attribute_not_exists")
}

func (s *ImageStatusSuite) TestStatusIs_PendingOrRejected_RequiresTheStatus() {
	s.NotContains(s.condition(repository.ImageStatusPending), "attribute_not_exists")
	s.NotContains(s.condition(repository.ImageStatusRejected), "attribute_not_exists")
}
