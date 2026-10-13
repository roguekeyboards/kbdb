package moderation

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
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
	s.overrides = &Overrides{s3: s.s3, images: &Images{Switches: s.switches}, bucket: testBucket, now: time.Now}
}

func (s *OverridesSuite) setStatus(from, to repository.ImageStatus, reason *string, ok bool) {
	s.switches.EXPECT().
		SetImageStatus(mock.Anything, "alice", "sw1", repository.SwitchImageKey(testKey), from, to, reason).
		Return(ok, nil)
}

func (s *OverridesSuite) stored(path string, status repository.ImageStatus) {
	p := repository.SwitchImageKey(path)
	s.switches.EXPECT().Get(mock.Anything, "alice", "sw1").
		Return(&repository.Switch{ID: "sw1", ImagePath: &p, ImageStatus: status}, nil).Once()
}

func (s *OverridesSuite) exists() {
	s.s3.EXPECT().HeadObject(mock.Anything, mock.MatchedBy(func(in *s3.HeadObjectInput) bool {
		return aws.ToString(in.Bucket) == testBucket && aws.ToString(in.Key) == testKey
	})).Return(&s3.HeadObjectOutput{}, nil)
}

func (s *OverridesSuite) tags(tags ...s3types.Tag) {
	s.s3.EXPECT().GetObjectTagging(mock.Anything, mock.MatchedBy(func(in *s3.GetObjectTaggingInput) bool {
		return aws.ToString(in.Bucket) == testBucket && aws.ToString(in.Key) == testKey
	})).Return(&s3.GetObjectTaggingOutput{TagSet: tags}, nil)
}

func tag(k, v string) s3types.Tag {
	return s3types.Tag{Key: aws.String(k), Value: aws.String(v)}
}

func (s *OverridesSuite) tagged() {
	s.s3.EXPECT().PutObjectTagging(mock.Anything, mock.MatchedBy(func(in *s3.PutObjectTaggingInput) bool {
		tags := in.Tagging.TagSet
		return aws.ToString(in.Bucket) == testBucket && aws.ToString(in.Key) == testKey && len(tags) == 1 &&
			aws.ToString(tags[0].Key) == "moderation" && aws.ToString(tags[0].Value) == "rejected"
	})).Return(&s3.PutObjectTaggingOutput{}, nil)
}

func (s *OverridesSuite) untagged() {
	s.tags(tag("moderation", "rejected"))
	s.s3.EXPECT().DeleteObjectTagging(mock.Anything, mock.MatchedBy(func(in *s3.DeleteObjectTaggingInput) bool {
		return aws.ToString(in.Bucket) == testBucket && aws.ToString(in.Key) == testKey
	})).Return(&s3.DeleteObjectTaggingOutput{}, nil)
}

func (s *OverridesSuite) TestApprove_Rejected() {
	s.exists()
	s.stored(testKey, repository.ImageStatusRejected)
	s.untagged()
	s.setStatus(repository.ImageStatusRejected, repository.ImageStatusApproved, nil, true)

	already, untagged, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().NoError(err)
	s.False(already)
	s.True(untagged)
}

func (s *OverridesSuite) TestApprove_AlreadyApproved() {
	s.exists()
	s.stored(testKey, repository.ImageStatusApproved)
	s.tags()

	already, untagged, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().NoError(err)
	s.True(already)
	s.False(untagged)
}

// A rerun after the status write failed: approved, but still tagged.
func (s *OverridesSuite) TestApprove_AlreadyApprovedStillTagged() {
	s.exists()
	s.stored(testKey, repository.ImageStatusApproved)
	s.untagged()

	already, untagged, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().NoError(err)
	s.True(already)
	s.True(untagged)
}

// The tag is left alone on an image that isn't rejected or approved.
func (s *OverridesSuite) TestApprove_Pending() {
	s.exists()
	s.stored(testKey, repository.ImageStatusPending)

	_, _, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().ErrorContains(err, "is pending, not rejected")
}

// The tag is left alone on a file no item holds, so it still expires.
func (s *OverridesSuite) TestApprove_Replaced() {
	s.exists()
	s.stored("switches/alice/sw1/image/up2", repository.ImageStatusRejected)

	_, _, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().ErrorIs(err, ErrNotCurrent)
}

