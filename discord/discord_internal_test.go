package discord

import (
	"errors"
	"fmt"

	"github.com/disgoorg/snowflake/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// snowflakeID converts a uint64 to snowflake.ID for test readability.
func snowflakeID(n uint64) snowflake.ID {
	return snowflake.ID(n)
}

var _ = Describe("DiscordHandleNotFoundError", func() {
	It("Error() returns a string containing the handle", func() {
		err := DiscordHandleNotFoundError{Handle: "testuser"}
		Expect(err.Error()).To(ContainSubstring("testuser"))
	})

	It("String() returns a string containing the handle", func() {
		err := DiscordHandleNotFoundError{Handle: "testuser"}
		Expect(err.String()).To(ContainSubstring("testuser"))
	})

	It("Error() and String() return the same value", func() {
		err := DiscordHandleNotFoundError{Handle: "testuser"}
		Expect(err.Error()).To(Equal(err.String()))
	})

	It("Is() returns true when target is the same type", func() {
		err := DiscordHandleNotFoundError{Handle: "a"}
		Expect(err.Is(DiscordHandleNotFoundError{Handle: "different"})).To(BeTrue())
	})

	It("Is() returns false when target is a different error type", func() {
		err := DiscordHandleNotFoundError{Handle: "a"}
		Expect(err.Is(fmt.Errorf("other error"))).To(BeFalse())
	})

	It("errors.Is works with DiscordHandleNotFoundError sentinel", func() {
		err := DiscordHandleNotFoundError{Handle: "missinguser"}
		Expect(errors.Is(err, DiscordHandleNotFoundError{})).To(BeTrue())
	})

	It("errors.Is returns false for unrelated errors", func() {
		err := fmt.Errorf("some other error")
		Expect(errors.Is(err, DiscordHandleNotFoundError{})).To(BeFalse())
	})
})

var _ = Describe("isAllowedUser", func() {
	DescribeTable("returns true for known admin IDs",
		func(id uint64) {
			Expect(isAllowedUser(snowflakeID(id))).To(BeTrue())
		},
		Entry("porkchop", uint64(208972532068515840)),
		Entry("ralli", uint64(371787234187280385)),
		Entry("kallil", uint64(418087017448996864)),
		Entry("geoff", uint64(942149076873543721)),
	)

	It("returns false for a non-admin user ID", func() {
		Expect(isAllowedUser(snowflakeID(999999999))).To(BeFalse())
	})
})
