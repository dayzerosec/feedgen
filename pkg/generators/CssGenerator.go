package generators

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/andybalholm/cascadia"
	"github.com/gorilla/feeds"
	"golang.org/x/net/html"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type CssGenerator struct {
	ItemSelectors map[string]*cascadia.Selector
	Url           string
	Title         string
	filterFunc    ItemFilterFunc
	itemModFunc   ItemModifierFunc
	updatedFormat string
	createdFormat string
}

type CssGeneratorConfig struct {
	Url           string `json:"url"`
	Title         string `json:"title"`
	ItemSelectors struct {
		Container     string `json:"container"`
		Title         string `json:"title,omitempty"`
		Link          string `json:"link,omitempty"`
		Author        string `json:"author,omitempty"`
		Description   string `json:"description,omitempty"`
		Id            string `json:"id,omitempty"`
		Updated       string `json:"updated,omitempty"`
		UpdatedFormat string `json:"updated_format,omitempty"`
		Created       string `json:"created,omitempty"`
		CreatedFormat string `json:"created_format,omitempty"`
		Content       string `json:"content,omitempty"`
	} `json:"item_selectors"`
}

func (g *CssGenerator) SetUpdatedTimeFormat(value string) {
	g.updatedFormat = value
}

func (g *CssGenerator) SetCreatedTimeFormat(value string) {
	g.createdFormat = value
}

func (g *CssGenerator) SetItemSelector(name, value string) error {
	if _, err := g.ItemSelectors[name]; !err {
		return errors.New(fmt.Sprintf("Unknown selector name: %s", name))
	}
	if value == "" {
		return nil
	}

	if sel, err := cascadia.Compile(value); err != nil {
		return err
	} else {
		g.ItemSelectors[name] = &sel
	}
	return nil
}

func (g *CssGenerator) SetItemContainer(selector string) error {
	return g.SetItemSelector("Container", selector)
}

func (g *CssGenerator) getText(node *html.Node) (string, error) {
	text := bytes.NewBufferString("")
	html.Render(text, node)
	tokenizer := html.NewTokenizer(text)
	builder := strings.Builder{}
	for {
		tt := tokenizer.Next()
		t := tokenizer.Token()

		err := tokenizer.Err()
		if err == io.EOF {
			break
		}

		switch tt {
		case html.ErrorToken:
			return builder.String(), err
		case html.TextToken:
			//data := strings.TrimSpace(t.Data)
			builder.WriteString(t.Data)
		}
	}
	return builder.String(), nil
}

func (g *CssGenerator) getLink(node *html.Node) (string, error) {
	for _, attr := range node.Attr {
		if strings.ToLower(attr.Key) == "href" {
			link := attr.Val
			if strings.HasPrefix(link, "./") {
				link = fmt.Sprintf("%s%s", g.Url, link[2:])
			} else if strings.HasPrefix(link, "/") {
				// This is nested so it won't fall through to the "is not http(s?)://" check
				if !strings.HasPrefix(link, "//") {
					urlInfo, err := url.Parse(g.Url)
					if err != nil {
						return "", err
					}
					link = fmt.Sprintf("https://%s%s", urlInfo.Hostname(), link)
				}
			} else if !(strings.HasPrefix(link, "https://") || strings.HasPrefix(link, "http://")) {
				link = fmt.Sprintf("%s/%s", g.Url, link)
			}
			return link, nil
		}
	}
	return "", errors.New("Unable to find href attribute for link.")

}

