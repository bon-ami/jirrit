// gerrit_test.go
package main

import (
	"testing"
)

func TestJiraTransfer(t *testing.T) {
	// suite.Run(t, &TestSuite{t: t, category: CategoryJira, action: "transfer a case to someone"})
}

func TestJiraTransition(t *testing.T) {
	// test1(t, CategoryJira, "move status of a case")
}

func TestJiraDetail(t *testing.T) {
	// test1(t, CategoryJira, "show details of a case")
}

func TestJiraComments(t *testing.T) {
	// test1(t, CategoryJira, "list comments of a case")
}

func TestJiraAddComment(t *testing.T) {
	// test1(t, CategoryJira, "add a comment to a case")
}

func TestJiraDelComment(t *testing.T) {
	// test1(t, CategoryJira, "delete a comment from a case")
}

func TestJiraModComment(t *testing.T) {
	// test1(t, CategoryJira, "change a comment from a case")
}

func TestJiraMyOpen(t *testing.T) {
	// suite.Run(t, &TestSuite{t: t, category: CategoryJira, action: "list my open cases"})
}

func TestJiraLink(t *testing.T) {
	// test1(t, CategoryJira, "link a case to another")
}

func TestJiraWatcherList(t *testing.T) {
	// test1(t, CategoryJira, "list watchers of a case")
}

func TestJiraWatcherCheck(t *testing.T) {
	// test1(t, CategoryJira, "check whether watching a case")
}

func TestJiraWatcherAdd(t *testing.T) {
	// test1(t, CategoryJira, "watch a case")
}

func TestJiraWatcherDel(t *testing.T) {
	// test1(t, CategoryJira, "unwatch a case")
}

func TestJiraAddFile(t *testing.T) {
	// test1(t, CategoryJira, "add a file to a case")
}

func TestJiraListFile(t *testing.T) {
	// test1(t, CategoryJira, "list files attached to a case")
}

func TestJiraGetFile(t *testing.T) {
	// test1(t, CategoryJira, "get a file to a case")
}

func TestJiraDelFile(t *testing.T) {
	// test1(t, CategoryJira, "remove a file attached to a case")
}

func TestJiraReject(t *testing.T) {
	// test1(t, CategoryJira, "reject a case from any known statuses")
}

func TestJiraClose(t *testing.T) {
	// test1(t, CategoryJira, "close a case to resolved from any known statuses")
}

func TestJiraCloseDef(t *testing.T) {
	// test1(t, CategoryJira, "close a case with default design as steps")
}

func TestJiraCloseGen(t *testing.T) {
	// test1(t, CategoryJira, "close a case with general requirement as steps")
}
