package main

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

func BugzillaTests(t *testing.T, action string, dependency bool) {
	s := TestSuite{category: CategoryBugzilla, action: action}
	if dependency {
		s.SetDependency("list my open cases", "TestBugzillaMyOpen")
	}
	suite.Run(t, &s)
	if s.IsSkipped() {
		t.SkipNow()
	}
}

// TestBugzillaMyOpen tests the BugzillaMyOpen function
func TestBugzillaMyOpen(t *testing.T) {
	BugzillaTests(t, "list my open cases", false)
}

// TestBugzillaComments tests the BugzillaComments function
func TestBugzillaComments(t *testing.T) {
	BugzillaTests(t, "list comments of a case", true)
}

// TestBugzillaWatcherList tests the BugzillaWatcherList function
func TestBugzillaWatcherList(t *testing.T) {
	BugzillaTests(t, "list watchers of a case", true)
}

// TestBugzillaListFile tests the BugzillaListFile function
func TestBugzillaListFile(t *testing.T) {
	BugzillaTests(t, "list files attached to a case", true)
}

// TestBugzillaGetFile tests the BugzillaGetFile function
func TestBugzillaGetFile(t *testing.T) {
	BugzillaTests(t, "get a file to a case", true)
}

// TestBugzillaDetail tests the BugzillaDetail function
func TestBugzillaDetail(t *testing.T) {
	BugzillaTests(t, "show details of a case", true)
}

// TestBugzillaAddComment tests the BugzillaAddComment function
// must-have command params: -i as ID; -c as comment
func TestBugzillaAddComment(t *testing.T) {
	BugzillaTests(t, "add a comment to a case", false)
}

// cases below needs more then ID as params

// TestBugzillaTransition tests the BugzillaTransition function
func TestBugzillaTransition(t *testing.T) {
	BugzillaTests(t, "move status of a case", false)
}

// TestBugzillaReject tests the BugzillaReject function
func TestBugzillaReject(t *testing.T) {
	BugzillaTests(t, "reject a case from any known statuses", false)
}

// TestBugzillaClose tests the BugzillaClose function
func TestBugzillaClose(t *testing.T) {
	BugzillaTests(t, "close a case to resolved from any known statuses", false)
}

// TestBugzillaLink tests the BugzillaLink function
func TestBugzillaLink(t *testing.T) {
	BugzillaTests(t, "link a case to another", false)
}

// TestBugzillaWatcherAdd tests the BugzillaWatcherAdd function
func TestBugzillaWatcherAdd(t *testing.T) {
	BugzillaTests(t, "watch a case", false)
}

// TestBugzillaWatcherDel tests the BugzillaWatcherDel function
func TestBugzillaWatcherDel(t *testing.T) {
	BugzillaTests(t, "unwatch a case", false)
}

// TestBugzillaAddFile tests the BugzillaAddFile function
func TestBugzillaAddFile(t *testing.T) {
	BugzillaTests(t, "add a file to a case", false)
}

// TestBugzillaTransfer tests the BugzillaTransfer function
func TestBugzillaTransfer(t *testing.T) {
	BugzillaTests(t, "transfer a case to someone", false)
}

// TestBugzillaCloseDef tests the BugzillaCloseDef function
func TestBugzillaCloseDef(t *testing.T) {
	BugzillaTests(t, "close a case with default design as steps", false)
}

// TestBugzillaCloseGen tests the BugzillaCloseGen function
func TestBugzillaCloseGen(t *testing.T) {
	BugzillaTests(t, "close a case with general requirement as steps", false)
}

// TestBugzillaRemoveFile tests the BugzillaDelFile function
func TestBugzillaRemoveFile(t *testing.T) {
	BugzillaTests(t, "remove a file attached to a case", false)
}

// TestBugzillaChangeComment tests the BugzillaModComment function
func TestBugzillaChangeComment(t *testing.T) {
	BugzillaTests(t, "change a comment from a case", false)
}

// TestBugzillaDeleteComment tests the BugzillaDelComment function
func TestBugzillaDeleteComment(t *testing.T) {
	BugzillaTests(t, "delete a comment from a case", false)
}
