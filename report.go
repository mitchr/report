package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

const (
	SUBJ_MAX = 50
	BODY_MAX = 72
)

type commit struct {
	subject string
	body string
	author string
	hash string

	subjMarked bool
	bodyMarks []int
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
	out, err := exec.Command("git", "--git-dir="+gitDir+"/.git", "log", "--format=" + fmtStr).CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	return strings.Split(string(out), "\n")
}

var blameFlag = flag.Bool("blame", false, "show name/e-mail and hash of offenders")

func main() {
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
		// TODO: exit gracefully
		os.Exit(1)
	}

	var commits []*commit

	_, err := os.Stat(gitDir)
	// if there was an error, then this is a local directory
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
	}	else {
		url := "https://api.github.com/repos/" + gitDir + "/commits?per_page=100"

		resp, err := http.Get(url)
		if err != nil {
			log.Fatal(err)
		}

		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}

		// unmarshal json response into node objects
		type node struct {
			Sha string
			Commit struct {
				Author struct {
					Name string
					Email string
				}
				Message string
			}
		}
		container := []node{}
		json.Unmarshal([]byte(body), &container)

		// extract commits from unmarshaled JSON
		commits = make([]*commit, len(container))
		for i, v := range container {
			splitMsg := strings.SplitN(v.Commit.Message, "\n\n", 2)
			subj := splitMsg[0]
			msg := ""
			if len(splitMsg) > 1 {
				msg = splitMsg[1]
			}

			c := newCommit(subj, msg, v.Commit.Author.Name + " <" + v.Commit.Author.Email + ">", v.Sha)
			commits[i] = c
		}
	}

	subjScore := 0
	bodyLineScore := 0
	bodyCommitScore := 0
	bodyTotal := 0

	for _, c := range commits {
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

	if *blameFlag {
		blame(commits)
	}

	fmt.Printf("Number of commits with subject lines above 50 characters: %d\n", subjScore)
	fmt.Printf("Percentage of commits with subject lines above 50 characters: %f\n", 100*float64(subjScore)/float64(len(commits)))

	fmt.Printf("Number of commits with body lines over 72 characters: %d\n", bodyCommitScore)
	fmt.Printf("Percentage of commit bodies with lines above 72 characters: %f\n", 100*float64(bodyCommitScore)/float64(len(commits)))

	fmt.Printf("Number of body lines (total) over 72 characters: %d\n", bodyLineScore)
	fmt.Printf("Percentage of body lines above 72 characters: %f\n", 100*float64(bodyLineScore)/float64(bodyTotal))
	fmt.Printf("Total number of commits in dataset: %d\n", len(commits))
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
