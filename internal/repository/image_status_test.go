package repository

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ImageStatusSuite struct {
	suite.Suite
}

func TestImageStatusSuite(t *testing.T) {
	suite.Run(t, new(ImageStatusSuite))
}

func (s *ImageStatusSuite) TestEffective_MissingIsApproved() {
	s.Equal(ImageStatusApproved, ImageStatus("").Effective())
	s.Equal(ImageStatusPending, ImageStatusPending.Effective())
	s.Equal(ImageStatusRejected, ImageStatusRejected.Effective())
}

func (s *ImageStatusSuite) TestSortedKeyboardImages_MissingStatusReadsAsApproved() {
	images := SortedKeyboardImages(map[string]KeyboardImageEntry{
		"img1": {Path: "keyboards/alice/kb1/images/img1"},
		"img2": {Path: "keyboards/alice/kb1/images/img2", Seq: 1, Status: ImageStatusPending},
	})

	s.Require().Len(images, 2)
	s.Equal(ImageStatusApproved, images[0].Status)
	s.Equal(ImageStatusPending, images[1].Status)
}

func (s *ImageStatusSuite) TestSortedBuildImages_CarriesStatusAndReason() {
	reason := "unprocessable"
	images := SortedBuildImages(map[string]BuildImageEntry{
		"img1": {Path: "builds/alice/b1/images/img1", Status: ImageStatusRejected, RejectionReason: &reason},
		"img2": {Path: "builds/alice/b1/images/img2", Seq: 1},
	})

	s.Require().Len(images, 2)
	s.Equal(ImageStatusRejected, images[0].Status)
	s.Equal(&reason, images[0].RejectionReason)
	s.Equal(ImageStatusApproved, images[1].Status)
}
