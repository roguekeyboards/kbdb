package legal_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLegal(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GET /privacy and GET /terms Suite")
}
