package simgrid_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/simgrid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newTestClient(server *httptest.Server) *simgrid.SimGridClient {
	c := simgrid.NewClient("test-token")
	c.BaseURL = server.URL
	return c
}

var _ = Describe("SimGridClient", func() {
	var (
		server *httptest.Server
		client *simgrid.SimGridClient
		mux    *http.ServeMux
	)

	BeforeEach(func() {
		mux = http.NewServeMux()
		server = httptest.NewServer(mux)
		client = newTestClient(server)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("GetEntriesForChampionship", func() {
		It("returns parsed entries on success", func() {
			mux.HandleFunc("/championships/champ1/entrylist", func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer test-token"))
				Expect(r.URL.Query().Get("format")).To(Equal("json"))
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(simgrid.EntryListResp{
					Entries: []simgrid.Entry{
						{
							CarNumber: 42,
							Drivers: []simgrid.Driver{
								{FirstName: "Max", LastName: "V", PlayerID: "S111"},
							},
						},
					},
				})
			})

			entries, err := client.GetEntriesForChampionship("champ1")
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].CarNumber).To(Equal(42))
			Expect(entries[0].Drivers[0].FirstName).To(Equal("Max"))
		})

		It("returns an error when server responds with 4xx", func() {
			mux.HandleFunc("/championships/champ1/entrylist", func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "not found", http.StatusNotFound)
			})
			_, err := client.GetEntriesForChampionship("champ1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("HTTP request failure"))
		})

		It("returns an error on malformed JSON", func() {
			mux.HandleFunc("/championships/champ1/entrylist", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("}{not json"))
			})
			_, err := client.GetEntriesForChampionship("champ1")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UsersForChampionship", func() {
		It("returns parsed users on success", func() {
			mux.HandleFunc("/championships/champ1/participating_users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]simgrid.User{
					{FirstName: "Lewis", LastName: "H", SteamID: "9999", DiscordHandle: "lewis"},
				})
			})

			users, err := client.UsersForChampionship("champ1")
			Expect(err).NotTo(HaveOccurred())
			Expect(users).To(HaveLen(1))
			Expect(users[0].FirstName).To(Equal("Lewis"))
			Expect(users[0].DiscordHandle).To(Equal("lewis"))
		})

		It("returns an error when server responds with 5xx", func() {
			mux.HandleFunc("/championships/champ1/participating_users", func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "server error", http.StatusInternalServerError)
			})
			_, err := client.UsersForChampionship("champ1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("HTTP request failure"))
		})
	})

	Describe("GetNextRound", func() {
		It("returns the track for the next race when it exists", func() {
			mux.HandleFunc("/championships/champ1", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(simgrid.Championship{
					Races: []simgrid.Race{
						{Track: simgrid.Track{Name: "Spa"}},
						{Track: simgrid.Track{Name: "Monza"}},
						{Track: simgrid.Track{Name: "Silverstone"}},
					},
				})
			})
			prev := config.Round{Number: 2}
			next, err := client.GetNextRound("champ1", prev)
			Expect(err).NotTo(HaveOccurred())
			Expect(next.Number).To(Equal(3))
			Expect(next.Track).To(Equal("Silverstone"))
		})

		It("returns an empty track when the race list is exhausted", func() {
			mux.HandleFunc("/championships/champ1", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(simgrid.Championship{
					Races: []simgrid.Race{
						{Track: simgrid.Track{Name: "Spa"}},
					},
				})
			})
			prev := config.Round{Number: 1}
			next, err := client.GetNextRound("champ1", prev)
			Expect(err).NotTo(HaveOccurred())
			Expect(next.Number).To(Equal(2))
			Expect(next.Track).To(Equal(""))
		})

		It("returns an error on HTTP failure", func() {
			mux.HandleFunc("/championships/champ1", func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "forbidden", http.StatusForbidden)
			})
			_, err := client.GetNextRound("champ1", config.Round{Number: 1})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("BuildDriverLookup", func() {
		BeforeEach(func() {
			mux.HandleFunc("/championships/champ1/participating_users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]simgrid.User{
					{FirstName: "Max", LastName: "V", SteamID: "111", DiscordHandle: "maxv"},
					{FirstName: "Lewis", LastName: "H", SteamID: "222", DiscordHandle: "lewish"},
				})
			})
		})

		It("builds a lookup of car number to driver", func() {
			mux.HandleFunc("/championships/champ1/entrylist", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(simgrid.EntryListResp{
					Entries: []simgrid.Entry{
						{CarNumber: 33, Drivers: []simgrid.Driver{{FirstName: "Max", LastName: "V", PlayerID: "S111"}}},
						{CarNumber: 44, Drivers: []simgrid.Driver{{FirstName: "Lewis", LastName: "H", PlayerID: "S222"}}},
					},
				})
			})

			lookup, err := client.BuildDriverLookup("champ1")
			Expect(err).NotTo(HaveOccurred())
			Expect(lookup).To(HaveLen(2))
			Expect(lookup[33].DiscordHandle).To(Equal("maxv"))
			Expect(lookup[44].DiscordHandle).To(Equal("lewish"))
		})

		It("skips entry drivers with blank first and last name", func() {
			mux.HandleFunc("/championships/champ1/entrylist", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(simgrid.EntryListResp{
					Entries: []simgrid.Entry{
						{CarNumber: 33, Drivers: []simgrid.Driver{
							{FirstName: "", LastName: "", PlayerID: "S111"},
						}},
					},
				})
			})

			lookup, err := client.BuildDriverLookup("champ1")
			Expect(err).NotTo(HaveOccurred())
			_ = lookup
		})

		It("returns an error when an entry driver is not in the user list", func() {
			mux.HandleFunc("/championships/champ1/entrylist", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(simgrid.EntryListResp{
					Entries: []simgrid.Entry{
						{CarNumber: 99, Drivers: []simgrid.Driver{{FirstName: "Ghost", LastName: "Driver", PlayerID: "S999"}}},
					},
				})
			})
			_, err := client.BuildDriverLookup("champ1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unknown driver"))
		})

		It("returns an error when the user API fails", func() {
			mux2 := http.NewServeMux()
			mux2.HandleFunc("/championships/champ1/participating_users", func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			})
			failServer := httptest.NewServer(mux2)
			defer failServer.Close()
			failClient := simgrid.NewClient("token")
			failClient.BaseURL = failServer.URL
			_, err := failClient.BuildDriverLookup("champ1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("HTTP request failure"))
		})
	})
})

var _ simgrid.EntryListResp
var _ simgrid.Entry
var _ simgrid.Driver
var _ simgrid.User
var _ simgrid.Race
var _ simgrid.Track
var _ simgrid.Championship

var _ = fmt.Sprintf
