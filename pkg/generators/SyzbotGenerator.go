package generators

import (
	"encoding/json"
	"fmt"
	"github.com/andybalholm/cascadia"
	"github.com/gorilla/feeds"
	"golang.org/x/net/html"
	"io/ioutil"
	url2 "net/url"
	"os"
	"sort"
	"strings"
	"time"
)

type SyzbotCrash struct {
	Id           string    `json:"id"`
	Title        string    `json:"title"`
	Repro        string    `json:"repro"`
	BisectStatus string    `json:"bisect_status"`
	FirstSeen    time.Time `json:"first_seen"`
}

type syzbotState struct {
	Crashes map[string]SyzbotCrash
}

type SyzbotGenerator struct {
	filterFunc  ItemFilterFunc
	itemModFunc ItemModifierFunc
	workdir     string
}

func (g *SyzbotGenerator) WorkDir(path string) error {
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

func (g *SyzbotGenerator) RegisterItemFilter(callback ItemFilterFunc) {
	g.filterFunc = callback
}
func (g *SyzbotGenerator) RegisterItemModifier(callback ItemModifierFunc) {
	g.itemModFunc = callback
}

func (g *SyzbotGenerator) saveState(state *syzbotState) error {
	fp, err := os.OpenFile(g.workdir+"/syzbot.state.json", os.O_WRONLY|os.O_CREATE, 0644)
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

func (g *SyzbotGenerator) loadState() (syzbotState, error) {
	content, err := ioutil.ReadFile(g.workdir + "/syzbot.state.json")
	if err != nil {
		if os.IsNotExist(err) {
			return syzbotState{
				make(map[string]SyzbotCrash),
			}, nil
		}
		return syzbotState{}, err
	}

	var out syzbotState
	err = json.Unmarshal(content, &out)
	return out, err
}

func (g *SyzbotGenerator) CompileSelector(selector string) (*cascadia.Selector, error) {
	sel, err := cascadia.Compile(selector)
	return &sel, err
}

func (g *SyzbotGenerator) Feed() (*feeds.Feed, error) {
	url := "https://syzkaller.appspot.com/upstream"
	cssg := CssGenerator{}
	contentSelector, err := g.CompileSelector("table.list_table:nth-of-type(2) tbody tr")
	if err != nil {
		return nil, err
	}
	titleSelector, err := g.CompileSelector("td.title")
	if err != nil {
		return nil, err
	}
	linkSelector, err := g.CompileSelector("td.title a")
	if err != nil {
		return nil, err
	}
	reproSelector, err := g.CompileSelector("td.stat")
	if err != nil {
		return nil, err
	}
	bisectStatusSelector, err := g.CompileSelector("td.bisect_status")
	if err != nil {
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

	var crashes []*SyzbotCrash
	for _, itemNode := range contentSelector.MatchAll(node) {
		crash := SyzbotCrash{}
		if titleNode := titleSelector.MatchFirst(itemNode); titleNode != nil {
			crash.Title, _ = cssg.GetText(titleNode)
		}

		if linkNode := linkSelector.MatchFirst(itemNode); linkNode != nil {
			link, err := cssg.GetLink(linkNode)
			if err != nil {
				panic(err)
			}
			if strings.Contains(link, "id=") {
				parsedLink, _ := url2.Parse(link)
				q := parsedLink.Query()
				crash.Id = q.Get("id")
			}
		}
		if reproNode := reproSelector.MatchFirst(itemNode); reproNode != nil {
			crash.Repro, _ = cssg.GetText(reproNode)
		}
		if bisectNode := bisectStatusSelector.MatchFirst(itemNode); bisectNode != nil {
			crash.BisectStatus, _ = cssg.GetText(bisectNode)
		}
		crashes = append(crashes, &crash)
	}

	hasUpdate := false
	for i, crash := range crashes {
		if _, found := state.Crashes[crash.Id]; !found {
			hasUpdate = true
			crashes[i].FirstSeen = time.Now()
			state.Crashes[crash.Id] = *crashes[i]
		} else {
			crashes[i].FirstSeen = state.Crashes[crash.Id].FirstSeen
		}
	}

	if hasUpdate {
		g.saveState(&state)
	}

	sort.Slice(crashes, func(i, j int) bool {
		// Since we want these is descending order return i > j
		return crashes[i].FirstSeen.After(crashes[j].FirstSeen)
	})

	out := feeds.Feed{}
	out.Title = "Syzbot - Upstream Crashes"
	out.Link = &feeds.Link{Href: "https://syzkaller.appspot.com/upstream"}
	out.Updated = time.Now()

	for _, crash := range crashes {

		newItem := &feeds.Item{
			Title: crash.Title,
			Link: &feeds.Link{
				Href: fmt.Sprintf("https://syzkaller.appspot.com/bug?id=%s", crash.Id),
			},
			Author: &feeds.Author{
				Name: "Syzbot",
			},
			Id:      crash.Id,
			Created: crash.FirstSeen,
			Updated: crash.FirstSeen,
		}

		if g.itemModFunc != nil {
			g.itemModFunc(newItem)
		}

		if g.filterFunc == nil || g.filterFunc(newItem) {
			out.Items = append(out.Items, newItem)
		}
		if len(out.Items) >= 20 {
			// Break here instead of just iterating over the first 20 to account for the filter removing some
			break
		}
	}

	return &out, nil
}
