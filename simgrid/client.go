package simgrid

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"rookies-bot/config"
	"rookies-bot/models"
)

type SimGridClient struct {
	token      string
	httpClient *http.Client
}

func NewClient(apitoken string) *SimGridClient {
	return &SimGridClient{
		token:      apitoken,
		httpClient: &http.Client{},
	}
}

var baseUrl = "https://www.thesimgrid.com/api/v1"

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
}

type Race struct {
	Track string `json:"track"`
}

type Championship struct {
	Races []Race `json:"races"`
}

type User struct {
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
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

func (sgc *SimGridClient) GetNextRound(id string, prev config.Round) (*config.Round, error) {
	resp, err := sgc.makeRequest("GET", fmt.Sprintf("/championships/%s", id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	championship := Championship{}
	err = json.Unmarshal(data, &championship)
	if err != nil {
		return nil, err
	}
	nextRoundNum := prev.Number + 1
	nextTrack := ""
	if len(championship.Races) >= nextRoundNum {
		nextTrack = championship.Races[nextRoundNum-1].Track
	}

	nextRound := config.Round{
		Number: nextRoundNum,
		Track:  nextTrack,
	}
	return &nextRound, nil

}

func (sgc *SimGridClient) BuildDriverLookup(id string) (models.DriverLookup, error) {
	userLookup := map[string]*User{}
	users, err := sgc.UsersForChampionship(id)
	if err != nil {
		return nil, err
	}
	for _, user := range users {
		name := fmt.Sprintf("%s%s", user.FirstName, user.LastName)
		userLookup[name] = &user
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
			name := fmt.Sprintf("%s%s", driver.FirstName, driver.LastName)
			if _, ok := userLookup[name]; ok {
				userLookup[name].CarNumber = entry.CarNumber
			} else {
				return nil, fmt.Errorf("Unknown driver: %s", name)
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
	toReq := fmt.Sprintf("%s%s", baseUrl, url)
	req, err := http.NewRequest(method, toReq, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sgc.token))

	resp, err := sgc.httpClient.Do(req)
	if resp.StatusCode >= 400 {
		return resp, fmt.Errorf("HTTP request failure: %s", resp.Status)
	}
	return resp, nil
}
