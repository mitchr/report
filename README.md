# report

report examines a git repo's commit log and determines if it follows the [50/72 rule](https://preslav.me/2015/02/21/what-s-with-the-50-72-rule/) rule

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

## Todo
* more statistics
* pagination to access entire commit log (current max is 100)
* blame
