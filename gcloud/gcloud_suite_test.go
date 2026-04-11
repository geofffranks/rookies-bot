package gcloud_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestGcloud(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GCloud Suite")
}
