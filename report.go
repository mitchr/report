package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

const (
	SUBJ_MAX = 50
	BODY_MAX = 72
)

type commit struct {
	subject string
	body    string
	author  string
	hash    string

	subjMarked bool
	bodyMarks  []int
}

func newCommit(subject, body, author, hash string) *commit {
	return &commit{subject, body, author, hash, false, []int{}}
}

func (c *commit) markSubj() {
	c.subjMarked = true
}

func (c *commit) markBody(lineno int) {
	c.bodyMarks = append(c.bodyMarks, lineno)
}

func parseGitLog(gitDir, fmtStr string) []string {
	out, err := exec.Command("git", "--git-dir="+gitDir+"/.git", "log", "--format="+fmtStr).CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	return strings.Split(string(out), "\n")
}

var blameFlag = flag.Bool("blame", false, "show name/e-mail and hash of offenders")
var OAUTH_TOKEN = os.Getenv("OAUTH")

func main() {
	flag.Usage = func() {
		fmt.Println("report: determine which commits in a git repo follow the 50/72 rule")
		flag.PrintDefaults()
	}
	flag.Parse()

	gitDir := ""
	if len(os.Args) > 1 {
		// if the flag is present, the directory will be the next argument
		if *blameFlag {
			gitDir = os.Args[2]
		} else {
			gitDir = os.Args[1]
		}
	} else {
		// exit gracefully
		flag.Usage()
		return
	}

	var commits []*commit

	// check if gitDir is a local directory
	_, err := os.Stat(gitDir)
	// if there was no error, then this is a local directory
	if err == nil {
		subjs := parseGitLog(gitDir, "\"%s\"")
		bodies := parseGitLog(gitDir, "\"%b\"")
		authors := parseGitLog(gitDir, "\"%an <%ae>\"")
		hashes := parseGitLog(gitDir, "\"%H\"")

		commits = make([]*commit, len(hashes))
		for i := range hashes {
			c := newCommit(subjs[i], bodies[i], authors[i], hashes[i])
			commits[i] = c
		}

		// if there was no error, its a github repo
	} else {
		// start at first 100 commits
		repoURL := "https://api.github.com/repos/" + gitDir + "/commits?per_page=100"

		// unmarshaled json will be held in this node object
		type node struct {
			Sha    string
			Commit struct {
				Author struct {
					Name  string
					Email string
				}
				Message string
			}
		}

		// client used for all http requests
		// http.DefaultTransport.(*http.Transport).TLSHandshakeTimeout = time.Minute * 2
		client := http.DefaultClient

		req, err := http.NewRequest("GET", repoURL, nil)
		if err != nil {
			log.Fatal(err)
		}

		// if the user has an api token, set authorization to get higher rate-limit
		if OAUTH_TOKEN != "" {
			req.Header.Set("Authorization", "token "+OAUTH_TOKEN)
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}

		baseURL := relParse(resp, "next")
		lastPage := 1
		// if there was more than one page of commits
		if baseURL != "" {
			baseURL = baseURL[:len(baseURL)-1]

			last := strings.Split(relParse(resp, "last"), "&page=")[1]

			// allocate enough space for all commits
			lastPage, _ = strconv.Atoi(last)
			commits = make([]*commit, lastPage*100)
		} else {
			commits = make([]*commit, 100)
		}

		// for every page of commits, spin up a new goroutine that will parse the
		// json into the appropriate commit structure
		i := 1
		var wg sync.WaitGroup
		for i <= lastPage {
			// increment WaitGroup counter
			wg.Add(1)

			go func(i int) {
				// when this goroutine finishes, decrement wg counter
				defer wg.Done()

				// pageReq := req
				// pageReq.URL, _ = url.Parse(baseURL + strconv.Itoa(i))
				pageReq, _ := http.NewRequest("GET", baseURL+strconv.Itoa(i), nil)
				pageReq.Close = true

				// small check for if there is only one page of commits
				if lastPage == 1 {
					pageReq.URL, _ = url.Parse(repoURL)
				}

				// if the user has an api token, set authorization to get higher rate-limit
				if OAUTH_TOKEN != "" {
					pageReq.Header.Set("Authorization", "token "+OAUTH_TOKEN)
				}

				resp, err := client.Do(pageReq)
				if err != nil {
					log.Fatal(err)
				}

				temp := make([]node, 100)
				json.NewDecoder(resp.Body).Decode(&temp)
				resp.Body.Close()

				for k, v := range temp {
					splitMsg := strings.SplitN(v.Commit.Message, "\n\n", 2)
					subj := splitMsg[0]
					msg := ""
					if len(splitMsg) > 1 {
						msg = splitMsg[1]
					}

					c := newCommit(subj, msg, v.Commit.Author.Name+" <"+v.Commit.Author.Email+">", v.Sha)
					commits[(i-1)*100+k] = c
				}
			}(i)
			i++
		}

		// wait for all goroutines to finish before computing statistics
		wg.Wait()
	}

	subjScore := 0
	bodyLineScore := 0
	bodyCommitScore := 0
	bodyTotal := 0

	for _, c := range commits {
		if c != nil {
			if len(c.subject) > SUBJ_MAX {
				subjScore++
				c.markSubj()
			}

			body := strings.Split(c.body, "\n")
			for lineno, line := range body {
				bodyTotal++
				if len(line) > BODY_MAX {
					bodyLineScore++

					// if this commit body has not been marked before, then this is
					// this first line in that body that has gone over the BODY_MAX
					// tolerance. we increment bodyCommitScore
					if len(c.bodyMarks) == 0 {
						bodyCommitScore++
					}

					c.markBody(lineno)
				}
			}
		}
	}

	if *blameFlag {
		blame(commits)
	}

	nonNilCommits := len(commits) - countNil(commits)

	fmt.Printf("Number of commits with subject lines above 50 characters: %d\n", subjScore)
	fmt.Printf("Percentage of commits with subject lines above 50 characters: %f\n", 100*float64(subjScore)/float64(nonNilCommits))

	fmt.Printf("Number of commits with body lines over 72 characters: %d\n", bodyCommitScore)
	fmt.Printf("Percentage of commit bodies with lines above 72 characters: %f\n", 100*float64(bodyCommitScore)/float64(nonNilCommits))

	fmt.Printf("Number of body lines (total) over 72 characters: %d\n", bodyLineScore)
	fmt.Printf("Percentage of body lines above 72 characters: %f\n", 100*float64(bodyLineScore)/float64(bodyTotal))
	fmt.Printf("Total number of commits in dataset: %d\n", nonNilCommits)
}

// takes a string url and returns the next url in the pagination sequence
// returns empty string if rel url could not be parsed from given key
func relParse(resp *http.Response, key string) string {
	headerLinks := strings.Split(resp.Header.Get("Link"), ",")
	for _, v := range headerLinks {
		next := strings.Split(v, ";")

		// TODO: what happens when there is a next page, but we *just* ran
		// out of api requests? how does it fail? is this even something to
		// worry about?
		// only investigate rel=next if there is actually more than one page
		if len(next) >= 2 {
			if strings.TrimSpace(next[1]) == "rel="+"\""+key+"\"" {
				return strings.Trim(strings.Trim(next[0], " "), "<>")
			}
		}
	}
	return ""
}

func blame(c []*commit) {
	for _, v := range c {
		if v.subjMarked {
			fmt.Println(v.author + " went over on subject")
		}

		if len(v.bodyMarks) != 0 {
			for _, line := range v.bodyMarks {
				fmt.Println(v.author + " went over on line")
				fmt.Printf("The line in question: %d\n", line)
			}
		}
	}
}

// counts number of nil elements in slice
func countNil(vals []*commit) int {
	count := 0
	for _, v := range vals {
		if v == nil {
			count++
		}
	}
	return count
}
