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
	QualiBans              []Driver
	QualiBansCarriedOver   []Driver
	PitStartsR1            []Driver
	PitStartsR1CarriedOver []Driver
	PitStartsR2            []Driver
	PitStartsR2CarriedOver []Driver
}

func (p *Penalties) Consolidate() config.Penalty {
	return config.Penalty{
		QualiBans:   uniqueDrivers(append(p.QualiBans, p.QualiBansCarriedOver...)),
		PitStartsR1: uniqueDrivers(append(p.PitStartsR1, p.PitStartsR1CarriedOver...)),
		PitStartsR2: uniqueDrivers(append(p.PitStartsR2, p.PitStartsR2CarriedOver...)),
	}

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
