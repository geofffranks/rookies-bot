package models_test

import (
	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/models"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Penalties", func() {
	var (
		driver1 = models.Driver{FirstName: "Alice", LastName: "Smith", CarNumber: 11, DiscordHandle: "alice"}
		driver2 = models.Driver{FirstName: "Bob", LastName: "Jones", CarNumber: 22, DiscordHandle: "bob"}
		driver3 = models.Driver{FirstName: "Carol", LastName: "Lee", CarNumber: 33, DiscordHandle: "carol"}
	)

	Describe("Consolidate()", func() {
		It("merges current and carried-over penalties into one Penalty struct", func() {
			p := models.Penalties{
				QualiBansR1:            []models.Driver{driver1},
				QualiBansR1CarriedOver: []models.Driver{driver2},
				QualiBansR2:            []models.Driver{driver3},
				QualiBansR2CarriedOver: []models.Driver{driver1},
				PitStartsR1:            []models.Driver{driver2},
				PitStartsR1CarriedOver: []models.Driver{driver3},
				PitStartsR2:            []models.Driver{driver1},
				PitStartsR2CarriedOver: []models.Driver{driver2},
			}

			result := p.Consolidate()

			Expect(result).To(BeAssignableToTypeOf(config.Penalty{}))
			// QualiBansR1: driver1(11) + driver2(22)
			Expect(result.QualiBansR1).To(ConsistOf(11, 22))
			// QualiBansR2: driver3(33) + driver1(11)
			Expect(result.QualiBansR2).To(ConsistOf(33, 11))
			// PitStartsR1: driver2(22) + driver3(33)
			Expect(result.PitStartsR1).To(ConsistOf(22, 33))
			// PitStartsR2: driver1(11) + driver2(22)
			Expect(result.PitStartsR2).To(ConsistOf(11, 22))
		})

		It("deduplicates drivers appearing in both current and carried-over lists", func() {
			p := models.Penalties{
				QualiBansR1:            []models.Driver{driver1},
				QualiBansR1CarriedOver: []models.Driver{driver1}, // same driver
			}
			result := p.Consolidate()
			Expect(result.QualiBansR1).To(HaveLen(1))
			Expect(result.QualiBansR1).To(ConsistOf(11))
		})

		It("returns empty slices when all fields are empty", func() {
			p := models.Penalties{}
			result := p.Consolidate()
			Expect(result.QualiBansR1).To(BeEmpty())
			Expect(result.QualiBansR2).To(BeEmpty())
			Expect(result.PitStartsR1).To(BeEmpty())
			Expect(result.PitStartsR2).To(BeEmpty())
		})
	})

	Describe("UniqueDriverNumbers()", func() {
		It("returns unique car numbers across all penalty lists", func() {
			p := models.Penalties{
				QualiBansR1:            []models.Driver{driver1},
				QualiBansR1CarriedOver: []models.Driver{driver2},
				QualiBansR2:            []models.Driver{driver3},
				QualiBansR2CarriedOver: []models.Driver{driver1}, // driver1 duplicate
				PitStartsR1:            []models.Driver{driver2}, // driver2 duplicate
				PitStartsR1CarriedOver: []models.Driver{},
				PitStartsR2:            []models.Driver{},
				PitStartsR2CarriedOver: []models.Driver{},
			}
			result := p.UniqueDriverNumbers()
			Expect(result).To(ConsistOf(11, 22, 33))
		})

		It("returns empty slice when no drivers have penalties", func() {
			p := models.Penalties{}
			result := p.UniqueDriverNumbers()
			Expect(result).To(BeEmpty())
		})

		It("handles a driver with penalties across all categories", func() {
			p := models.Penalties{
				QualiBansR1:            []models.Driver{driver1},
				QualiBansR1CarriedOver: []models.Driver{driver1},
				QualiBansR2:            []models.Driver{driver1},
				QualiBansR2CarriedOver: []models.Driver{driver1},
				PitStartsR1:            []models.Driver{driver1},
				PitStartsR1CarriedOver: []models.Driver{driver1},
				PitStartsR2:            []models.Driver{driver1},
				PitStartsR2CarriedOver: []models.Driver{driver1},
			}
			result := p.UniqueDriverNumbers()
			Expect(result).To(HaveLen(1))
			Expect(result).To(ConsistOf(11))
		})
	})
})
