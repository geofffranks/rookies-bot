package config_test

import (
	"github.com/geofffranks/rookies-bot/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseSeasonTerm", func() {
	DescribeTable("extracts the term from a season string",
		func(season, expected string) {
			term, err := config.ParseSeasonTerm(season)
			Expect(err).NotTo(HaveOccurred())
			Expect(term).To(Equal(expected))
		},
		Entry("term only", "Fall", "Fall"),
		Entry("year and term", "2026 Summer", "Summer"),
		Entry("two-word term with year", "2027 New Year", "New Year"),
		Entry("two-word term only", "New Year", "New Year"),
		Entry("lowercase", "2026 spring", "Spring"),
		Entry("extra whitespace", "  2026   Winter  ", "Winter"),
	)

	It("returns an error for an unrecognized term", func() {
		_, err := config.ParseSeasonTerm("2026 Autumn")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Autumn"))
	})
})

var _ = Describe("NextTerm", func() {
	DescribeTable("advances through the season cycle",
		func(current, expected string) {
			next, err := config.NextTerm(current)
			Expect(err).NotTo(HaveOccurred())
			Expect(next).To(Equal(expected))
		},
		Entry("New Year -> Spring", "New Year", "Spring"),
		Entry("Spring -> Summer", "Spring", "Summer"),
		Entry("Summer -> Fall", "Summer", "Fall"),
		Entry("Fall -> Winter", "Fall", "Winter"),
		Entry("Winter wraps to New Year", "Winter", "New Year"),
	)

	It("returns an error for an unknown term", func() {
		_, err := config.NextTerm("Autumn")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("RoleNameForTerm", func() {
	DescribeTable("maps each term to its (inconsistent) role name",
		func(term, expected string) {
			role, err := config.RoleNameForTerm(term)
			Expect(err).NotTo(HaveOccurred())
			Expect(role).To(Equal(expected))
		},
		Entry("Summer", "Summer", "GT4 Rookies Summer"),
		Entry("Fall", "Fall", "GT4 Rookies Fall"),
		Entry("Winter", "Winter", "GT4 Rookies Winter"),
		Entry("New Year", "New Year", "GT4 Rookie New"),
		Entry("Spring", "Spring", "GT4 Rookies Springs"),
	)

	It("returns an error for an unknown term", func() {
		_, err := config.RoleNameForTerm("Autumn")
		Expect(err).To(HaveOccurred())
	})
})
