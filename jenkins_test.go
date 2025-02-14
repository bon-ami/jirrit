package main

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

func JenkinsTests(t *testing.T, action string) {
	s := TestSuite{category: CategoryJenkins, action: action}
	suite.Run(t, &s)
	if s.IsSkipped() {
		t.SkipNow()
	}
}

func TestJenkinsListJobs(t *testing.T) {
	JenkinsTests(t, "list jobs")
}

func TestJenkinsListBuilds(t *testing.T) {
	JenkinsTests(t, "list builds")
}

// since there are too many builds, all tests need command line parameters

func TestJenkinsShowBuildDetails(t *testing.T) {
	JenkinsTests(t, "show details of a build")
}

func TestJenkinsGetBuildLog(t *testing.T) {
	JenkinsTests(t, "get log of a build")
}
