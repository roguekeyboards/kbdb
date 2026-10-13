package moderation

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/rogueserenity/kbdb/internal/imagekey"
	"github.com/rogueserenity/kbdb/internal/moderation/mocks"
	"github.com/rogueserenity/kbdb/internal/repository"
	repomocks "github.com/rogueserenity/kbdb/internal/repository/mocks"
)

type OverridesSuite struct {
	suite.Suite
	s3        *mocks.MockS3API
	switches  *repomocks.MockSwitchRepository
	overrides *Overrides
}

func TestOverridesSuite(t *testing.T) {
	suite.Run(t, new(OverridesSuite))
}

func (s *OverridesSuite) SetupTest() {
	s.s3 = mocks.NewMockS3API(s.T())
	s.switches = repomocks.NewMockSwitchRepository(s.T())
	s.overrides = &Overrides{s3: s.s3, images: &Images{Switches: s.switches}, bucket: testBucket}
}

func (s *OverridesSuite) setStatus(from, to repository.ImageStatus, reason *string, ok bool) {
	s.switches.EXPECT().
		SetImageStatus(mock.Anything, "alice", "sw1", repository.SwitchImageKey(testKey), from, to, reason).
		Return(ok, nil)
}

func (s *OverridesSuite) stored(path string, status repository.ImageStatus) {
	p := repository.SwitchImageKey(path)
	s.switches.EXPECT().Get(mock.Anything, "alice", "sw1").
		Return(&repository.Switch{ID: "sw1", ImagePath: &p, ImageStatus: status}, nil)
}

func (s *OverridesSuite) untagged() {
	s.s3.EXPECT().DeleteObjectTagging(mock.Anything, mock.MatchedBy(func(in *s3.DeleteObjectTaggingInput) bool {
		return aws.ToString(in.Bucket) == testBucket && aws.ToString(in.Key) == testKey
	})).Return(&s3.DeleteObjectTaggingOutput{}, nil)
}

func (s *OverridesSuite) tagged() {
	s.s3.EXPECT().PutObjectTagging(mock.Anything, mock.MatchedBy(func(in *s3.PutObjectTaggingInput) bool {
		tags := in.Tagging.TagSet
		return aws.ToString(in.Key) == testKey && len(tags) == 1 &&
			aws.ToString(tags[0].Key) == "moderation" && aws.ToString(tags[0].Value) == "rejected"
	})).Return(&s3.PutObjectTaggingOutput{}, nil)
}

func (s *OverridesSuite) TestApprove_Rejected() {
	s.setStatus(repository.ImageStatusRejected, repository.ImageStatusApproved, nil, true)
	s.untagged()

	already, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().NoError(err)
	s.False(already)
}

func (s *OverridesSuite) TestApprove_AlreadyApproved() {
	s.setStatus(repository.ImageStatusRejected, repository.ImageStatusApproved, nil, false)
	s.stored(testKey, repository.ImageStatusApproved)
	s.untagged()

	already, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().NoError(err)
	s.True(already)
}

func (s *OverridesSuite) TestApprove_Pending() {
	s.setStatus(repository.ImageStatusRejected, repository.ImageStatusApproved, nil, false)
	s.stored(testKey, repository.ImageStatusPending)

	_, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().ErrorContains(err, "is pending, not rejected")
}

func (s *OverridesSuite) TestApprove_Replaced() {
	s.setStatus(repository.ImageStatusRejected, repository.ImageStatusApproved, nil, false)
	s.stored("switches/alice/sw1/image/up2", repository.ImageStatusRejected)

	_, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().ErrorIs(err, ErrNotCurrent)
}

func (s *OverridesSuite) TestApprove_UntagFails() {
	s.setStatus(repository.ImageStatusRejected, repository.ImageStatusApproved, nil, true)
	s.s3.EXPECT().DeleteObjectTagging(mock.Anything, mock.Anything).Return(nil, errors.New("boom"))

	_, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().ErrorContains(err, "boom")
}

func (s *OverridesSuite) TestApprove_UnknownKey() {
	_, err := s.overrides.Approve(s.T().Context(), "other/thing")
	s.Require().ErrorIs(err, imagekey.ErrUnknownKey)
}

func (s *OverridesSuite) TestReject_Approved() {
	reason := "manual"
	s.setStatus(repository.ImageStatusApproved, repository.ImageStatusRejected, &reason, true)
	s.tagged()

	already, err := s.overrides.Reject(s.T().Context(), testKey, reason)
	s.Require().NoError(err)
	s.False(already)
}

func (s *OverridesSuite) TestReject_AlreadyRejected() {
	reason := "manual"
	s.setStatus(repository.ImageStatusApproved, repository.ImageStatusRejected, &reason, false)
	s.stored(testKey, repository.ImageStatusRejected)
	s.tagged()

	already, err := s.overrides.Reject(s.T().Context(), testKey, reason)
	s.Require().NoError(err)
	s.True(already)
}

func (s *OverridesSuite) TestReject_Pending() {
	reason := "manual"
	s.setStatus(repository.ImageStatusApproved, repository.ImageStatusRejected, &reason, false)
	s.stored(testKey, repository.ImageStatusPending)

	_, err := s.overrides.Reject(s.T().Context(), testKey, reason)
	s.Require().ErrorContains(err, "is pending, not approved")
}

func (s *OverridesSuite) TestReject_WriteFails() {
	reason := "manual"
	s.switches.EXPECT().SetImageStatus(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(false, errors.New("boom"))

	_, err := s.overrides.Reject(s.T().Context(), testKey, reason)
	s.Require().ErrorContains(err, "boom")
}
