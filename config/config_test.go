package config_test

import (
	"os"
	"path/filepath"

	"github.com/geofffranks/rookies-bot/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Round", func() {
	Describe("String()", func() {
		It("formats as 'Round N - Track'", func() {
			r := config.Round{Number: 3, Track: "Monza"}
			Expect(r.String()).To(Equal("Round 3 - Monza"))
		})
	})
})

var _ = Describe("LoadRoundConfig", func() {
	It("parses valid YAML", func() {
		yaml := `
penalties:
  quali_bans_r1: [12, 34]
  pit_starts_r2: [56]
next_round:
  number: 4
  track: "Spa"
`
		rc, err := config.LoadRoundConfig([]byte(yaml))
		Expect(err).NotTo(HaveOccurred())
		Expect(rc.Penalties.QualiBansR1).To(Equal([]int{12, 34}))
		Expect(rc.Penalties.PitStartsR2).To(Equal([]int{56}))
		Expect(rc.NextRound.Number).To(Equal(4))
		Expect(rc.NextRound.Track).To(Equal("Spa"))
	})

	It("converts tabs to spaces before parsing", func() {
		yamlWithTabs := "penalties:\n\tquali_bans_r1: [99]\n"
		rc, err := config.LoadRoundConfig([]byte(yamlWithTabs))
		Expect(err).NotTo(HaveOccurred())
		Expect(rc.Penalties.QualiBansR1).To(Equal([]int{99}))
	})

	It("returns an error for invalid YAML", func() {
		_, err := config.LoadRoundConfig([]byte("}{garbage"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed parsing YAML data"))
	})

	It("returns empty struct for empty input", func() {
		rc, err := config.LoadRoundConfig([]byte(""))
		Expect(err).NotTo(HaveOccurred())
		Expect(rc).To(Equal(&config.RoundConfig{}))
	})
})

var _ = Describe("Load", func() {
	var (
		tmpDir          string
		botConfigPath   string
		roundConfigPath string
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "rookies-bot-config-test")
		Expect(err).NotTo(HaveOccurred())

		botConfigPath = filepath.Join(tmpDir, "bot.yml")
		err = os.WriteFile(botConfigPath, []byte(`
simgrid_api_token: "tok123"
championship_id: "champ42"
season: "2026"
discord_token: "disc-tok"
discord_channel_id: 1234567890
discord_role_name: "Rookies"
discord_briefing_channel_id: 9876543210
service_account_token_file: "/dev/null"
briefing_template_doc_id: "tmpl1"
briefing_folder_id: "folder1"
tracker_template_doc_id: "tmpl2"
tracker_folder_id: "folder2"
`), 0644)
		Expect(err).NotTo(HaveOccurred())

		roundConfigPath = filepath.Join(tmpDir, "round.yml")
		err = os.WriteFile(roundConfigPath, []byte(`
next_round:
  number: 3
  track: "Monza"
previous_round:
  number: 2
  track: "Spa"
`), 0644)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	It("loads both configs and merges them", func() {
		cfg, err := config.Load(botConfigPath, roundConfigPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.SimGridApiToken).To(Equal("tok123"))
		Expect(cfg.Season).To(Equal("2026"))
		Expect(cfg.NextRound.Track).To(Equal("Monza"))
		Expect(cfg.PreviousRound.Track).To(Equal("Spa"))
	})

	It("loads bot config alone when round config path is empty", func() {
		cfg, err := config.Load(botConfigPath, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.SimGridApiToken).To(Equal("tok123"))
		Expect(cfg.NextRound.Track).To(Equal(""))
	})

	It("returns an error when bot config file does not exist", func() {
		_, err := config.Load("/no/such/file.yml", "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed reading"))
	})

	It("returns an error when round config file does not exist", func() {
		_, err := config.Load(botConfigPath, "/no/such/round.yml")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed reading"))
	})

	It("returns an error when bot config has invalid YAML", func() {
		err := os.WriteFile(botConfigPath, []byte("}{garbage"), 0644)
		Expect(err).NotTo(HaveOccurred())
		_, err = config.Load(botConfigPath, "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed parsing"))
	})
})
