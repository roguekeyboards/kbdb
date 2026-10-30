package moderation

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"os"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rektypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/roguekeyboards/kbdb/internal/moderation/mocks"
	"github.com/roguekeyboards/kbdb/internal/repository"
	repomocks "github.com/roguekeyboards/kbdb/internal/repository/mocks"
)

const (
	testBucket = "images"
	testKey    = "switches/alice/sw1/image/up1"
)

type ModeratorSuite struct {
	suite.Suite
	s3          *mocks.MockS3API
	rekognition *mocks.MockRekognitionAPI
	switches    *repomocks.MockSwitchRepository
	moderator   *Moderator
}

func TestModeratorSuite(t *testing.T) {
	suite.Run(t, new(ModeratorSuite))
}

func (s *ModeratorSuite) SetupTest() {
	s.s3 = mocks.NewMockS3API(s.T())
	s.rekognition = mocks.NewMockRekognitionAPI(s.T())
	s.switches = repomocks.NewMockSwitchRepository(s.T())
	s.moderator = &Moderator{
		s3:          s.s3,
		rekognition: s.rekognition,
		images:      &Images{Switches: s.switches},
	}
}

func (s *ModeratorSuite) handle(key string) error {
	return s.moderator.Handle(s.T().Context(), events.S3Event{Records: []events.S3EventRecord{{
		S3: events.S3Entity{
			Bucket: events.S3Bucket{Name: testBucket},
			Object: events.S3Object{Key: key, URLDecodedKey: key},
		},
	}}})
}

func (s *ModeratorSuite) stored(path string, status repository.ImageStatus) {
	p := repository.SwitchImageKey(path)
	s.switches.EXPECT().Get(mock.Anything, "alice", "sw1").
		Return(&repository.Switch{ID: "sw1", ImagePath: &p, ImageStatus: status}, nil)
}

func (s *ModeratorSuite) object(data []byte) {
	s.s3.EXPECT().GetObject(mock.Anything, mock.Anything).
		Return(&s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil)
}

func (s *ModeratorSuite) webp(path string) {
	data, err := os.ReadFile(path) //nolint:gosec // test fixture.
	s.Require().NoError(err)
	s.object(data)
}

func (s *ModeratorSuite) labels(labels ...rektypes.ModerationLabel) {
	s.rekognition.EXPECT().
		DetectModerationLabels(mock.Anything, mock.MatchedBy(func(in *rekognition.DetectModerationLabelsInput) bool {
			return in.Image.S3Object == nil &&
				bytes.HasPrefix(in.Image.Bytes, []byte{0xFF, 0xD8}) &&
				aws.ToFloat32(in.MinConfidence) == 80
		})).
		Return(&rekognition.DetectModerationLabelsOutput{ModerationLabels: labels}, nil)
}

func (s *ModeratorSuite) expectStatus(to repository.ImageStatus, reason *string) *mock.Call {
	return s.switches.EXPECT().
		SetImageStatus(mock.Anything, "alice", "sw1", repository.SwitchImageKey(testKey), repository.ImageStatusPending, to, reason).
		Return(true, nil).Call
}

func (s *ModeratorSuite) expectTagged() *mock.Call {
	return s.s3.EXPECT().
		PutObjectTagging(mock.Anything, mock.MatchedBy(func(in *s3.PutObjectTaggingInput) bool {
			tags := in.Tagging.TagSet
			return aws.ToString(in.Key) == testKey && len(tags) == 1 &&
				aws.ToString(tags[0].Key) == "moderation" && aws.ToString(tags[0].Value) == "rejected"
		})).
		Return(&s3.PutObjectTaggingOutput{}, nil).Call
}

func (s *ModeratorSuite) TestNoLabels_Approved() {
	s.stored(testKey, repository.ImageStatusPending)
	s.webp("testdata/half-transparent.webp")
	s.labels()
	s.expectStatus(repository.ImageStatusApproved, nil)

	s.Require().NoError(s.handle(testKey))
}

func (s *ModeratorSuite) TestLabelOverThreshold_TaggedThenRejectedWithTheLabel() {
	s.stored(testKey, repository.ImageStatusPending)
	s.webp("testdata/half-transparent.webp")
	s.labels(label("Hate Symbols", "", 1, 95), label("Nazi Party", "Hate Symbols", 2, 95))
	tagged := s.expectTagged()
	s.expectStatus(repository.ImageStatusRejected, aws.String("Hate Symbols")).NotBefore(tagged)

	s.Require().NoError(s.handle(testKey))
}

func (s *ModeratorSuite) TestCalibration_RequestsEveryLabel() {
	s.moderator.calibration = true
	s.stored(testKey, repository.ImageStatusPending)
	s.webp("testdata/half-transparent.webp")
	s.rekognition.EXPECT().
		DetectModerationLabels(mock.Anything, mock.MatchedBy(func(in *rekognition.DetectModerationLabelsInput) bool {
			return aws.ToFloat32(in.MinConfidence) == 0
		})).
		Return(&rekognition.DetectModerationLabelsOutput{ModerationLabels: []rektypes.ModerationLabel{
			label("Violence", "", 1, 30),
		}}, nil)
	s.expectStatus(repository.ImageStatusApproved, nil)

	s.Require().NoError(s.handle(testKey))
}

