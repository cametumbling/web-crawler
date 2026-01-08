package htmlparser

import (
	"io"

	"golang.org/x/net/html"
)

// ExtractLinks parses HTML from the reader and returns all href attributes
// found in <a> tags. Returns raw href strings exactly as they appear in the HTML.
func ExtractLinks(r io.Reader) ([]string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	var links []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					links = append(links, attr.Val)
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return links, nil
}
