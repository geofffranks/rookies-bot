package simgrid

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/models"
)

type SimGridClient struct {
	token      string
	httpClient *http.Client
	BaseURL    string
}

func NewClient(apitoken string) *SimGridClient {
	return &SimGridClient{
		token:      apitoken,
		httpClient: &http.Client{},
		BaseURL:    "https://www.thesimgrid.com/api/v1",
	}
}

type EntryListResp struct {
	Entries []Entry `json:"entries"`
}

type Entry struct {
	Drivers   []Driver `json:"drivers"`
	CarNumber int      `json:"raceNumber"`
}

type Driver struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	PlayerID  string `json:"playerID"`
}

type Race struct {
	Track Track `json:"track"`
}

type Track struct {
	Name string `json:"name"`
}

type Championship struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	HostName  string `json:"host_name"`
	StartDate string `json:"start_date"`
	Races     []Race `json:"races"`
}

// StartYear parses the championship's RFC3339 start_date and returns its year.
func (c *Championship) StartYear() (int, error) {
	t, err := time.Parse(time.RFC3339, c.StartDate)
	if err != nil {
		return 0, fmt.Errorf("could not parse championship start date %q: %w", c.StartDate, err)
	}
	return t.Year(), nil
}

// ChampionshipListItem is a single entry in the championships index response,
// which carries only id and name.
type ChampionshipListItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type User struct {
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	SteamID       string `json:"steam64_id"`
	DiscordHandle string `json:"username"`
	CarNumber     int
}

func (sgc *SimGridClient) GetEntriesForChampionship(id string) ([]Entry, error) {
	resp, err := sgc.makeRequest("GET", fmt.Sprintf("/championships/%s/entrylist?format=json", id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var elr EntryListResp
	err = json.Unmarshal(data, &elr)
	if err != nil {
		return nil, err
	}

	return elr.Entries, nil
}

func (sgc *SimGridClient) UsersForChampionship(id string) ([]User, error) {
	resp, err := sgc.makeRequest("GET", fmt.Sprintf("/championships/%s/participating_users", id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	users := []User{}
	err = json.Unmarshal(data, &users)
	if err != nil {
		return nil, err
	}
	return users, nil
}

// ListUpcomingChampionships returns all upcoming multi-race championships
// (id + name only).
func (sgc *SimGridClient) ListUpcomingChampionships() ([]ChampionshipListItem, error) {
	resp, err := sgc.makeRequest("GET", "/championships?status=upcoming&races_count=full_championships")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var items []ChampionshipListItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// GetChampionship returns the full detail for a single championship.
func (sgc *SimGridClient) GetChampionship(id string) (*Championship, error) {
	resp, err := sgc.makeRequest("GET", fmt.Sprintf("/championships/%s", id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var champ Championship
	if err := json.Unmarshal(data, &champ); err != nil {
		return nil, err
	}
	return &champ, nil
}

// FindSeasonChampionship finds the single upcoming championship hosted by host
// whose name contains both "rookies" and the given season term. Candidates are
// pre-filtered by name from the cheap index call, then confirmed against the
// detail endpoint's host_name. Returns an error when zero or more than one
// championship matches.
func (sgc *SimGridClient) FindSeasonChampionship(host, term string) (*Championship, error) {
	items, err := sgc.ListUpcomingChampionships()
	if err != nil {
		return nil, fmt.Errorf("failed listing upcoming championships: %w", err)
	}

	lowerTerm := strings.ToLower(term)
	var matches []*Championship
	for _, item := range items {
		lowerName := strings.ToLower(item.Name)
		if !strings.Contains(lowerName, "rookies") || !strings.Contains(lowerName, lowerTerm) {
			continue
		}
		champ, err := sgc.GetChampionship(strconv.Itoa(item.ID))
		if err != nil {
			return nil, fmt.Errorf("failed fetching championship %d: %w", item.ID, err)
		}
		if strings.EqualFold(champ.HostName, host) {
			matches = append(matches, champ)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no upcoming %s Rookies championship matching season %q found", host, term)
	case 1:
		return matches[0], nil
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, fmt.Sprintf("%q (#%d)", m.Name, m.ID))
		}
		return nil, fmt.Errorf("multiple upcoming %s Rookies championships match season %q: %s", host, term, strings.Join(names, ", "))
	}
}

func (sgc *SimGridClient) GetNextRound(id string, prev config.Round) (*config.Round, error) {
	championship, err := sgc.GetChampionship(id)
	if err != nil {
		return nil, err
	}

	nextRoundNum := prev.Number + 1
	nextTrack := ""
	if len(championship.Races) >= nextRoundNum {
		nextTrack = championship.Races[nextRoundNum-1].Track.Name
	}

	return &config.Round{Number: nextRoundNum, Track: nextTrack}, nil
}

func (sgc *SimGridClient) BuildDriverLookup(id string) (models.DriverLookup, error) {
	userLookup := map[string]*User{}
	users, err := sgc.UsersForChampionship(id)
	if err != nil {
		return nil, err
	}
	for _, user := range users {
		userLookup[fmt.Sprintf("S%s", user.SteamID)] = &user
	}

	entries, err := sgc.GetEntriesForChampionship(id)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		for _, driver := range entry.Drivers {
			if driver.FirstName == "" && driver.LastName == "" {
				continue
			}
			if _, ok := userLookup[driver.PlayerID]; ok {
				userLookup[driver.PlayerID].CarNumber = entry.CarNumber
			} else {
				return nil, fmt.Errorf("unknown driver: %#v", driver)
			}
		}
	}

	parsedUsers := models.DriverLookup{}
	for _, user := range userLookup {
		parsedUsers[user.CarNumber] = models.Driver{
			FirstName:     user.FirstName,
			LastName:      user.LastName,
			DiscordHandle: user.DiscordHandle,
			CarNumber:     user.CarNumber,
		}
	}
	return parsedUsers, nil
}

func (sgc *SimGridClient) makeRequest(method, url string) (*http.Response, error) {
	toReq := fmt.Sprintf("%s%s", sgc.BaseURL, url)
	req, err := http.NewRequest(method, toReq, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sgc.token))

	resp, err := sgc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return resp, fmt.Errorf("HTTP request failure: %s", resp.Status)
	}
	return resp, nil
}
