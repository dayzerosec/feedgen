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
	"time"
)

type H1Generator struct {
	filterFunc  ItemFilterFunc
	itemModFunc ItemModifierFunc
}

func (g *H1Generator) Feed() (*feeds.Feed, error) {
	data, _ := g.query()
	feed := feeds.Feed{
		Title:   "HackerOne Recently Disclosed",
		Link:    &feeds.Link{Href: "https://hackerone.com/hacktivity?querystring=&filter=type:public&order_direction=DESC&order_field=latest_disclosable_activity_at&followed_only=false"},
		Updated: time.Now(),
	}
	if _, ok := data["data"]; !ok {
		return nil, errors.New("Unable to parse HackerOne response. resp[data]")
	}
	data, _ = data["data"].(map[string]interface{})

	if _, ok := data["hacktivity_items"]; !ok {
		return nil, errors.New("Unable to parse HackerOne response. resp[data][hacktivity_items]")
	}
	data, _ = data["hacktivity_items"].(map[string]interface{})

	if _, ok := data["edges"]; !ok {
		return nil, errors.New("Unable to parse HackerOne response. resp[data][hacktivity_items][edges]")
	}
	for _, e := range data["edges"].([]interface{}) {
		edge := e.(map[string]interface{})
		if _, ok := edge["node"].(map[string]interface{}); !ok {
			return nil, errors.New("Edge is missing key(node)")
		}
		node, _ := edge["node"].(map[string]interface{})
		nodeType := node["__typename"].(string)
		if nodeType != "Disclosed" {
			continue
		}

		// Finally, we are in the individual reports
		report := struct {
			Id, Reporter, Team, Title, Url, Currency, Severity string
			Bounty                                             float64
			Modified                                           time.Time
		}{}

		for key, val := range node {
			if val == nil {
				continue
			}
			switch key {
			case "reporter":
				item := val.(map[string]interface{})
				report.Reporter = item["username"].(string)
			case "team":
				item := val.(map[string]interface{})
				report.Team = item["name"].(string)
			case "report":
				item := val.(map[string]interface{})
				report.Id = item["id"].(string)
				report.Title = item["title"].(string)
				report.Url = item["url"].(string)
			case "severity_rating":
				switch item := val.(type) {
				case string:
					report.Severity = item
				}
			case "total_awarded_amount":
				switch item := val.(type) {
				case int:
					report.Bounty = float64(item)
				case float64:
					report.Bounty = item
				}
			case "currency":
				switch item := val.(type) {
				case string:
					report.Currency = item
				}
			case "latest_disclosable_activity_at":
				switch item := val.(type) {
				case string:
					ts, err := time.Parse(time.RFC3339Nano, item)
					if err != nil {
						log.Printf("Failed to parse '%s' -- %s\n", item, err.Error())
					}
					report.Modified = ts
				}

			}

		}
		title := fmt.Sprintf("[%s] %s - %s", report.Team, report.Severity, report.Title)
		if report.Bounty > 0 {
			title = fmt.Sprintf("%s (%.2f%s)", title, report.Bounty, report.Currency)
		}

		newItem := &feeds.Item{
			Title:       title,
			Link:        &feeds.Link{Href: report.Url},
			Source:      nil,
			Author:      &feeds.Author{Name: report.Reporter},
			Description: "",
			Id:          report.Id,
			Updated:     report.Modified,
			Created:     time.Time{},
			Enclosure:   nil,
			Content:     "",
		}

		if g.itemModFunc != nil {
			g.itemModFunc(newItem)
		}

		if g.filterFunc == nil || g.filterFunc(newItem) {
			feed.Items = append(feed.Items, newItem)
		}
	}
	return &feed, nil
}

func (g *H1Generator) RegisterItemFilter(callback ItemFilterFunc) {
	g.filterFunc = callback
}
func (g *H1Generator) RegisterItemModifier(callback ItemModifierFunc) {
	g.itemModFunc = callback
}

