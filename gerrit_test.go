package main

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

func GerritTests(t *testing.T, action string, dependency bool) {
	s := TestSuite{category: CategoryGerrit, action: action}
	if dependency {
		s.SetDependency("list my open submits", "TestGerritListMyOpenSubmits")
	}
	suite.Run(t, &s)
	if s.IsSkipped() {
		t.SkipNow()
	}
}

func TestGerritListMyOpenSubmits(t *testing.T) {
	GerritTests(t, "list my open submits", true)
}

func TestGerritListAllOpenSubmits(t *testing.T) {
	GerritTests(t, "list all open submits", false)
}

func TestGerritListMyOpenCommits(t *testing.T) {
	GerritTests(t, "list my open commits", false)
}

func TestGerritShowDetailsOfSubmit(t *testing.T) {
	GerritTests(t, "show details of a submit", true)
}

func TestGerritShowRevisionsOfSubmit(t *testing.T) {
	GerritTests(t, "show revisions of a submit", true)
}

func TestGerritShowHistoryOfSubmit(t *testing.T) {
	GerritTests(t, "show history of a submit", true)
}

func TestGerritShowReviewersAndScoresOfSubmit(t *testing.T) {
	GerritTests(t, "show reviewers and scores of a submit", true)
}

func TestGerritShowCurrentRevisionOrCommitOfSubmit(t *testing.T) {
	GerritTests(t, "show current revision or commit of a submit", true)
}

func TestGerritRebaseSubmit(t *testing.T) {
	GerritTests(t, "rebase a submit", true)
}

func TestGerritShowRelatedSubmitsOfOne(t *testing.T) {
	GerritTests(t, "show related submits of one", true)
}

func TestGerritListFilesOfSubmitByRevision(t *testing.T) {
	GerritTests(t, "list files of a submit by revision", true)
}

// cases below needs more then ID as params

func TestGerritListMergedSubmits(t *testing.T) {
	GerritTests(t, "list merged submits of someone", false)
}

func TestGerritListSbOpenSubmits(t *testing.T) {
	GerritTests(t, "list sbs open submits", false)
}

func TestGerritMergeSubmit(t *testing.T) {
	GerritTests(t, "merge a submit", false)
}

func TestGerritAddScoresToSubmit(t *testing.T) {
	GerritTests(t, "add scores to a submit", false)
}

func TestGerritAddScoresAndWaitForMerge(t *testing.T) {
	GerritTests(t, "add scores, wait for it to be mergable and merge a submit", false)
}

func TestGerritWaitForMergeableAndMergeSbSubmits(t *testing.T) {
	GerritTests(t, "wait for mergable and merge sbs submits", false)
}

func TestGerritAbandonAllMyOpenSubmits(t *testing.T) {
	GerritTests(t, "abandon all my open submits", false)
}

func TestGerritAbandonSubmit(t *testing.T) {
	GerritTests(t, "abandon a submit", false)
}

func TestGerritCherryPickMyOpenSubmits(t *testing.T) {
	GerritTests(t, "cherry pick all my open submits", false)
}

func TestGerritCherryPickSubmit(t *testing.T) {
	GerritTests(t, "cherry pick a submit", false)
}

func TestGerritRevertSubmit(t *testing.T) {
	GerritTests(t, "revert a submit", false)
}

func TestGerritListConfigOfProject(t *testing.T) {
	GerritTests(t, "list config of a project", false)
}

func TestGerritDownloadFileOfSubmit(t *testing.T) {
	GerritTests(t, "download a file of a submit", false)
}

func TestGerritMultiple(t *testing.T) {
	GerritTests(t, "list my open submits;show current revision or commit of a submit", false)
}
