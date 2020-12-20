package generators

import (
	"errors"
	"github.com/gorilla/feeds"
	"github.com/mmcdole/gofeed"
)

// RssGenerator might seem weird to start with a feed for an RSS generator but the idea here is to
// enable feeds to be filtered, or otherwise modified (like pulling the content or fixing dates)
type RssGenerator struct {
	filterFunc  ItemFilterFunc
	itemModFunc ItemModifierFunc
	Url         string
}

func (g *RssGenerator) Feed() (*feeds.Feed, error) {
	if g.Url == "" {
		return nil, errors.New("Missing .Url field value")
	}

	parser := gofeed.NewParser()
	original, err := parser.ParseURL(g.Url)
	if err != nil {
		return nil, err
	}
	feed := &feeds.Feed{}
	feed.Title = original.Title
	feed.Description = original.Description
	feed.Author = &feeds.Author{original.Author.Name, original.Author.Email}
	feed.Link = &feeds.Link{Href: original.Link}
	for _, item := range original.Items {
		newItem := &feeds.Item{
			Title:       item.Title,
			Link:        &feeds.Link{Href: item.Link},
			Author:      &feeds.Author{item.Author.Name, item.Author.Email},
			Description: item.Description,
			Id:          item.GUID,
			Updated:     *item.UpdatedParsed,
			Content:     item.Content,
		}
		if g.itemModFunc != nil {
			g.itemModFunc(newItem)
		}
		if g.filterFunc == nil || g.filterFunc(newItem) {
			feed.Items = append(feed.Items, newItem)
		}
	}
	return feed, nil
}

func (g *RssGenerator) RegisterItemFilter(callback ItemFilterFunc) {
	g.filterFunc = callback
}
func (g *RssGenerator) RegisterItemModifier(callback ItemModifierFunc) {
	g.itemModFunc = callback
}
