package generators

import (
	"github.com/gorilla/feeds"
)

type ItemFilterFunc func(item *feeds.Item) bool
type ItemModifierFunc func(item *feeds.Item)

type Generator interface {
	Feed() (*feeds.Feed, error)
	RegisterItemFilter(callback ItemFilterFunc)
	RegisterItemModifier(callback ItemModifierFunc)
}