// The image changed between the read and the conditional write.
func (s *OverridesSuite) TestApprove_ChangedDuringUpdate() {
	s.exists()
	s.stored(testKey, repository.ImageStatusRejected)
	s.untagged()
	s.setStatus(repository.ImageStatusRejected, repository.ImageStatusApproved, nil, false)
	s.stored("switches/alice/sw1/image/up2", repository.ImageStatusPending)

	_, _, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().ErrorIs(err, ErrNotCurrent)
}

// The status write never happens, so a failed untag leaves the image
// rejected rather than approved and still expiring.
func (s *OverridesSuite) TestApprove_UntagFails() {
	s.exists()
	s.stored(testKey, repository.ImageStatusRejected)
	s.tags(tag("moderation", "rejected"))
	s.s3.EXPECT().DeleteObjectTagging(mock.Anything, mock.Anything).Return(nil, errors.New("boom"))

	_, _, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().ErrorContains(err, "boom")
}

func (s *OverridesSuite) TestApprove_KeepsOtherTags() {
	s.exists()
	s.stored(testKey, repository.ImageStatusRejected)
	s.tags(tag("team", "kbdb"), tag("moderation", "rejected"))
	s.s3.EXPECT().PutObjectTagging(mock.Anything, mock.MatchedBy(func(in *s3.PutObjectTaggingInput) bool {
		tags := in.Tagging.TagSet
		return aws.ToString(in.Key) == testKey && len(tags) == 1 && aws.ToString(tags[0].Key) == "team"
	})).Return(&s3.PutObjectTaggingOutput{}, nil)
	s.setStatus(repository.ImageStatusRejected, repository.ImageStatusApproved, nil, true)

	_, untagged, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().NoError(err)
	s.True(untagged)
}

func (s *OverridesSuite) TestApprove_FileDeleted() {
	s.s3.EXPECT().HeadObject(mock.Anything, mock.Anything).Return(nil, &s3types.NotFound{})

	_, _, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().ErrorIs(err, ErrImageGone)
}

func (s *OverridesSuite) TestApprove_FilePastExpiry() {
	s.overrides.now = func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) }
	s.s3.EXPECT().HeadObject(mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{
		Expiration: aws.String(`expiry-date="Tue, 13 Oct 2026 00:00:00 GMT", rule-id="expire-rejected-images"`),
	}, nil)

	_, _, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().ErrorIs(err, ErrImageGone)
}

func (s *OverridesSuite) TestApprove_FileExpiresLater() {
	s.overrides.now = func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) }
	s.s3.EXPECT().HeadObject(mock.Anything, mock.Anything).Return(&s3.HeadObjectOutput{
		Expiration: aws.String(`expiry-date="Wed, 14 Oct 2026 00:00:00 GMT", rule-id="expire-rejected-images"`),
	}, nil)
	s.stored(testKey, repository.ImageStatusRejected)
	s.untagged()
	s.setStatus(repository.ImageStatusRejected, repository.ImageStatusApproved, nil, true)

	_, _, err := s.overrides.Approve(s.T().Context(), testKey)
	s.Require().NoError(err)
}

func (s *OverridesSuite) TestApprove_UnknownKey() {
	_, _, err := s.overrides.Approve(s.T().Context(), "other/thing")
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

func (s *OverridesSuite) TestReject_TagFails() {
	reason := "manual"
	s.setStatus(repository.ImageStatusApproved, repository.ImageStatusRejected, &reason, true)
	s.s3.EXPECT().PutObjectTagging(mock.Anything, mock.Anything).Return(nil, errors.New("boom"))

	_, err := s.overrides.Reject(s.T().Context(), testKey, reason)
	s.Require().ErrorContains(err, "boom")
}

func (s *OverridesSuite) TestReject_FileGone() {
	reason := "manual"
	s.setStatus(repository.ImageStatusApproved, repository.ImageStatusRejected, &reason, true)
	s.s3.EXPECT().PutObjectTagging(mock.Anything, mock.Anything).Return(nil, &s3types.NoSuchKey{})

	_, err := s.overrides.Reject(s.T().Context(), testKey, reason)
	s.Require().NoError(err)
}

func (s *OverridesSuite) TestReject_WriteFails() {
	reason := "manual"
	s.switches.EXPECT().SetImageStatus(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(false, errors.New("boom"))

	_, err := s.overrides.Reject(s.T().Context(), testKey, reason)
	s.Require().ErrorContains(err, "boom")
}
