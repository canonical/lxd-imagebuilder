package stream

import (
	"sort"
	"time"
)

type StreamIndexEntry struct {
	Datatype string   `json:"datatype"`
	Path     string   `json:"path"`
	Format   string   `json:"format"`
	Updated  string   `json:"updated"`
	Products []string `json:"products"`
}

type StreamIndex struct {
	Format string                      `json:"format"`
	Index  map[string]StreamIndexEntry `json:"index"`
}

// NewStreamIndex creates new empty index.
func NewStreamIndex() StreamIndex {
	return StreamIndex{
		Format: "index:1.0",
		Index:  make(map[string]StreamIndexEntry),
	}
}

// AddEntry adds catalog and a list of its products to the index.
func (i *StreamIndex) AddEntry(streamName string, catalogPath string, catalog ProductCatalog) {
	products := make([]string, 0, len(catalog.Products))
	for p := range catalog.Products {
		products = append(products, p)
	}

	sort.Strings(products)

	i.Index[streamName] = StreamIndexEntry{
		Format:   "products:1.0",
		Path:     catalogPath,
		Datatype: catalog.DataType,
		Updated:  time.Now().Format(time.RFC3339),
		Products: products,
	}
}
