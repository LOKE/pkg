// Package menuclient is a client-side copy of the benchmark menu types, as a
// client generated from menu.meta.json would declare them. Its protobuf code
// is generated from that JTD file, not from the structs.
package menuclient

type Menu struct {
	Products []Product `json:"products"`
}

type Product struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Price       int32    `json:"price"`
	Available   bool     `json:"available"`
	Tags        []string `json:"tags"`
	Options     []Option `json:"options"`
}

type Option struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price int32  `json:"price"`
}
