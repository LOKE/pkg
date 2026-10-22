package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/LOKE/pkg/errors"
	"github.com/LOKE/pkg/lokerpc"
)

type Menu struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Sections []Section `json:"sections"`
}

type Section struct {
	Title    string     `json:"title"`
	Items    []MenuItem `json:"items"`
	Children []Section  `json:"children"`
}

type MenuItem struct {
	ID          int               `json:"id"`
	Name        string            `json:"name"`
	Description *string           `json:"description,omitempty"`
	PriceCents  int               `json:"priceCents"`
	Available   bool              `json:"available"`
	GlutenFree  bool              `json:"gluten-free"`
	Tags        []string          `json:"tags"`
	Options     []Option          `json:"options"`
	Calories    *int              `json:"calories,omitempty"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	Labels      map[string]string `json:"labels"`
	Metadata    any               `json:"metadata"`
}

type Option struct {
	Name       string `json:"name"`
	ExtraCents int    `json:"extraCents"`
}

type GetMenuRequest struct {
	MenuID string `json:"menuId"`
}

type SaveResult struct {
	Revision int       `json:"revision"`
	SavedAt  time.Time `json:"savedAt"`
}

type DeleteMenuRequest struct {
	MenuID string `json:"menuId"`
}

// MenuClient is shaped like the output of the lokerpc client generator.
type MenuClient struct {
	lokerpc.Client
}

func (c MenuClient) GetMenu(ctx context.Context, req GetMenuRequest) (Menu, error) {
	var res Menu
	err := c.DoRequest(ctx, "getMenu", req, &res)
	return res, err
}

func (c MenuClient) SaveMenu(ctx context.Context, req Menu) (SaveResult, error) {
	var res SaveResult
	err := c.DoRequest(ctx, "saveMenu", req, &res)
	return res, err
}

func (c MenuClient) DeleteMenu(ctx context.Context, req DeleteMenuRequest) error {
	return c.DoRequest(ctx, "deleteMenu", req, nil)
}

type menuStore struct {
	mu        sync.Mutex
	menus     map[string]Menu
	revisions map[string]int
}

func newMenuService(codecs ...lokerpc.Codec) *lokerpc.Service {
	store := &menuStore{menus: map[string]Menu{}, revisions: map[string]int{}}

	var opts []lokerpc.ServiceOption
	if len(codecs) > 0 {
		opts = append(opts, lokerpc.ServeCodecs(codecs...))
	}

	return lokerpc.NewService("menu", "Menus", lokerpc.EndpointCodecMap{
		"getMenu":    lokerpc.MakeStandardEndpointCodec(store.getMenu, "Fetch a menu"),
		"saveMenu":   lokerpc.MakeStandardEndpointCodec(store.saveMenu, "Store a menu"),
		"deleteMenu": lokerpc.MakeVoidEndpointCodec(store.deleteMenu, "Delete a menu"),
	}, opts...)
}

func (s *menuStore) getMenu(_ context.Context, req GetMenuRequest) (Menu, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	menu, ok := s.menus[req.MenuID]
	if !ok {
		return Menu{}, errors.NewPublicError("menu_not_found", fmt.Sprintf("no menu %q", req.MenuID))
	}
	return menu, nil
}

func (s *menuStore) saveMenu(_ context.Context, menu Menu) (SaveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.menus[menu.ID] = menu
	s.revisions[menu.ID]++
	return SaveResult{Revision: s.revisions[menu.ID], SavedAt: time.Now().UTC()}, nil
}

func (s *menuStore) deleteMenu(_ context.Context, req DeleteMenuRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.menus, req.MenuID)
	return nil
}

// seedMenu builds a menu with itemCount items spread over a few sections.
func seedMenu(itemCount int) Menu {
	updatedAt := time.Date(2026, 3, 14, 9, 26, 53, 589793000, time.UTC)
	menu := Menu{ID: "lunch", Name: "Lunch"}

	for i := range itemCount {
		if i%25 == 0 {
			menu.Sections = append(menu.Sections, Section{
				Title:    fmt.Sprintf("Section %d", i/25+1),
				Children: []Section{{Title: "Specials"}},
			})
		}

		item := MenuItem{
			ID:         i + 1,
			Name:       fmt.Sprintf("Dish number %d", i+1),
			PriceCents: 850 + i*5,
			Available:  i%7 != 0,
			GlutenFree: i%3 == 0,
			Tags:       []string{"lunch", "popular"},
			Options:    []Option{{Name: "Extra cheese", ExtraCents: 150}, {Name: "Large", ExtraCents: 300}},
			UpdatedAt:  updatedAt.Add(time.Duration(i) * time.Minute),
			Labels:     map[string]string{"station": "grill", "course": "main"},
			Metadata:   map[string]any{"rank": float64(i), "seasonal": i%2 == 0},
		}
		if i%2 == 0 {
			item.Description = new(fmt.Sprintf("A hearty serving of dish %d with house sauce", i+1))
			item.Calories = new(400 + i)
		}

		section := &menu.Sections[len(menu.Sections)-1]
		section.Items = append(section.Items, item)
	}
	return menu
}
