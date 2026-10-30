package crawler

import (
	"bytes"
	"context"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/repository"
)

type Engine struct {
	storage     *repository.SafeMemoryStorage
	workerCount int
	maxPages    int
	fetcher     *Fetcher
}

type CrawlTask struct {
	URL   string
	Level int
}

type crawlResult struct {
	task          CrawlTask
	links         []domain.LinkResult
	statusCode    int
	customStatus  domain.LinkStatus
	redirectChain []string
	bodyBytes     []byte
	finalURL      string
	err           error
}

type taskQueue struct {
	tasks []CrawlTask
	head  int
}

func NewEngine(store *repository.SafeMemoryStorage, workers, maxPages int, fetcher *Fetcher) *Engine {
	if maxPages <= 0 {
		maxPages = 500
	}
	return &Engine{
		storage:     store,
		workerCount: workers,
		maxPages:    maxPages,
		fetcher:     fetcher,
	}
}

func (q *taskQueue) Push(task CrawlTask) {
	q.tasks = append(q.tasks, task)
}

func (q *taskQueue) Pop() (CrawlTask, bool) {
	if q.head >= len(q.tasks) {
		return CrawlTask{}, false
	}
	task := q.tasks[q.head]
	q.head++

	if q.head == len(q.tasks) {
		q.tasks = q.tasks[:0]
		q.head = 0
	}
	return task, true
}

func (q *taskQueue) Peek() (CrawlTask, bool) {
	if q.head >= len(q.tasks) {
		return CrawlTask{}, false
	}
	return q.tasks[q.head], true
}

func (q *taskQueue) Len() int {
	return len(q.tasks) - q.head
}

func isSameHost(hostA, hostB string) bool {
	hA := strings.TrimPrefix(strings.ToLower(strings.Split(hostA, ":")[0]), "www.")
	hB := strings.TrimPrefix(strings.ToLower(strings.Split(hostB, ":")[0]), "www.")
	return hA == hB && hA != ""
}

