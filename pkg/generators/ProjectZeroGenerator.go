package generators

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/feeds"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type ProjectZeroGenerator struct {
	filterFunc      ItemFilterFunc
	itemModFunc     ItemModifierFunc
	workdir         string
	xsrfToken       string
	xsrfLastUpdated time.Time
}

type MonorailIssue struct {
	StatusRef struct {
		Status string `json:"status"`
	} `json:"statusRef"`
	OpenedTimestamp        int    `json:"openedTimestamp"`
	LocalId                int    `json:"localId"`
	ProjectName            string `json:"projectName"`
	OwnerModifiedTimestamp int    `json:"ownerModifiedTimestamp"`
	AttachmentCount        int    `json:"attachmentCount"`
	StarCount              int    `json:"starCount"`
	ModifiedTimestamp      int    `json:"modifiedTimestamp"`
	Summary                string `json:"summary"`
	OwnerRef               struct {
		DisplayName string `json:"displayName"`
		UserId      string `json:"userId""`
	} `json:"ownerRef"`
	LabelRefs []struct {
		Label string `json:"label"`
	} `json:"labelRefs"`
	CCRefs []struct {
		DisplayName string `json:"displayName"`
		IsDerived   bool   `json:"isDerived"`
		UserId      string `json:"userId"`
	} `json:"ccRefs"`
	StatusModifiedTimestamp    int `json:"statusModifiedTimestamp"`
	ComponentModifiedTimestamp int `json:"componentModifiedTimestamp"`
	ReporterRef                struct {
		DisplayName string `json:"displayName"`
		UserId      string `json:"userId"`
	} `json:"reportedRef"`
}

type monorailQueryResponse struct {
	TotalResults int             `json:"totalResults"`
	Issues       []MonorailIssue `json:"issues"`
}

type monorailQuery struct {
	ProjectNames []string `json:"projectNames"`
	Query        string   `json:"query"`
	CannedQuery  int      `json:"cannedQuery"`
	SortSpec     string   `json:"sortSpec"`
	Pagination   struct {
		Start    int `json:"start,omitempty"`
		MaxItems int `json:"maxItems"`
	} `json:"pagination"`
}

func (g *ProjectZeroGenerator) WorkDir(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(path, 0644); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	g.workdir = path
	return nil
}

func (g *ProjectZeroGenerator) getXsrfToken() (string, error) {
	// I believe the XSRF Token expires after 2 hours but no harm is refreshing the page sooner
	if g.xsrfToken != "" && g.xsrfLastUpdated.Add(1*time.Hour).After(time.Now()) {
		return g.xsrfToken, nil
	}
	client := http.Client{
		Timeout: 15 * time.Second,
	}

	res, err := client.Get("https://bugs.chromium.org/p/project-zero/issues/list?q=&can=1&sort=-id")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	if !strings.Contains(string(body), "'token': '") {
		return "", errors.New("unable to retrieve XSRF token for project zero tracker")
	}
	g.xsrfToken = strings.Split(strings.Split(string(body), "'token': '")[1], "'")[0]
	g.xsrfLastUpdated = time.Now()
	return g.xsrfToken, nil
}

func (g *ProjectZeroGenerator) queryIssues(start, count int) (monorailQueryResponse, error) {
	url := "https://bugs.chromium.org/prpc/monorail.Issues/ListIssues"
	data := monorailQuery{
		ProjectNames: []string{"project-zero"},
		Query:        "",
		CannedQuery:  1,
		SortSpec:     "id",
		//-id for descending but because we end up processing in order sorting in ascending gets
		// us a descending order after adding to the state
	}
	data.Pagination.Start = start
	data.Pagination.MaxItems = count

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return monorailQueryResponse{}, err
	}

	xsrfToken, err := g.getXsrfToken()
	if err != nil {
		return monorailQueryResponse{}, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return monorailQueryResponse{}, err
	}
	req.Header.Add("content-type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:83.0) Gecko/20100101 Firefox/83.0")
	req.Header.Add("X-Xsrf-Token", xsrfToken)
	req.Header.Add("Origin", "https://bugs.chromium.org")

	client := http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return monorailQueryResponse{}, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return monorailQueryResponse{}, err
	}
	lines := strings.SplitN(string(body), "\n", 2)
	if len(lines) != 2 {
		return monorailQueryResponse{}, errors.New("monorail response was not well formatted (missing junk before json)")
	}
	var out monorailQueryResponse
	err = json.Unmarshal([]byte(lines[1]), &out)
	if err != nil {
		return monorailQueryResponse{}, err
	}
	return out, nil
}