func (s *ModeratorSuite) TestReplacedImage_SkipsRekognition() {
	s.stored("switches/alice/sw1/image/up2", repository.ImageStatusPending)

	s.Require().NoError(s.handle(testKey))
}

func (s *ModeratorSuite) TestAlreadyDecided_SkipsRekognition() {
	s.stored(testKey, repository.ImageStatusApproved)

	s.Require().NoError(s.handle(testKey))
}

func (s *ModeratorSuite) TestItemDeleted_SkipsRekognition() {
	s.switches.EXPECT().Get(mock.Anything, "alice", "sw1").Return(nil, repository.ErrNotFound)

	s.Require().NoError(s.handle(testKey))
}

func (s *ModeratorSuite) TestNotAnImageKey_Ignored() {
	s.Require().NoError(s.handle("something/else"))
}

func (s *ModeratorSuite) TestRekognitionRefusesImage_RejectedUnprocessable() {
	s.stored(testKey, repository.ImageStatusPending)
	s.webp("testdata/half-transparent.webp")
	s.rekognition.EXPECT().DetectModerationLabels(mock.Anything, mock.Anything).
		Return(nil, &rektypes.InvalidImageFormatException{Message: aws.String("bad")})
	tagged := s.expectTagged()
	s.expectStatus(repository.ImageStatusRejected, aws.String("unprocessable")).NotBefore(tagged)

	s.Require().NoError(s.handle(testKey))
}

func (s *ModeratorSuite) TestUndecodableWebP_RejectedUnprocessable() {
	s.stored(testKey, repository.ImageStatusPending)
	s.object([]byte("RIFF\x10\x00\x00\x00WEBPVP8 not really"))
	s.expectTagged()
	s.expectStatus(repository.ImageStatusRejected, aws.String("unprocessable"))

	s.Require().NoError(s.handle(testKey))
}

func (s *ModeratorSuite) TestNotWebP_RejectedNotWebP() {
	var buf bytes.Buffer
	s.Require().NoError(jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 10, 10)), nil))
	s.stored(testKey, repository.ImageStatusPending)
	s.object(buf.Bytes())
	tagged := s.expectTagged()
	s.expectStatus(repository.ImageStatusRejected, aws.String("not_webp")).NotBefore(tagged)

	s.Require().NoError(s.handle(testKey))
}

func (s *ModeratorSuite) TestOverMaxEdge_RejectedTooLarge() {
	s.stored(testKey, repository.ImageStatusPending)
	s.webp("testdata/oversized.webp")
	tagged := s.expectTagged()
	s.expectStatus(repository.ImageStatusRejected, aws.String("too_large")).NotBefore(tagged)

	s.Require().NoError(s.handle(testKey))
}

func (s *ModeratorSuite) TestRekognitionTemporaryError_ReturnedForRetry() {
	s.stored(testKey, repository.ImageStatusPending)
	s.webp("testdata/half-transparent.webp")
	s.rekognition.EXPECT().DetectModerationLabels(mock.Anything, mock.Anything).
		Return(nil, &rektypes.ThrottlingException{Message: aws.String("slow down")})

	s.Require().Error(s.handle(testKey))
}

func (s *ModeratorSuite) TestTaggingError_ReturnedBeforeTheStatusWrite() {
	s.stored(testKey, repository.ImageStatusPending)
	s.webp("testdata/half-transparent.webp")
	s.labels(label("Explicit", "", 1, 99))
	s.s3.EXPECT().PutObjectTagging(mock.Anything, mock.Anything).Return(nil, errors.New("s3 down"))

	s.Require().Error(s.handle(testKey))
}

func (s *ModeratorSuite) TestStatusChangedMeanwhile_NotAnError() {
	s.stored(testKey, repository.ImageStatusPending)
	s.webp("testdata/half-transparent.webp")
	s.labels()
	s.switches.EXPECT().
		SetImageStatus(mock.Anything, "alice", "sw1", repository.SwitchImageKey(testKey), repository.ImageStatusPending, repository.ImageStatusApproved, (*string)(nil)).
		Return(false, nil)

	s.Require().NoError(s.handle(testKey))
}

func (s *ModeratorSuite) TestStatusWriteError_Returned() {
	s.stored(testKey, repository.ImageStatusPending)
	s.webp("testdata/half-transparent.webp")
	s.labels()
	s.switches.EXPECT().
		SetImageStatus(mock.Anything, "alice", "sw1", repository.SwitchImageKey(testKey), repository.ImageStatusPending, repository.ImageStatusApproved, (*string)(nil)).
		Return(false, errors.New("dynamodb down"))

	s.Require().Error(s.handle(testKey))
}
