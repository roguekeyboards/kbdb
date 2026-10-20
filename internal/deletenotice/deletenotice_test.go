package deletenotice

import (
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/deletenotice/mocks"
)

type SNSSuite struct {
	suite.Suite

	mockClient *mocks.MockSNSAPI
	notices    *SNS
}

func TestSNSSuite(t *testing.T) {
	suite.Run(t, new(SNSSuite))
}

func (s *SNSSuite) SetupTest() {
	s.mockClient = mocks.NewMockSNSAPI(s.T())
	s.notices = &SNS{client: s.mockClient, topicARN: "arn:aws:sns:us-east-2:1:topic"}
}

func (s *SNSSuite) TestPublishesUserIDToTopic() {
	s.mockClient.EXPECT().
		Publish(mock.Anything, mock.MatchedBy(func(in *sns.PublishInput) bool {
			return *in.TopicArn == "arn:aws:sns:us-east-2:1:topic" && strings.Contains(*in.Message, "user-live-abc")
		})).
		Return(&sns.PublishOutput{}, nil)

	s.Require().NoError(s.notices.Notify(s.T().Context(), "user-live-abc"))
}

func (s *SNSSuite) TestPublishFails_Error() {
	s.mockClient.EXPECT().Publish(mock.Anything, mock.Anything).Return(nil, errors.New("sns down"))

	s.Require().Error(s.notices.Notify(s.T().Context(), "user-live-abc"))
}
