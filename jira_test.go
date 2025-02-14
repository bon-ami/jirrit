// gerrit_test.go
package main

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

func JiraTests(t *testing.T, action string, dependency bool) {
	s := TestSuite{category: CategoryJira, action: action}
	if dependency {
		s.SetDependency("list my open cases", "TestJiraMyOpen")
	}
	suite.Run(t, &s)
	if s.IsSkipped() {
		t.SkipNow()
	}
}

func TestJiraDetail(t *testing.T) {
	JiraTests(t, "show details of a case", true)
}

func TestJiraComments(t *testing.T) {
	JiraTests(t, "list comments of a case", true)
}

func TestJiraMyOpen(t *testing.T) {
	JiraTests(t, "list my open cases", false)
}

func TestJiraWatcherList(t *testing.T) {
	JiraTests(t, "list watchers of a case", true)
}

func TestJiraWatcherCheck(t *testing.T) {
	JiraTests(t, "check whether watching a case", true)
}

func TestJiraWatcherAdd(t *testing.T) {
	JiraTests(t, "watch a case", true)
}

func TestJiraWatcherDel(t *testing.T) {
	JiraTests(t, "unwatch a case", true)
}

func TestJiraListFile(t *testing.T) {
	JiraTests(t, "list files attached to a case", true)
}

// cases below needs more then ID as params

func TestJiraTransfer(t *testing.T) {
	JiraTests(t, "transfer a case to someone", false)
}

func TestJiraTransition(t *testing.T) {
	JiraTests(t, "move status of a case", false)
}

func TestJiraAddComment(t *testing.T) {
	JiraTests(t, "add a comment to a case", false)
}

func TestJiraDelComment(t *testing.T) {
	JiraTests(t, "delete a comment from a case", false)
}

func TestJiraModComment(t *testing.T) {
	JiraTests(t, "change a comment from a case", false)
}

func TestJiraLink(t *testing.T) {
	JiraTests(t, "link a case to another", false)
}

func TestJiraAddFile(t *testing.T) {
	JiraTests(t, "add a file to a case", false)
}

func TestJiraGetFile(t *testing.T) {
	JiraTests(t, "get a file to a case", false)
}

func TestJiraDelFile(t *testing.T) {
	JiraTests(t, "remove a file attached to a case", false)
}

func TestJiraReject(t *testing.T) {
	JiraTests(t, "reject a case from any known statuses", false)
}

func TestJiraClose(t *testing.T) {
	JiraTests(t, "close a case to resolved from any known statuses", false)
}

func TestJiraCloseDef(t *testing.T) {
	JiraTests(t, "close a case with default design as steps", false)
}

func TestJiraCloseGen(t *testing.T) {
	JiraTests(t, "close a case with general requirement as steps", false)
}
