package models

import "github.com/geofffranks/rookies-bot/config"

type DriverLookup map[int]Driver

type Driver struct {
	FirstName     string
	LastName      string
	DiscordHandle string
	CarNumber     int
}

type Penalties struct {
	QualiBansR1            []Driver
	QualiBansR1CarriedOver []Driver
	QualiBansR2            []Driver
	QualiBansR2CarriedOver []Driver
	PitStartsR1            []Driver
	PitStartsR1CarriedOver []Driver
	PitStartsR2            []Driver
	PitStartsR2CarriedOver []Driver
}

func (p *Penalties) Consolidate() config.Penalty {
	return config.Penalty{
		QualiBansR1: uniqueDrivers(append(p.QualiBansR1, p.QualiBansR1CarriedOver...)),
		QualiBansR2: uniqueDrivers(append(p.QualiBansR2, p.QualiBansR2CarriedOver...)),
		PitStartsR1: uniqueDrivers(append(p.PitStartsR1, p.PitStartsR1CarriedOver...)),
		PitStartsR2: uniqueDrivers(append(p.PitStartsR2, p.PitStartsR2CarriedOver...)),
	}

}

func (p *Penalties) UniqueDriverNumbers() []int {
	return uniqueDrivers(append(p.QualiBansR1,
		append(p.QualiBansR1CarriedOver,
			append(p.QualiBansR2,
				append(p.QualiBansR2CarriedOver,
					append(p.PitStartsR1,
						append(p.PitStartsR1CarriedOver,
							append(p.PitStartsR2, p.PitStartsR2CarriedOver...)...,
						)...,
					)...,
				)...,
			)...,
		)...,
	))
}

func uniqueDrivers(drivers []Driver) []int {
	l := map[int]struct{}{}

	for _, driver := range drivers {
		l[driver.CarNumber] = struct{}{}
	}

	carNumbers := []int{}
	for num, _ := range l {
		carNumbers = append(carNumbers, num)
	}
	return carNumbers
}
