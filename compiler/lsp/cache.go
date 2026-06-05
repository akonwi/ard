package lsp

import (
	"sync"

	"go.lsp.dev/uri"
)

// Doc holds the state of a single open document in the workspace.
type Doc struct {
	URI      uri.URI
	Language string
	Version  int32
	Text     string
}

// DocumentCache tracks all open documents for the LSP workspace.
type DocumentCache struct {
	mu   sync.Mutex
	docs map[uri.URI]*Doc
}

// NewDocumentCache creates an empty document cache.
func NewDocumentCache() *DocumentCache {
	return &DocumentCache{
		docs: make(map[uri.URI]*Doc),
	}
}

// Open registers a document that was opened in the editor.
func (c *DocumentCache) Open(u uri.URI, language string, version int32, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.docs[u] = &Doc{
		URI:      u,
		Language: language,
		Version:  version,
		Text:     text,
	}
}

// Update replaces the content of an already-open document.
func (c *DocumentCache) Update(u uri.URI, version int32, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if doc, ok := c.docs[u]; ok {
		doc.Version = version
		doc.Text = text
	}
}

// Close removes a document from the cache.
func (c *DocumentCache) Close(u uri.URI) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.docs, u)
}

// Get returns a snapshot copy of the document for the given URI, or nil if not open.
// Returning a copy prevents async diagnostics and feature handlers from observing
// partially-updated document state after the cache lock is released.
func (c *DocumentCache) Get(u uri.URI) *Doc {
	c.mu.Lock()
	defer c.mu.Unlock()
	doc := c.docs[u]
	if doc == nil {
		return nil
	}
	copy := *doc
	return &copy
}

// Snapshot returns copies of all cached documents.
func (c *DocumentCache) Snapshot() []Doc {
	c.mu.Lock()
	defer c.mu.Unlock()

	docs := make([]Doc, 0, len(c.docs))
	for _, doc := range c.docs {
		if doc == nil {
			continue
		}
		docs = append(docs, *doc)
	}
	return docs
}
