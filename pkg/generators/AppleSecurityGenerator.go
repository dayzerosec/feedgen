package generators

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/feeds"
	"golang.org/x/net/html"
	"net/http"
	"time"
)

type AppleSecurityGenerator struct {
	filterFunc  ItemFilterFunc
	itemModFunc ItemModifierFunc
}
type Page struct {
	Props struct {
		PageProps struct {
			Blogs []BlogEntry `json:"blogs"`
		} `json:"pageProps"`
	} `json:"props"`
}

type BlogEntry struct {
	Id          string `json:"id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Date        string `json:"date"`
}

func (e BlogEntry) Time() time.Time {
	t, _ := time.Parse("2006-01-02", e.Date)
	return t
}

func (e BlogEntry) Link() string {
	return fmt.Sprintf("https://security.apple.com/blog/%s", e.Slug)
}

func (g *AppleSecurityGenerator) RegisterItemFilter(callback ItemFilterFunc) {
	g.filterFunc = callback
}
func (g *AppleSecurityGenerator) RegisterItemModifier(callback ItemModifierFunc) {
	g.itemModFunc = callback
}

func (g *AppleSecurityGenerator) Feed() (*feeds.Feed, error) {
	page, err := g.getParsedPage()
	if err != nil {
		return nil, err
	}

	feed := feeds.Feed{
		Title:   "Apple Security Research",
		Link:    &feeds.Link{Href: "https://security.apple.com/blog/"},
		Updated: time.Now(),
	}

	for _, blog := range page.Props.PageProps.Blogs {
		item := &feeds.Item{
			Title:   blog.Title,
			Link:    &feeds.Link{Href: blog.Link()},
			Created: blog.Time(),
			Updated: blog.Time(),
			Content: blog.Description,
		}
		if g.filterFunc != nil && !g.filterFunc(item) {
			continue
		}
		if g.itemModFunc != nil {
			g.itemModFunc(item)
		}
		feed.Items = append(feed.Items, item)
	}
	return &feed, nil
}

func getJsonNode(nodes *html.Node) *html.Node {
	if nodes.Type == html.ElementNode && nodes.Data == "script" {
		for _, attr := range nodes.Attr {
			if attr.Key == "type" && attr.Val == "application/json" {
				return nodes
			}
		}
	}
	for c := nodes.FirstChild; c != nil; c = c.NextSibling {
		if data := getJsonNode(c); data != nil {
			return data
		}
	}
	return nil
}

func (g *AppleSecurityGenerator) getParsedPage() (*Page, error) {
	res, err := http.Get("https://security.apple.com/blog/")
	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get Apple Security blog: %d", res.StatusCode)
	}

	defer func() { _ = res.Body.Close() }()

	nodes, err := html.Parse(res.Body)
	if err != nil {
		return nil, err
	}

	jsonNode := getJsonNode(nodes)
	if jsonNode == nil {
		return nil, fmt.Errorf("failed to find JSON node")
	}

	var page Page
	if err = json.Unmarshal([]byte(jsonNode.FirstChild.Data), &page); err != nil {
		return nil, err
	}
	return &page, nil
}
