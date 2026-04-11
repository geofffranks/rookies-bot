package discord_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDiscord(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Discord Suite")
}
