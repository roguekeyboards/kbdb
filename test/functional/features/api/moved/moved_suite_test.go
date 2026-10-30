package moved_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMoved(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pages moved to mykeebs.com Suite")
}
