package lsp

import (
	"fmt"
	"net/url"

	"go.lsp.dev/uri"
)

func filePathFromURI(u uri.URI) (path string, err error) {
	parsed, err := url.ParseRequestURI(string(u))
	if err != nil {
		return "", fmt.Errorf("unsupported document URI %q: %w", u, err)
	}
	if parsed.Scheme != uri.FileScheme {
		return "", fmt.Errorf("unsupported document URI %q: only file:// URIs are supported", u)
	}
	defer func() {
		if r := recover(); r != nil {
			path = ""
			err = fmt.Errorf("unsupported document URI %q: %v", u, r)
		}
	}()
	return u.Filename(), nil
}

func docFilePath(doc *Doc) (string, bool) {
	if doc == nil {
		return "", false
	}
	path, err := filePathFromURI(doc.URI)
	return path, err == nil
}

func overlaySources(docs []Doc) map[string]string {
	overlays := map[string]string{}
	for i := range docs {
		path, ok := docFilePath(&docs[i])
		if !ok {
			continue
		}
		overlays[path] = docs[i].Text
	}
	return overlays
}
