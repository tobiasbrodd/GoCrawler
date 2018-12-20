package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type response struct {
	url  string
	body string
	urls []string
}

type result struct {
	url  string
	body string
}

// Fetcher fetches responses
type Fetcher interface {
	Fetch(url string) (resp response, err error)
}

// Parser parses responses
type Parser interface {
	Parse(resp response) (res result)
}

var responses = make(chan response)
var results = make(chan result)

var visit = make(chan string)
var sites = make(chan string)

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
func SitesHandler() {
	visited := map[string]bool{}

	for url := range sites {
		if _, ok := visited[url]; ok {
			fmt.Printf("Already visited %v\n", url)
		} else {
			visited[url] = true
			IncreaseSitesLeft()
			visit <- url
		}
	}

	close(visit)
}

// Crawler crawls a site
func Crawler(url string, fetcher Fetcher) {
	fmt.Printf("Crawling %v\n", url)

	resp, err := fetcher.Fetch(url)

	if err != nil {
		fmt.Printf("Error on %v: %v\n", url, err)
		DecreaseSitesLeft()
		return
	}

	responses <- resp

	for _, url := range resp.urls {
		sites <- url
	}

	DecreaseSitesLeft()
}

// Crawl the web
func Crawl(baseURL string) {
	fmt.Printf("Begin crawl using base URL: %v\n", baseURL)

	go SitesHandler()

	sites <- baseURL
	for url := range visit {
		go Crawler(url, fetcher)
	}

	close(responses)
}

// Analyser converts a response to a result
func Analyser(resp response, parser Parser) {
	fmt.Printf("Analyse response: %v\n", resp.url)

	results <- parser.Parse(resp)
	waitGroup.Done()
}

// Analyse responses
func Analyse() {
	fmt.Println("Begin analyse")

	parser := fakeParser{}

	for resp := range responses {
		waitGroup.Add(1)
		go Analyser(resp, parser)
	}

	waitGroup.Wait()
	close(results)
}

func main() {
	go Crawl("https://golang.org/")
	go Analyse()

	for res := range results {
		fmt.Printf("Result: %v\n", res.url)
	}
}

type fakeResponse struct {
	body string
	urls []string
}

type fakeFetcher map[string]*fakeResponse

func (f *fakeFetcher) Fetch(url string) (resp response, err error) {
	if resp, ok := (*f)[url]; ok {
		return response{url, resp.body, resp.urls}, nil
	}
	return response{url, "", nil}, fmt.Errorf("Not found: %s", url)
}

type fakeParser struct{}

func (f fakeParser) Parse(resp response) (res result) {
	return result{resp.url, resp.body}
}

var fetcher = &fakeFetcher{
	"https://golang.org/": &fakeResponse{
		"The Go Programming Language",
		[]string{
			"https://golang.org/pkg/",
			"https://golang.org/cmd/",
		},
	},
}
