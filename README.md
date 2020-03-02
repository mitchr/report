# report

report examines a git repo's commit log and determines if it follows the [50/72 rule](https://preslav.me/2015/02/21/what-s-with-the-50-72-rule/).

## Build
````
go get github.com/mitchr/report
````

## Usage

To generate a report for a local repo:
````
report /path/to/repo
````

For a github repo:
````
report author/repo
````
By default, the GitHub api rate-limits requests to 60/hr, so the maximum number of commits that can be examined would be 6000. By setting the *OAUTH* environment variable to either a personal or application token, you can make up to 5000/hr, or get up to 500000 commits.
````
export OAUTH=YOUR_TOKEN_HERE
````

So basically, if you want to look at a large repo with a fairly large commit history, [get a token](https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line) (or just clone it locally and run report on that).

To show information about committers that have not followed 50/72:
````
report --blame repo/
````

## Todo
* more statistics
* limit number of concurrent goroutines so we don't hit file descriptor limit (````ulimit -n````)
	* generate stats for [torvalds/linux](https://github.com/torvalds/linux)
* ~~pagination to access entire commit log (current max is 100)~~
* ~~blame~~