func (g *ProjectZeroGenerator) allIssues() ([]MonorailIssue, error) {
	var out []MonorailIssue
	start := 0
	count := 1000

	for {
		newIssues, err := g.queryIssues(start, count)
		if err != nil {
			return nil, err
		}

		out = append(out, newIssues.Issues...)

		start += count
		if start > newIssues.TotalResults {
			break
		}
	}
	return out, nil
}

type projectZeroState struct {
	DisclosedTimestamps map[int]time.Time `json:"disclosed"`
	LatestIds           []int             `json:"latest"`
}

func (g *ProjectZeroGenerator) saveState(state *projectZeroState) error {
	fp, err := os.OpenFile(g.workdir+"/projectzero.state.json", os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer fp.Close()

	jsBytes, err := json.Marshal(state)
	if err != nil {
		return err
	}
	fp.Write(jsBytes)

	return nil
}

func (g *ProjectZeroGenerator) loadState() (projectZeroState, error) {
	content, err := ioutil.ReadFile(g.workdir + "/projectzero.state.json")
	if err != nil {
		if os.IsNotExist(err) {
			return projectZeroState{
				make(map[int]time.Time),
				[]int{},
			}, nil
		}
		return projectZeroState{}, err
	}

	var out projectZeroState
	err = json.Unmarshal(content, &out)
	return out, err
}

func (g *ProjectZeroGenerator) Feed() (*feeds.Feed, error) {
	state, err := g.loadState()
	if err != nil {
		return nil, err
	}

	allIssues, err := g.allIssues()
	if err != nil {
		return nil, err
	}

	hasUpdate := false
	for _, issue := range allIssues {
		if _, ok := state.DisclosedTimestamps[issue.LocalId]; !ok {
			state.DisclosedTimestamps[issue.LocalId] = time.Now()
			if len(state.LatestIds) >= 19 {
				state.LatestIds = append([]int{issue.LocalId}, state.LatestIds[:19]...)
			} else {
				state.LatestIds = append([]int{issue.LocalId}, state.LatestIds...)
			}
			log.Printf("[%d] %s\n", issue.LocalId, issue.Summary)
			hasUpdate = true
		}
	}
	if hasUpdate {
		g.saveState(&state)
	}

	out := feeds.Feed{}
	out.Title = "Project Zero Bug Tracker"
	out.Link = &feeds.Link{Href: "https://bugs.chromium.org/p/project-zero/issues/list?q=&can=1&sort=-id"}
	out.Updated = time.Now()

	for _, id := range state.LatestIds {
		// Yeah, this is a poor choice to iterate, but whatever its easier
		for _, issue := range allIssues {
			if issue.LocalId == id {
				newItem := &feeds.Item{
					Title: issue.Summary,
					Link: &feeds.Link{
						Href: fmt.Sprintf("https://bugs.chromium.org/p/project-zero/issues/detail?id=%d", issue.LocalId),
					},
					Author: &feeds.Author{
						Name: issue.OwnerRef.DisplayName,
					},
					Description: issue.Summary,
					Id:          fmt.Sprintf("%d", issue.LocalId),
					Created:     time.Unix(int64(issue.OpenedTimestamp), 0),
					Updated:     state.DisclosedTimestamps[issue.LocalId],
					Content:     issue.Summary,
				}

				if g.itemModFunc != nil {
					g.itemModFunc(newItem)
				}

				// BUG: Filtering here will just reduce the feed size ideally we should get a feed
				// with the expected number of elements every time
				if g.filterFunc == nil || g.filterFunc(newItem) {
					out.Items = append(out.Items, newItem)
				}
				break
			}
		}
	}
	return &out, nil

}

func (g *ProjectZeroGenerator) RegisterItemFilter(callback ItemFilterFunc) {
	g.filterFunc = callback
}
func (g *ProjectZeroGenerator) RegisterItemModifier(callback ItemModifierFunc) {
	g.itemModFunc = callback
}
