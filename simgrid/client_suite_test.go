package simgrid_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestSimgrid(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Simgrid Suite")
}
