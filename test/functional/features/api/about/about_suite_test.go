package about_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAbout(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GET /about/ai Suite")
}
