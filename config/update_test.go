package config_test

import (
	"os"
	"path/filepath"

	"github.com/geofffranks/rookies-bot/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

var _ = Describe("UpdateBotConfigFile", func() {
	var path string

	BeforeEach(func() {
		dir, err := os.MkdirTemp("", "rookies-bot-update-test")
		Expect(err).NotTo(HaveOccurred())
		path = filepath.Join(dir, "config.yml")
		err = os.WriteFile(path, []byte(`# rookies-bot config
discord_token: super-secret-token
season: Fall
championship_id: "9485"
discord_role_name: GT4 Rookie
briefing_folder_id: old-briefing
tracker_folder_id: old-tracker
`), 0600)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(filepath.Dir(path))
	})

	It("updates the requested keys", func() {
		err := config.UpdateBotConfigFile(path, map[string]string{
			"season":             "2026 Summer",
			"championship_id":    "24877",
			"discord_role_name":  "GT4 Rookies Summer",
			"briefing_folder_id": "new-briefing",
			"tracker_folder_id":  "new-tracker",
		})
		Expect(err).NotTo(HaveOccurred())

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		var parsed map[string]string
		Expect(yaml.Unmarshal(data, &parsed)).To(Succeed())

		Expect(parsed["season"]).To(Equal("2026 Summer"))
		Expect(parsed["championship_id"]).To(Equal("24877"))
		Expect(parsed["discord_role_name"]).To(Equal("GT4 Rookies Summer"))
		Expect(parsed["briefing_folder_id"]).To(Equal("new-briefing"))
		Expect(parsed["tracker_folder_id"]).To(Equal("new-tracker"))
	})

	It("preserves comments and untouched keys (including secrets)", func() {
		err := config.UpdateBotConfigFile(path, map[string]string{"season": "2026 Summer"})
		Expect(err).NotTo(HaveOccurred())

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("# rookies-bot config"))
		Expect(string(data)).To(ContainSubstring("discord_token: super-secret-token"))
	})

	It("appends a key that is not already present", func() {
		err := config.UpdateBotConfigFile(path, map[string]string{"new_key": "new-value"})
		Expect(err).NotTo(HaveOccurred())

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		var parsed map[string]string
		Expect(yaml.Unmarshal(data, &parsed)).To(Succeed())
		Expect(parsed["new_key"]).To(Equal("new-value"))
	})

	It("returns an error when the file does not exist", func() {
		err := config.UpdateBotConfigFile("/no/such/file.yml", map[string]string{"season": "x"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed reading"))
	})

	It("returns an error when the file is not a YAML mapping", func() {
		Expect(os.WriteFile(path, []byte("- just\n- a\n- list\n"), 0600)).To(Succeed())
		err := config.UpdateBotConfigFile(path, map[string]string{"season": "x"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not a YAML mapping"))
	})
})
