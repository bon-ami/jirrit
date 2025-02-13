package main

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

// TestBugzillaMyOpen tests the BugzillaMyOpen function
func TestBugzillaMyOpen(t *testing.T) {
	suite.Run(t, &TestSuite{ /*t: t,*/ category: CategoryBugzilla, action: "list my open cases"})
}

/*
// TestBugzillaAddComment tests the BugzillaAddComment function
func TestBugzillaAddComment(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "add a comment to a case"})
}*/

// TestBugzillaComments tests the BugzillaComments function
func TestBugzillaComments(t *testing.T) {
	suite.Run(t, &TestSuite{ /*t: t,*/ category: CategoryBugzilla, action: "list comments of a case",
		dependsOnDesc: "list my open cases", dependsOnFunc: "TestBugzillaMyOpen"})
}

/*
// TestBugzillaTransition tests the BugzillaTransition function
func TestBugzillaTransition(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "move status of a case"})
}

// TestBugzillaReject tests the BugzillaReject function
func TestBugzillaReject(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "reject a case from any known statuses"})
}

// TestBugzillaClose tests the BugzillaClose function
func TestBugzillaClose(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "close a case to resolved from any known statuses"})
}

// TestBugzillaLink tests the BugzillaLink function
func TestBugzillaLink(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "link a case to another"})
}

// TestBugzillaWatcherList tests the BugzillaWatcherList function
func TestBugzillaWatcherList(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "list watchers of a case"})
}

// TestBugzillaWatcherAdd tests the BugzillaWatcherAdd function
func TestBugzillaWatcherAdd(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "watch a case"})
}

// TestBugzillaWatcherDel tests the BugzillaWatcherDel function
func TestBugzillaWatcherDel(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "unwatch a case"})
}

// TestBugzillaAddFile tests the BugzillaAddFile function
func TestBugzillaAddFile(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "add a file to a case"})
}

// TestBugzillaListFile tests the BugzillaListFile function
func TestBugzillaListFile(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "list files attached to a case"})
}

// TestBugzillaGetFile tests the BugzillaGetFile function
func TestBugzillaGetFile(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "get a file to a case"})
}

// TestBugzillaTransfer tests the BugzillaTransfer function
func TestBugzillaTransfer(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "transfer a case to someone"})
}

// TestBugzillaDetail tests the BugzillaDetail function
func TestBugzillaDetail(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "show details of a case"})
}

// TestBugzillaCloseDef tests the BugzillaCloseDef function
func TestBugzillaCloseDef(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "close a case with default design as steps"})
}

// TestBugzillaCloseGen tests the BugzillaCloseGen function
func TestBugzillaCloseGen(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "close a case with general requirement as steps"})
}

// TestBugzillaRemoveFile tests the BugzillaDelFile function
func TestBugzillaRemoveFile(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "remove a file attached to a case"})
}

// TestBugzillaChangeComment tests the BugzillaModComment function
func TestBugzillaChangeComment(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "change a comment from a case"})
}

// TestBugzillaDeleteComment tests the BugzillaDelComment function
func TestBugzillaDeleteComment(t *testing.T) {
	suite.Run(t, &TestSuite{t: t, category: CategoryBugzilla, action: "delete a comment from a case"})
}
*/
