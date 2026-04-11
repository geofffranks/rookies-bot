package discord

import (
	"errors"
	"fmt"

	"github.com/disgoorg/snowflake/v2"
	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/models"
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

var _ = Describe("buildPenalizedDriverList", func() {
	var driverLookup models.DriverLookup

	BeforeEach(func() {
		driverLookup = models.DriverLookup{
			42: {FirstName: "Max", LastName: "V", CarNumber: 42, DiscordHandle: "maxv"},
			77: {FirstName: "Valt", LastName: "B", CarNumber: 77, DiscordHandle: "valtb"},
		}
	})

	It("returns nil slice for empty car number list", func() {
		drivers, err := buildPenalizedDriverList(driverLookup, []int{})
		Expect(err).NotTo(HaveOccurred())
		Expect(drivers).To(BeNil())
	})

	It("returns the correct drivers for known car numbers", func() {
		drivers, err := buildPenalizedDriverList(driverLookup, []int{42, 77})
		Expect(err).NotTo(HaveOccurred())
		Expect(drivers).To(HaveLen(2))
		Expect(drivers[0].CarNumber).To(Equal(42))
		Expect(drivers[1].CarNumber).To(Equal(77))
	})

	It("returns a single driver when one car number is given", func() {
		drivers, err := buildPenalizedDriverList(driverLookup, []int{42})
		Expect(err).NotTo(HaveOccurred())
		Expect(drivers).To(HaveLen(1))
		Expect(drivers[0].DiscordHandle).To(Equal("maxv"))
	})

	It("returns an error when a car number is not in the lookup", func() {
		_, err := buildPenalizedDriverList(driverLookup, []int{99})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("99"))
	})

	It("returns an error mentioning the unknown number even when some are valid", func() {
		_, err := buildPenalizedDriverList(driverLookup, []int{42, 99})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("99"))
	})
})

var _ = Describe("buildPenaltyList", func() {
	var (
		driverLookup models.DriverLookup
		roundConfig  *config.RoundConfig
	)

	BeforeEach(func() {
		driverLookup = models.DriverLookup{
			1: {CarNumber: 1, DiscordHandle: "d1"},
			2: {CarNumber: 2, DiscordHandle: "d2"},
			3: {CarNumber: 3, DiscordHandle: "d3"},
			4: {CarNumber: 4, DiscordHandle: "d4"},
			5: {CarNumber: 5, DiscordHandle: "d5"},
			6: {CarNumber: 6, DiscordHandle: "d6"},
			7: {CarNumber: 7, DiscordHandle: "d7"},
			8: {CarNumber: 8, DiscordHandle: "d8"},
		}
		roundConfig = &config.RoundConfig{
			Penalties: config.Penalty{
				QualiBansR1: []int{1},
				QualiBansR2: []int{2},
				PitStartsR1: []int{3},
				PitStartsR2: []int{4},
			},
			CarriedOverPenalties: config.Penalty{
				QualiBansR1: []int{5},
				QualiBansR2: []int{6},
				PitStartsR1: []int{7},
				PitStartsR2: []int{8},
			},
		}
	})

	It("populates all 8 penalty fields correctly", func() {
		penalties, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).NotTo(HaveOccurred())

		Expect(penalties.QualiBansR1).To(HaveLen(1))
		Expect(penalties.QualiBansR1[0].CarNumber).To(Equal(1))

		Expect(penalties.QualiBansR2).To(HaveLen(1))
		Expect(penalties.QualiBansR2[0].CarNumber).To(Equal(2))

		Expect(penalties.PitStartsR1).To(HaveLen(1))
		Expect(penalties.PitStartsR1[0].CarNumber).To(Equal(3))

		Expect(penalties.PitStartsR2).To(HaveLen(1))
		Expect(penalties.PitStartsR2[0].CarNumber).To(Equal(4))

		Expect(penalties.QualiBansR1CarriedOver).To(HaveLen(1))
		Expect(penalties.QualiBansR1CarriedOver[0].CarNumber).To(Equal(5))

		Expect(penalties.QualiBansR2CarriedOver).To(HaveLen(1))
		Expect(penalties.QualiBansR2CarriedOver[0].CarNumber).To(Equal(6))

		Expect(penalties.PitStartsR1CarriedOver).To(HaveLen(1))
		Expect(penalties.PitStartsR1CarriedOver[0].CarNumber).To(Equal(7))

		Expect(penalties.PitStartsR2CarriedOver).To(HaveLen(1))
		Expect(penalties.PitStartsR2CarriedOver[0].CarNumber).To(Equal(8))
	})

	It("returns empty penalty slices when config has no car numbers", func() {
		roundConfig = &config.RoundConfig{}
		penalties, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(penalties.QualiBansR1).To(BeNil())
		Expect(penalties.QualiBansR2).To(BeNil())
		Expect(penalties.PitStartsR1).To(BeNil())
		Expect(penalties.PitStartsR2).To(BeNil())
	})

	It("returns error when QualiBansR1 has unknown car number", func() {
		roundConfig.Penalties.QualiBansR1 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("999"))
	})

	It("returns error when QualiBansR2 has unknown car number", func() {
		roundConfig.Penalties.QualiBansR2 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when PitStartsR1 has unknown car number", func() {
		roundConfig.Penalties.PitStartsR1 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when PitStartsR2 has unknown car number", func() {
		roundConfig.Penalties.PitStartsR2 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when QualiBansR1CarriedOver has unknown car number", func() {
		roundConfig.CarriedOverPenalties.QualiBansR1 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when QualiBansR2CarriedOver has unknown car number", func() {
		roundConfig.CarriedOverPenalties.QualiBansR2 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when PitStartsR1CarriedOver has unknown car number", func() {
		roundConfig.CarriedOverPenalties.PitStartsR1 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when PitStartsR2CarriedOver has unknown car number", func() {
		roundConfig.CarriedOverPenalties.PitStartsR2 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})
})