// mockQuery will just pull the response body from a text file instead of hitting the api
func (g *H1Generator) mockQuery(filename string) (map[string]interface{}, error) {
	body, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	out := make(map[string]interface{})
	err = json.Unmarshal(body, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (g *H1Generator) query() (map[string]interface{}, error) {
	url := "https://hackerone.com/graphql"
	body := `{
	  "operationName": "HacktivityPageQuery",
	  "variables": {
		"querystring": "",
		"where": {
		  "report": {
			"disclosed_at": {
			  "_is_null": false
			}
		  }
		},
		"orderBy": null,
		"secureOrderBy": {
		  "latest_disclosable_activity_at": {
			"_direction": "DESC"
		  }
		},
		"count": 25,
		"maxShownVoters": 10
	  },
	  "query": "query HacktivityPageQuery($querystring: String, $orderBy: HacktivityItemOrderInput, $secureOrderBy: FiltersHacktivityItemFilterOrder, $where: FiltersHacktivityItemFilterInput, $count: Int, $cursor: String, $maxShownVoters: Int) {\n  me {\n    id\n    __typename\n  }\n  hacktivity_items(first: $count, after: $cursor, query: $querystring, order_by: $orderBy, secure_order_by: $secureOrderBy, where: $where) {\n    total_count\n    ...HacktivityList\n    __typename\n  }\n}\n\nfragment HacktivityList on HacktivityItemConnection {\n  total_count\n  pageInfo {\n    endCursor\n    hasNextPage\n    __typename\n  }\n  edges {\n    node {\n      ... on HacktivityItemInterface {\n        id\n        databaseId: _id\n        ...HacktivityItem\n        __typename\n      }\n      __typename\n    }\n    __typename\n  }\n  __typename\n}\n\nfragment HacktivityItem on HacktivityItemUnion {\n  type: __typename\n  ... on HacktivityItemInterface {\n    id\n    votes {\n      total_count\n      __typename\n    }\n    voters: votes(last: $maxShownVoters) {\n      edges {\n        node {\n          id\n          user {\n            id\n            username\n            __typename\n          }\n          __typename\n        }\n        __typename\n      }\n      __typename\n    }\n    upvoted: upvoted_by_current_user\n    __typename\n  }\n  ... on Undisclosed {\n    id\n    ...HacktivityItemUndisclosed\n    __typename\n  }\n  ... on Disclosed {\n    id\n    ...HacktivityItemDisclosed\n    __typename\n  }\n  ... on HackerPublished {\n    id\n    ...HacktivityItemHackerPublished\n    __typename\n  }\n}\n\nfragment HacktivityItemUndisclosed on Undisclosed {\n  id\n  reporter {\n    id\n    username\n    ...UserLinkWithMiniProfile\n    __typename\n  }\n  team {\n    handle\n    name\n    medium_profile_picture: profile_picture(size: medium)\n    url\n    id\n    ...TeamLinkWithMiniProfile\n    __typename\n  }\n  latest_disclosable_action\n  latest_disclosable_activity_at\n  requires_view_privilege\n  total_awarded_amount\n  currency\n  __typename\n}\n\nfragment TeamLinkWithMiniProfile on Team {\n  id\n  handle\n  name\n  __typename\n}\n\nfragment UserLinkWithMiniProfile on User {\n  id\n  username\n  __typename\n}\n\nfragment HacktivityItemDisclosed on Disclosed {\n  id\n  reporter {\n    id\n    username\n    ...UserLinkWithMiniProfile\n    __typename\n  }\n  team {\n    handle\n    name\n    medium_profile_picture: profile_picture(size: medium)\n    url\n    id\n    ...TeamLinkWithMiniProfile\n    __typename\n  }\n  report {\n    id\n    title\n    substate\n    url\n    __typename\n  }\n  latest_disclosable_action\n  latest_disclosable_activity_at\n  total_awarded_amount\n  severity_rating\n  currency\n  __typename\n}\n\nfragment HacktivityItemHackerPublished on HackerPublished {\n  id\n  reporter {\n    id\n    username\n    ...UserLinkWithMiniProfile\n    __typename\n  }\n  team {\n    id\n    handle\n    name\n    medium_profile_picture: profile_picture(size: medium)\n    url\n    ...TeamLinkWithMiniProfile\n    __typename\n  }\n  report {\n    id\n    url\n    title\n    substate\n    __typename\n  }\n  latest_disclosable_activity_at\n  severity_rating\n  __typename\n}\n"
	}`
	client := http.Client{
		Timeout: 15 * time.Second,
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("user-agent", "feedgen/0.1")
	req.Header.Add("X-Auth-Token", "----")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	responseBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	out := make(map[string]interface{})
	err = json.Unmarshal(responseBody, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}
