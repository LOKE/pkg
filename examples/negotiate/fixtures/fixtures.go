// Package fixtures holds the types the protobuf generator is exercised on:
// the benchmark menu and the edge-value type used by the tests. Generated
// code lives next to it in proto_gen.go; regenerate with go generate ./...
package fixtures

//go:generate go run github.com/tinylib/msgp -file fixtures.go -o msgp_gen.go -io=false -tests=false

//msgp:tag json
//msgp:newtime
//msgp:timezone utc
//msgp:shim netip.Addr as:[]byte using:addrToBytes/addrFromBytes mode:cast

import (
	"fmt"
	"net/netip"
	"strings"
	"time"
)

type BenchMenu struct {
	Products []BenchProduct `json:"products"`
}

type BenchProduct struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Price       int32         `json:"price"`
	Available   bool          `json:"available"`
	Tags        []string      `json:"tags"`
	Options     []BenchOption `json:"options"`
}

type BenchOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price int32  `json:"price"`
}

type Node struct {
	Name     string `json:"name"`
	Children []Node `json:"children"`
}

type Edge struct {
	OptInt     *int             `json:"optInt,omitempty"`
	Negative   int              `json:"negative"`
	Small      int8             `json:"small"`
	Unsigned   uint32           `json:"unsigned"`
	Float32    float32          `json:"float32"`
	Float64    float64          `json:"float64"`
	Names      []string         `json:"names"`
	Pointers   []*string        `json:"pointers"`
	Matrix     [][]int          `json:"matrix"`
	Groups     map[string][]int `json:"groups"`
	Tree       *Node            `json:"tree,omitempty"`
	When       time.Time        `json:"when"`
	Anything   any              `json:"anything"`
	Hyphenated string           `json:"hy-phen"`
	Addr       netip.Addr       `json:"addr"`
}

// LargeMenu is 200 products with long descriptions, 3 tags and 8 options each.
func LargeMenu() BenchMenu {
	menu := BenchMenu{}
	for i := range 200 {
		product := BenchProduct{
			ID: fmt.Sprintf("product-%03d", i), Name: fmt.Sprintf("Roasted seasonal bowl %d", i),
			Description: strings.Repeat("Freshly prepared seasonal vegetables, grains and house dressing. ", 23),
			Price:       int32(1200 + i), Available: true, Tags: []string{"seasonal", "vegetarian", "popular"},
		}
		for j := range 8 {
			product.Options = append(product.Options, BenchOption{ID: fmt.Sprintf("option-%d", j), Name: fmt.Sprintf("Extra topping %d", j), Price: int32(j * 75)})
		}
		menu.Products = append(menu.Products, product)
	}
	return menu
}
