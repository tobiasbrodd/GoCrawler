package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/net/html"
)

// ---------- Crawler ----------

type site struct {
	url   string
	depth int
}

var responses = make(chan response)
var results = make(chan result)

var visit = make(chan site)
var sites = make(chan site)

var done = make(chan bool)

var waitGroup sync.WaitGroup

var sitesLeft int64

// IncreaseSitesLeft increases sites left
func IncreaseSitesLeft() {
	atomic.AddInt64(&sitesLeft, 1)
}

// DecreaseSitesLeft decreases sites left
func DecreaseSitesLeft() {
	atomic.AddInt64(&sitesLeft, -1)
	if atomic.LoadInt64(&sitesLeft) == 0 {
		close(sites)
	}
}

// SitesHandler handles the sites channel
func SitesHandler(verbose bool) {
	visited := map[string]bool{}

	for s := range sites {
		url := s.url
		if _, ok := visited[url]; ok {
			if verbose {
				fmt.Printf("Already visited %v\n", url)
			}
		} else {
			visited[url] = true
			IncreaseSitesLeft()
			visit <- s
		}
	}

	close(visit)
}

// Crawler crawls a site
func Crawler(s site, depth int, fetcher Fetcher, verbose bool) {
	if verbose {
		fmt.Printf("Crawling URL: %v\n", s.url)
	}

	resp, err := fetcher.Fetch(s.url)

	if err != nil {
		if verbose {
			fmt.Printf("Error on %v: %v\n", s.url, err)
		}
		DecreaseSitesLeft()
		return
	}

	responses <- resp

	if s.depth >= depth {
		if verbose {
			fmt.Printf("Reached max depth: %v\n", depth)
		}
		DecreaseSitesLeft()
		return
	}

	for _, url := range resp.urls {
		sites <- site{url, s.depth + 1}
	}

	DecreaseSitesLeft()
}

// Crawl the web
func Crawl(baseURL string, depth int, verbose bool) {
	fetcher := fetcher{}

	go SitesHandler(verbose)

	sites <- site{baseURL, 1}
	for s := range visit {
		go Crawler(s, depth, fetcher, verbose)
	}

	close(responses)
}

// Analyser converts a response to a result
func Analyser(resp response, parser Parser, verbose bool) {
	if verbose {
		fmt.Printf("Analysing response from: %v\n", resp.url)
	}

	results <- parser.Parse(resp)
	waitGroup.Done()
}

// Analyse responses
func Analyse(verbose bool) {
	parser := parser{}

	for resp := range responses {
		waitGroup.Add(1)
		go Analyser(resp, parser, verbose)
	}

	waitGroup.Wait()
	close(results)
}

func main() {
	url := flag.String("url", "https://golang.org/", "Set starting URL.")
	depth := flag.Int("depth", 1, "Set to >= 1 to specify depth.")
	verbose := flag.Bool("verbose", true, "Set to false to disable printing.")

	flag.Parse()

	go Crawl(*url, *depth, *verbose)
	go Analyse(*verbose)

	for res := range results {
		fmt.Printf("Result: %v\n", res.url)
	}
}

// ---------- Fetcher ----------

type response struct {
	url  string
	urls []string
}

// Fetcher fetches responses
type Fetcher interface {
	Fetch(url string) (resp response, err error)
}

type fetcher struct{}

// Fetch fetches URLs
func (f fetcher) Fetch(url string) (response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return response{url, []string{}}, err
	}

	return response{url, GetAllLinks(url, resp.Body)}, nil
}

// ---------- Parser ----------

type result struct {
	url string
}

// Parser parses responses
type Parser interface {
	Parse(resp response) (res result)
}

type parser struct{}

// Parse parses responses
func (p parser) Parse(resp response) result {
	return result{resp.url}
}

// ---------- Links ----------

// GetAllLinks retrieves all links from a HTML body
func GetAllLinks(baseURL string, body io.Reader) []string {
	var links []string
	page := html.NewTokenizer(body)
	for {
		tokenType := page.Next()

		switch tokenType {
		case html.ErrorToken:
			return links
		case html.StartTagToken, html.EndTagToken:
			token := page.Token()
			if "a" == token.Data {
				for _, attr := range token.Attr {
					if attr.Key == "href" {
						link := TrimLink(attr.Val)
						if len(link) != 0 {
							link = FixLink(baseURL, link)
							links = append(links, link)
						}
					}
				}
			}
		}
	}
}

// TrimLink removes characters in links
func TrimLink(link string) string {
	link = strings.TrimSpace(link)
	link = strings.SplitN(link, "#", 2)[0]
	link = strings.Trim(link, "#")
	link = strings.TrimSpace(link)

	return link
}

// FixLink fixes broken links
func FixLink(baseURL string, link string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if len(link) > 1 && link[0:2] == "//" {
		link = strings.TrimLeft(link, "/")
		link = strings.Join([]string{"http://", link}, "")
	} else if link[0] == '/' {
		link = strings.TrimLeft(link, "/")
		link = strings.Join([]string{baseURL, link}, "/")
	}

	return link
}