func (g *CssGenerator) fetch() (string, error) {
	client := http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Get(g.Url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (g *CssGenerator) Feed() (*feeds.Feed, error) {
	if g.Url == "" {
		return nil, errors.New("No Url has been set for this feed.")
	}

	res, err := g.fetch()
	if err != nil {
		return nil, err
	}

	node, err := html.Parse(strings.NewReader(res))
	if err != nil {
		return nil, err
	}

	itemSel, ok := g.ItemSelectors["Container"]
	if !ok || itemSel == nil {
		return nil, errors.New("Missing an item Container selector")
	}

	feed := &feeds.Feed{}
	feed.Link = &feeds.Link{Href: g.Url}
	feed.Title = g.Title
	for _, itemNode := range itemSel.MatchAll(node) {
		feedItem := feeds.Item{}
		for key, sel := range g.ItemSelectors {
			if sel == nil || key == "Container" {
				continue
			}

			valueNode := sel.MatchFirst(itemNode)
			if valueNode == nil {
				continue
			}
			textValue, err := g.getText(valueNode)
			textValue = strings.TrimSpace(textValue)
			if err != nil {
				return feed, err
			}
			switch key {
			case "Title":
				feedItem.Title = textValue
			case "Link":
				href, err := g.getLink(valueNode)
				if err != nil {
					log.Println(err.Error())
				}
				feedItem.Link = &feeds.Link{Href: href}
			case "Author":
				feedItem.Author = &feeds.Author{Name: textValue}
			case "Description":
				description := bytes.NewBufferString("")
				html.Render(description, valueNode)
				feedItem.Description = description.String()
			case "Id":
				feedItem.Id = textValue
			case "Content":
				content := bytes.NewBufferString("")
				html.Render(content, valueNode)
				feedItem.Description = content.String()
			case "Updated":
				if g.updatedFormat != "" {
					t, err := time.Parse(g.updatedFormat, textValue)
					if err == nil {
						feedItem.Updated = t
					} else {
						log.Println(err.Error())
					}
				} else {
					log.Println("Missing format for updated timestamp")
				}

			case "Created":
				if g.createdFormat != "" {
					t, err := time.Parse(g.createdFormat, textValue)
					if err == nil {
						feedItem.Updated = t
					} else {
						log.Println(err.Error())
					}
				} else {
					log.Println("Missing format for created timestamp")
				}
			}
		}
		if g.itemModFunc != nil {
			g.itemModFunc(&feedItem)
		}

		if g.filterFunc == nil || g.filterFunc(&feedItem) {
			feed.Items = append(feed.Items, &feedItem)
		}
	}
	return feed, nil
}

func NewCssGenerator(title, url string) CssGenerator {
	return CssGenerator{
		Url:   url,
		Title: title,
		ItemSelectors: map[string]*cascadia.Selector{
			"Title":       nil,
			"Link":        nil,
			"Author":      nil,
			"Description": nil,
			"Id":          nil,
			"Updated":     nil,
			"Created":     nil,
			"Content":     nil,
			"Container":   nil,
		},
		// TODO: Enclosure support
	}
}

func NewCssGeneratorFromConfig(c CssGeneratorConfig) CssGenerator {
	gen := NewCssGenerator(c.Title, c.Url)
	gen.SetItemContainer(c.ItemSelectors.Container)
	gen.SetItemSelector("Title", c.ItemSelectors.Title)
	gen.SetItemSelector("Link", c.ItemSelectors.Link)
	gen.SetItemSelector("Description", c.ItemSelectors.Description)
	gen.SetItemSelector("Author", c.ItemSelectors.Author)
	gen.SetItemSelector("Created", c.ItemSelectors.Created)
	gen.SetItemSelector("Updated", c.ItemSelectors.Updated)
	gen.SetItemSelector("Id", c.ItemSelectors.Id)
	gen.SetItemSelector("Content", c.ItemSelectors.Content)
	gen.SetUpdatedTimeFormat(c.ItemSelectors.UpdatedFormat)
	gen.SetCreatedTimeFormat(c.ItemSelectors.CreatedFormat)
	return gen
}

func NewCssGeneratorFromJson(file string) (CssGenerator, error) {
	c := CssGeneratorConfig{}

	content, err := ioutil.ReadFile(file)
	if err != nil {
		return CssGenerator{}, err
	}

	err = json.Unmarshal(content, &c)
	if err != nil {
		return CssGenerator{}, err
	}

	return NewCssGeneratorFromConfig(c), nil
}

func (g *CssGenerator) RegisterItemFilter(callback ItemFilterFunc) {
	g.filterFunc = callback
}
func (g *CssGenerator) RegisterItemModifier(callback ItemModifierFunc) {
	g.itemModFunc = callback
}