func (e *Engine) Start(ctx context.Context, startURL string) (bool, time.Time, time.Time) {
	startedAt := time.Now()
	isSSR := true

	parsedStartURL, err := url.Parse(startURL)
	if err != nil {
		log.Printf("[Crawler Engine] Failed to parse start URL %q: %v", startURL, err)
		return isSSR, startedAt, time.Now()
	}
	baseHost := parsedStartURL.Host

	log.Printf("[Crawler Engine] Starting crawl for %s (baseHost=%s, workers=%d, maxPages=%d)", startURL, baseHost, e.workerCount, e.maxPages)

	tasksChan := make(chan CrawlTask)
	resultsChan := make(chan crawlResult, e.workerCount)

	var workersWg sync.WaitGroup
	for i := 1; i <= e.workerCount; i++ {
		workersWg.Add(1)
		go func() {
			defer workersWg.Done()

			sendResult := func(res crawlResult) bool {
				select {
				case <-ctx.Done():
					return false
				case resultsChan <- res:
					return true
				}
			}

			for task := range tasksChan {
				if ctx.Err() != nil {
					return
				}
				res, err := e.fetcher.FetchPage(task.URL)
				if err != nil {
					log.Printf("[Crawler Worker] Fetch error for %s: %v", task.URL, err)
					if !sendResult(crawlResult{task: task, err: err}) {
						return
					}
					continue
				}

				if res.StatusCode >= 400 {
					log.Printf("[Crawler Worker] Page %s returned HTTP status %d", task.URL, res.StatusCode)
					if !sendResult(crawlResult{
						task:          task,
						statusCode:    res.StatusCode,
						customStatus:  res.CustomStatus,
						redirectChain: res.RedirectChain,
						finalURL:      res.FinalURL,
					}) {
						return
					}
					continue
				}

				foundLinks, _ := ExtractDetailedLinks(res.FinalURL, bytes.NewReader(res.Body))
				log.Printf("[Crawler Worker] %s (HTTP %d): extracted %d links", res.FinalURL, res.StatusCode, len(foundLinks))

				if !sendResult(crawlResult{
					task:          task,
					links:         foundLinks,
					statusCode:    res.StatusCode,
					customStatus:  domain.StatusOK,
					redirectChain: res.RedirectChain,
					bodyBytes:     res.Body,
					finalURL:      res.FinalURL,
				}) {
					return
				}
			}
		}()
	}

	extTasksChan := make(chan domain.LinkResult, 100)
	var extWorkersWg sync.WaitGroup
	const extWorkerCount = 20

	for i := 0; i < extWorkerCount; i++ {
		extWorkersWg.Add(1)
		go func() {
			defer extWorkersWg.Done()
			for extItem := range extTasksChan {
				if ctx.Err() != nil {
					return
				}
				httpStatus, chain, customStatus := e.fetcher.CheckExternalLinkWithChain(extItem.URL)
				if extItem.CustomStatus == domain.StatusOK {
					extItem.CustomStatus = customStatus
				}
				extItem.HTTPStatus = httpStatus
				extItem.RedirectChain = chain
				e.storage.UpdateExternal(extItem.URL, extItem)
			}
		}()
	}

	queue := &taskQueue{}
	queue.Push(CrawlTask{URL: startURL, Level: 1})
	e.storage.AddInternal(domain.LinkResult{URL: startURL, DepthLevel: 1})

	inProgress := 0
	crawledCount := 0

	for queue.Len() > 0 || inProgress > 0 {
		var currentTask CrawlTask
		var targetChan chan CrawlTask

		if queue.Len() > 0 {
			currentTask, _ = queue.Peek()
			targetChan = tasksChan
		}
		select {
		case <-ctx.Done():
			log.Printf("[Crawler Engine] Crawl interrupted by context cancellation for %s", startURL)
			goto finishCrawl
		case targetChan <- currentTask:
			queue.Pop()
			inProgress++
			crawledCount++
		case res := <-resultsChan:
			inProgress--

			if res.err != nil {
				e.storage.UpdateInternalDetails(res.task.URL, domain.LinkResult{
					URL:          res.task.URL,
					CustomStatus: domain.StatusBrokenLink,
					HTTPStatus:   0,
				})
				continue
			}

			if res.task.Level == 1 {
				isSSR = CheckIsSSR(string(res.bodyBytes))
				if finalURLParsed, err := url.Parse(res.finalURL); err == nil && finalURLParsed.Host != "" {
					baseHost = finalURLParsed.Host
				}
			}

			e.storage.UpdateInternalDetails(res.task.URL, domain.LinkResult{
				URL:           res.task.URL,
				HTTPStatus:    res.statusCode,
				CustomStatus:  res.customStatus,
				DepthLevel:    res.task.Level,
				RedirectChain: res.redirectChain,
			})

			nextLevel := res.task.Level + 1
			for _, linkItem := range res.links {
				linkItem.DepthLevel = nextLevel

				if linkItem.CustomStatus != domain.StatusOK && linkItem.CustomStatus != domain.StatusFileAsset {
					if e.storage.AddExternal(linkItem.URL, linkItem) {
						select {
						case extTasksChan <- linkItem:
						case <-ctx.Done():
							goto finishCrawl
						}
					}
					continue
				}

				u, err := url.Parse(linkItem.URL)
				if err != nil {
					linkItem.CustomStatus = domain.StatusInvalidProtocol
					if e.storage.AddExternal(linkItem.URL, linkItem) {
						select {
						case extTasksChan <- linkItem:
						case <-ctx.Done():
							goto finishCrawl
						}
					}
					continue
				}

				if isSameHost(u.Host, baseHost) {
					if e.storage.AddInternal(linkItem) {
						if linkItem.CustomStatus != domain.StatusFileAsset && (crawledCount+queue.Len()) < e.maxPages {
							queue.Push(CrawlTask{URL: linkItem.URL, Level: nextLevel})
						}
					}
				} else {
					if e.storage.AddExternal(linkItem.URL, linkItem) {
						select {
						case extTasksChan <- linkItem:
						case <-ctx.Done():
							goto finishCrawl
						}
					}
				}
			}
		}
	}

finishCrawl:
	close(tasksChan)
	workersWg.Wait()
	close(extTasksChan)
	extWorkersWg.Wait()
	e.storage.MarkFinished()

	finishedAt := time.Now()
	prog := e.storage.GetProgress()
	log.Printf("[Crawler Engine] Crawl complete for %s in %v. Total found=%d (Internal=%d, External=%d)", startURL, finishedAt.Sub(startedAt), prog.TotalFound, prog.InternalFound, prog.ExternalFound)
	return isSSR, startedAt, finishedAt
}
