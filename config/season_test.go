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

var _ = Describe("ParseSeasonSelection", func() {
	DescribeTable("normalizes supported inputs to the canonical term",
		func(input, expected string) {
			term, err := config.ParseSeasonSelection(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(term).To(Equal(expected))
		},
		Entry("bare lowercase term", "fall", "Fall"),
		Entry("bare mixed-case term", "Fall", "Fall"),
		Entry("bare uppercase term", "WINTER", "Winter"),
		Entry("bare two-word term", "New Year", "New Year"),
		Entry("lowercase two-word term", "new year", "New Year"),
		Entry("bare Spring", "Spring", "Spring"),
		Entry("bare Summer", "Summer", "Summer"),
		Entry("leading year", "2026 Fall", "Fall"),
		Entry("surrounding whitespace", "  Fall  ", "Fall"),
		Entry("role name", "GT4 Rookies Fall", "Fall"),
		Entry("role name lowercase", "gt4 rookies fall", "Fall"),
		Entry("role name mixed case", "Gt4 Rookies Winter", "Winter"),
		Entry("irregular New role alias", "GT4 Rookie New", "New Year"),
		Entry("irregular Springs role alias", "GT4 Rookies Springs", "Spring"),
		Entry("role-prefixed canonical term", "GT4 Rookies Spring", "Spring"),
		Entry("role-prefixed singular rookie", "gt4 rookie fall", "Fall"),
		Entry("repeated internal whitespace in role name", "GT4   Rookies   Fall", "Fall"),
		Entry("repeated internal whitespace in two-word term", "New    Year", "New Year"),
	)

	It("returns no selection for empty input", func() {
		term, err := config.ParseSeasonSelection("")
		Expect(err).NotTo(HaveOccurred())
		Expect(term).To(BeEmpty())
	})

	It("returns no selection for whitespace-only input", func() {
		term, err := config.ParseSeasonSelection("   ")
		Expect(err).NotTo(HaveOccurred())
		Expect(term).To(BeEmpty())
	})

	It("rejects an unsupported season with an actionable message", func() {
		_, err := config.ParseSeasonSelection("Autumn")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Autumn"))
		Expect(err.Error()).To(ContainSubstring("New Year"))
		Expect(err.Error()).To(ContainSubstring("GT4 Rookies Fall"))
	})

	DescribeTable("rejects year-only nonempty input",
		func(input string) {
			_, err := config.ParseSeasonSelection(input)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid season"))
		},
		Entry("trailing space", "2026 "),
		Entry("surrounding whitespace", " 2026\t"),
	)

	It("rejects a term glued to the role prefix without a space", func() {
		_, err := config.ParseSeasonSelection("GT4 RookiesFall")
		Expect(err).To(HaveOccurred())
	})
})
