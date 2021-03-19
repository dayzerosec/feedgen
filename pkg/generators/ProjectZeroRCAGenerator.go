package generators

import (
	"encoding/json"
	"fmt"
	"github.com/andybalholm/cascadia"
	"github.com/gorilla/feeds"
	"golang.org/x/net/html"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type ProjectZeroRCAGenerator struct {
	filterFunc  ItemFilterFunc
	itemModFunc ItemModifierFunc
	workdir     string
}

type projectZeroRCAState struct {
	KnownRCA map[string]RCAInfo `json:"known"`
}

type RCAInfo struct {
	Cve       string
	Title     string
	Author    string
	Link      string
	Disclosed *time.Time
}

func (g *ProjectZeroRCAGenerator) WorkDir(path string) error {
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

func (g *ProjectZeroRCAGenerator) saveState(state *projectZeroRCAState) error {
	fp, err := os.OpenFile(g.workdir+"/projectzerorca.state.json", os.O_WRONLY|os.O_CREATE, 0644)
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

func (g *ProjectZeroRCAGenerator) loadState() (projectZeroRCAState, error) {
	content, err := ioutil.ReadFile(g.workdir + "/projectzerorca.state.json")
	if err != nil {
		if os.IsNotExist(err) {
			return projectZeroRCAState{
				make(map[string]RCAInfo),
			}, nil
		}
		return projectZeroRCAState{}, err
	}

	var out projectZeroRCAState
	err = json.Unmarshal(content, &out)
	return out, err
}

func httpGet(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	client := http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (g *ProjectZeroRCAGenerator) AddItemSelector(dest *map[string]*cascadia.Selector, name, value string) error {
	if sel, err := cascadia.Compile(value); err != nil {
		return err
	} else {
		(*dest)[name] = &sel
	}
	return nil
}

func (g *ProjectZeroRCAGenerator) RegisterItemFilter(callback ItemFilterFunc) {
	g.filterFunc = callback
}
func (g *ProjectZeroRCAGenerator) RegisterItemModifier(callback ItemModifierFunc) {
	g.itemModFunc = callback
}

func (g *ProjectZeroRCAGenerator) Feed() (*feeds.Feed, error) {
	url := "https://googleprojectzero.github.io/0days-in-the-wild/rca.html"
	sel := make(map[string]*cascadia.Selector)
	cssg := CssGenerator{}

	if err := g.AddItemSelector(&sel, "content", ".post-content table:first-of-type tr:not(:first-of-type):not(:last-of-type)"); err != nil {
		return nil, err
	}
	if err := g.AddItemSelector(&sel, "title", "td:first-of-type"); err != nil {
		return nil, err
	}
	if err := g.AddItemSelector(&sel, "link", "a:last-of-type"); err != nil {
		return nil, err
	}
	if err := g.AddItemSelector(&sel, "author", ".post-content p:first-of-type"); err != nil {
		return nil, err
	}
	if err := g.AddItemSelector(&sel, "date", ".post-content p:nth-of-type(2)"); err != nil {
		return nil, err
	}

	state, err := g.loadState()
	if err != nil {
		return nil, err
	}

	res, err := httpGet(url)
	if err != nil {
		return nil, err
	}

	node, err := html.Parse(res.Body)
	if err != nil {
		return nil, err
	}

	var rcas []RCAInfo
	for _, itemNode := range sel["content"].MatchAll(node) {
		var title string
		var rca RCAInfo
		if titleNode := sel["title"].MatchFirst(itemNode); titleNode != nil {
			title, _ = cssg.GetText(titleNode)
		}
		if linkNode := sel["link"].MatchFirst(itemNode); linkNode != nil {
			rca.Link, _ = cssg.GetLink(linkNode)
		}
		splits := strings.SplitN(title, ":", 2)
		if len(splits) != 2 {
			fmt.Println("Got an unexpected title: %s", title)
			continue
		}
		rca.Cve = strings.TrimSpace(splits[0])
		rca.Title = strings.TrimSpace(splits[1])
		rcas = append(rcas, rca)
	}

	now := time.Now()
	hasUpdate := false
	for _, rca := range rcas {
		if _, found := state.KnownRCA[rca.Cve]; !found {
			hasUpdate = true
			fmt.Printf("New RCA: %s [%s]\n", rca.Title, rca.Cve)
			res, err = httpGet(rca.Link)
			if err != nil {
				return nil, err
			}
			node, err := html.Parse(res.Body)
			if err != nil {
				return nil, err
			}

			if a := sel["author"].MatchFirst(node); a != nil {
				rca.Author, _ = cssg.GetText(a)
			}
			/* I was going to parse the date out of the report however it appears that date is not related to
			   the release of the RCA itself, so i'll just trust the script to find the RCA when its fresh */
			rca.Disclosed = &now
			state.KnownRCA[rca.Cve] = rca
		}
	}

	if hasUpdate {
		g.saveState(&state)
	}

	var cves []string
	for cve, _ := range state.KnownRCA {
		cves = append(cves, cve)
	}
	sort.Slice(cves, func(i, j int) bool {
		// Since we want these is descending order return i > j
		if state.KnownRCA[cves[i]].Disclosed.Equal(*state.KnownRCA[cves[j]].Disclosed) {
			var iyear, iid int
			fmt.Sscanf(cves[i], "CVE-%d-%d", &iyear, &iid)

			var jyear, jid int
			fmt.Sscanf(cves[j], "CVE-%d-%d", &jyear, &jid)

			if iyear == jyear {
				return iid > jid
			}
			return iyear > jyear
		}
		return state.KnownRCA[cves[i]].Disclosed.After(*state.KnownRCA[cves[j]].Disclosed)
	})

	out := feeds.Feed{}
	out.Title = "Project Zero - Root Cause Analysis"
	out.Link = &feeds.Link{Href: "https://googleprojectzero.github.io/0days-in-the-wild/rca.html"}
	out.Updated = time.Now()

	for _, cveid := range cves {
		rca := state.KnownRCA[cveid]
		newItem := &feeds.Item{
			Title: fmt.Sprintf("%s: %s", rca.Cve, rca.Title),
			Link: &feeds.Link{
				Href: fmt.Sprintf(rca.Link),
			},
			Author: &feeds.Author{
				Name: rca.Author,
			},
			Id:      rca.Cve,
			Created: *rca.Disclosed,
			Updated: *rca.Disclosed,
		}

		if g.itemModFunc != nil {
			g.itemModFunc(newItem)
		}

		// BUG: Filtering here will just reduce the feed size ideally we should get a feed
		// with the expected number of elements every time
		if g.filterFunc == nil || g.filterFunc(newItem) {
			out.Items = append(out.Items, newItem)
		}
	}

	return &out, nil

}
